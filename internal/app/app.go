// Package app собирает и запускает healthbot: конфиг → логирование →
// самопроверка (--dry-run) или long polling в Telegram с graceful shutdown.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"healthbot/internal/config"
	"healthbot/internal/embeddings"
	"healthbot/internal/historydb"
	"healthbot/internal/intake"
	"healthbot/internal/kb"
	"healthbot/internal/llm"
	"healthbot/internal/logging"
	"healthbot/internal/metrics"
	"healthbot/internal/nlu"
	"healthbot/internal/router"
	"healthbot/internal/store"
	"healthbot/internal/telegramfmt"
	"healthbot/internal/textutil"
	"healthbot/internal/tools"
	"healthbot/internal/typing"

	"github.com/mymmrac/telego"
)

// getMeTimeout — таймаут проверки токена и прокси при старте (NFR-4).
const getMeTimeout = 15 * time.Second

// Run — точка входа приложения.
//
// В режиме dryRun к Telegram не подключается: проверяет конфигурацию и прогоняет
// /help через фейковый отправитель (CI smoke-тест, ТЗ §12).
func Run(ctx context.Context, dryRun bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Передаём секреты в логгер: они маскируются во всех записях (NFR-1).
	logger, closer, err := logging.New(cfg.LogDir, cfg.BotToken, cfg.LLMAPIKey)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := closer.Close(); cerr != nil {
			logger.Error(logging.Msg("app:Run", "не удалось закрыть лог-файл"), "error", cerr)
		}
	}()

	logger.Info(logging.Msg("app:Run", "запуск healthbot"), "config", cfg)
	if cfg.LLMMode == config.ModeAPI {
		logger.Warn(logging.Msg("app:Run", "LLM_MODE=api: текст сообщений уходит внешнему сервису (FR-6.5)"))
	}

	if dryRun {
		if err = selfCheck(logger); err != nil {
			return err
		}
		logger.Info(logging.Msg("app:Run", "dry-run: конфигурация валидна, /help отвечает"))
		return nil
	}

	if err = cfg.RequireBotToken(); err != nil {
		return err
	}
	stopMetrics, err := startMetricsServer(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer stopMetrics()

	proxyURL, err := cfg.TelegramProxy()
	if err != nil {
		return err
	}
	if proxyURL != nil {
		logger.Info(logging.Msg("app:Run", "Telegram API через прокси"), "proxy", proxyURL.Scheme+"://"+proxyURL.Host)
	}

	chatProvider, err := buildChatProvider(cfg, logger)
	if err != nil {
		return err
	}
	historyCache := store.NewCache()
	historyIndex := openHistoryIndex(ctx, cfg, logger)
	if closer, ok := historyIndex.(interface{ Close() error }); ok && closer != nil {
		defer func() {
			if closeErr := closer.Close(); closeErr != nil {
				logger.Warn(logging.Msg("app:Run", "не удалось закрыть SQLite-индекс"), "error", closeErr)
			}
		}()
	}
	agent := buildAgent(ctx, chatProvider, cfg, logger, historyCache)

	telegramClient := newTelegramHTTPClient(proxyURL)
	bot, err := telego.NewBot(cfg.BotToken,
		telego.WithHTTPClient(telegramClient),
		telego.WithLogger(newTelegoLogger(logger, cfg.BotToken)),
	)
	if err != nil {
		return logging.SanitizeError(fmt.Errorf("создать Telegram-бота: %w", err), cfg.BotToken)
	}

	// getMe проверяет токен и прокси до старта long polling и даёт имя аккаунта
	// для лога: по нему видно, под каким ботом запущен инстанс (NFR-3).
	botInfo, err := getBotInfo(ctx, bot)
	if err != nil {
		return logging.SanitizeError(err, cfg.BotToken)
	}
	logger.Info(logging.Msg("app:Run", "бот запущен на аккаунте "+botAccount(botInfo)), "bot_id", botInfo.ID)

	typingFunc := func(ctx context.Context, chatID int64) error {
		return bot.SendChatAction(ctx, &telego.SendChatActionParams{
			ChatID: telego.ChatID{ID: chatID},
			Action: telego.ChatActionTyping,
		})
	}

	commands := newCommandHandlers(cfg, logger, historyCache, historyIndex)
	intakeService, err := buildIntake(cfg, logger, bot, telegramClient, chatProvider, typingFunc, commands, historyCache, historyIndex)
	if err != nil {
		return err
	}

	startSundayScheduler(ctx, cfg, commands, bot, logger)

	handler := buildRouter(bot, logger, cfg, agent, agent, intakeService, typingFunc, commands)

	if cfg.WebhookEnabled {
		if err = runWebhook(ctx, cfg, bot, handler, logger); err != nil {
			return logging.SanitizeError(err, cfg.BotToken)
		}
		return nil
	}
	if err = runLongPolling(ctx, bot, handler, logger); err != nil {
		return logging.SanitizeError(err, cfg.BotToken)
	}
	return nil
}

// openHistoryIndex открывает SQLite read-model и перестраивает его из Markdown.
// Ошибка индекса не фатальна: бот продолжает на Markdown/cache (NFR-2).
func openHistoryIndex(ctx context.Context, cfg config.Config, logger *slog.Logger) store.Index {
	if strings.TrimSpace(cfg.DataRoot) == "" {
		return nil
	}
	path := filepath.Join(cfg.DataRoot, ".healthbot", "history.db")
	db, err := historydb.Open(path)
	if err != nil {
		logger.Warn(logging.Msg("app:openHistoryIndex", "SQLite-индекс недоступен, работаем на Markdown"),
			"error", err)
		return nil
	}
	started := time.Now()
	if err := db.Rebuild(ctx, cfg.DataRoot); err != nil {
		logger.Warn(logging.Msg("app:openHistoryIndex", "не удалось перестроить SQLite-индекс, работаем на Markdown"),
			"error", err)
		_ = db.Close()
		return nil
	}
	logger.Info(logging.Msg("app:openHistoryIndex", "SQLite-индекс истории готов"),
		"path", path, "duration_ms", time.Since(started).Milliseconds())
	return db
}

// buildRouter собирает роутер со всеми командами и приёмом данных.
// Вынесено из Run, чтобы E2E-тесты проверяли реальную проводку (а не копию).
func buildRouter(sender router.Sender, logger *slog.Logger, cfg config.Config, chat router.Chatter, ask router.AskHandler, intakeService router.Intake, typingFunc typing.ActionFunc, commands *commandHandlers) *router.Router {
	return router.New(sender, logger, router.Allowlist{
		Athlete: cfg.AthleteChatID,
		Wife:    cfg.WifeChatID,
	}, router.WithChat(chat), router.WithAsk(ask), router.WithIntake(intakeService), router.WithTyping(typingFunc),
		router.WithSettings(commands), router.WithStatus(commands), router.WithWeek(commands), router.WithReview(commands))
}

// llmCallTimeout — бюджет одного LLM-вызова (FR-6.6).
const llmCallTimeout = 45 * time.Second

// visionCallTimeout — бюджет vision-вызова: загрузка модели на слабой машине
// может занимать десятки секунд (FR-6.8.6).
const visionCallTimeout = 180 * time.Second

// buildChatProvider собирает провайдера для чата/текста (FR-6).
func buildChatProvider(cfg config.Config, logger *slog.Logger) (llm.Provider, error) {
	provider, err := llm.FromOptions(llm.Options{
		Mode:        cfg.LLMMode,
		OllamaURL:   cfg.OllamaURL,
		OllamaModel: cfg.OllamaModel,
		APIURL:      cfg.LLMAPIURL,
		APIKey:      cfg.LLMAPIKey,
		Model:       cfg.LLMModel,
		Fallback:    cfg.LLMFallback,
		Timeout:     llmCallTimeout,
		Logger:      logger,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM: %w", err)
	}
	if cfg.MetricsEnabled {
		provider = llm.WithMetrics(provider, cfg.LLMMode, llmModelName(cfg))
	}
	logger.Info(logging.Msg("app:Run", "LLM-провайдер подключён"),
		"mode", cfg.LLMMode, "model", llmModelName(cfg))
	return provider, nil
}

// buildAgent собирает агента с инструментами поверх провайдера (FR-6.7).
func buildAgent(ctx context.Context, provider llm.Provider, cfg config.Config, logger *slog.Logger, historyCache *store.Cache) *llm.Agent {
	index := loadKnowledgeIndex(ctx, cfg, logger)
	registry := tools.NewRegistry(
		tools.NewLastCheckin(cfg.DataRoot, tools.WithCache(historyCache)),
		tools.NewKnowledgeSearch(index),
	)
	agent := llm.NewAgent(provider, registry.Descriptors(), registry, logger)
	if index != nil {
		agent.WithContextProvider(func(ctx context.Context, text string) string {
			return knowledgeContext(ctx, index, text)
		})
	}
	return agent
}

// knowledgeContext форматирует top-K фрагментов для системного промпта.
func knowledgeContext(ctx context.Context, index *kb.Index, query string) string {
	if !textutil.LooksLikeQuestion(query) {
		return ""
	}
	results := index.SearchHybrid(ctx, query, nlu.Athlete, 4)
	if len(results) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, result := range results {
		builder.WriteString("- " + result.Path + " — " + result.Title)
		if result.Heading != "" {
			builder.WriteString(" / " + result.Heading)
		}
		if len(result.PMIDs) > 0 {
			builder.WriteString(" (PMID: " + strings.Join(result.PMIDs, ", ") + ")")
		}
		builder.WriteString("\n  " + strings.ReplaceAll(result.Snippet, "\n", " ") + "\n")
	}
	return builder.String()
}

// loadKnowledgeIndex строит поисковый индекс по база_знаний/: BM25 + embeddings
// (Ollama /api/embed, кэш на диске). При сбое embeddings — лексический fallback.
func loadKnowledgeIndex(ctx context.Context, cfg config.Config, logger *slog.Logger) *kb.Index {
	root := filepath.Join(cfg.DataRoot, "база_знаний")
	if !cfg.EmbeddingEnabled {
		index, err := kb.Load(root)
		if err != nil {
			logger.Warn(logging.Msg("app:loadKnowledgeIndex", "база знаний не загружена, /ask будет без источников"),
				"root", root, "error", err)
			return nil
		}
		logger.Info(logging.Msg("app:loadKnowledgeIndex", "база знаний загружена (лексический поиск)"),
			"root", root, "chunks", index.Size())
		return index
	}

	embeddingURL := cfg.EmbeddingURL
	if strings.TrimSpace(embeddingURL) == "" {
		embeddingURL = cfg.OllamaURL
	}
	embedder := embeddings.NewOllama(embeddingURL, cfg.EmbeddingModel, 60*time.Second)
	cachePath := filepath.Join(cfg.DataRoot, ".healthbot", "kb_embeddings.json")
	index, err := kb.LoadWithEmbeddings(ctx, root, cachePath, embedder)
	if err != nil {
		logger.Warn(logging.Msg("app:loadKnowledgeIndex", "embeddings недоступны, гибридный поиск отключён"),
			"root", root, "model", cfg.EmbeddingModel, "error", err)
		lexical, lexicalErr := kb.Load(root)
		if lexicalErr != nil {
			logger.Warn(logging.Msg("app:loadKnowledgeIndex", "база знаний не загружена, /ask будет без источников"),
				"root", root, "error", lexicalErr)
			return nil
		}
		return lexical
	}
	logger.Info(logging.Msg("app:loadKnowledgeIndex", "база знаний загружена"),
		"root", root, "chunks", index.Size(), "hybrid", index.HasEmbeddings(), "embedding_model", cfg.EmbeddingModel)
	return index
}

// buildVisionProvider собирает отдельный провайдер для скриншотов (FR-6.8.1).
// Возвращает nil, если vision не настроен.
func buildVisionProvider(cfg config.Config, logger *slog.Logger) (llm.Provider, error) {
	if !cfg.VisionEnabled() {
		return nil, nil
	}
	provider, err := llm.VisionFromOptions(llm.Options{
		Mode:         cfg.LLMMode,
		OllamaURL:    cfg.OllamaURL,
		OllamaModel:  cfg.OllamaModel,
		APIURL:       cfg.LLMAPIURL,
		APIKey:       cfg.LLMAPIKey,
		Model:        cfg.LLMModel,
		VisionMode:   cfg.VisionMode(),
		VisionModel:  cfg.VisionModel(),
		VisionAPIURL: cfg.VisionAPIURL(),
		VisionAPIKey: cfg.VisionAPIKey(),
		Timeout:      visionCallTimeout,
		Logger:       logger,
	})
	if err != nil {
		return nil, fmt.Errorf("vision: %w", err)
	}
	if cfg.MetricsEnabled {
		provider = llm.WithMetrics(provider, cfg.VisionMode(), cfg.VisionModel())
	}
	logger.Info(logging.Msg("app:Run", "vision-провайдер подключён"),
		"mode", cfg.VisionMode(), "model", cfg.VisionModel())
	return provider, nil
}

// buildIntake собирает приём фото (скачивание из Telegram) и текстовых чек-инов.
func buildIntake(cfg config.Config, logger *slog.Logger, bot *telego.Bot, client *http.Client, chatProvider llm.Provider, typingFunc typing.ActionFunc, coach *commandHandlers, historyCache *store.Cache, historyIndex store.IndexUpdater) (*intake.Service, error) {
	visionProvider, err := buildVisionProvider(cfg, logger)
	if err != nil {
		return nil, err
	}

	extractor := intake.NewExtractor(chatProvider, visionProvider, logger)
	downloader := &telegramDownloader{bot: bot, token: cfg.BotToken, client: client}
	send := func(ctx context.Context, chatID int64, text string) error {
		_, err := bot.SendMessage(ctx, &telego.SendMessageParams{
			ChatID:    telego.ChatID{ID: chatID},
			Text:      telegramfmt.Format(text),
			ParseMode: telegramfmt.ParseMode,
		})
		return err
	}

	saver := &storeSaver{
		writer: store.NewWriter(store.NewPaths(cfg.DataRoot), store.WithWriterCache(historyCache), store.WithIndexUpdater(historyIndex)),
		root:   cfg.DataRoot,
	}
	personResolver := func(chatID int64) nlu.Person {
		if cfg.WifeChatID != 0 && chatID == cfg.WifeChatID {
			return nlu.Wife
		}
		return nlu.Athlete
	}

	service := intake.NewService(extractor, downloader, send, logger,
		intake.WithTyping(typingFunc),
		intake.WithSaver(saver),
		intake.WithPersonResolver(personResolver),
		intake.WithCoach(coach),
		intake.WithTrainingLLM(chatProvider),
		intake.WithTrainingCoach(coach),
		intake.WithTrainingReporter(coach),
		intake.WithNutritionCoach(coach),
	)
	logger.Info(logging.Msg("app:Run", "приём данных подключён"),
		"vision", cfg.VisionEnabled(), "vision_model", cfg.VisionModel())
	return service, nil
}

// telegramDownloader скачивает файлы Telegram через прокси-клиент бота.
type telegramDownloader struct {
	bot    *telego.Bot
	token  string
	client *http.Client
}

// Download реализует intake.Downloader.
func (d *telegramDownloader) Download(ctx context.Context, fileID string) ([]byte, error) {
	file, err := d.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, logging.SanitizeError(fmt.Errorf("app: getFile: %w", err), d.token)
	}

	fileURL := "https://api.telegram.org/file/bot" + d.token + "/" + file.FilePath
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, logging.SanitizeError(fmt.Errorf("app: создать запрос файла: %w", err), d.token)
	}

	response, err := d.client.Do(request)
	if err != nil {
		return nil, logging.SanitizeError(fmt.Errorf("app: скачать файл: %w", err), d.token)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, logging.SanitizeError(fmt.Errorf("app: скачать файл: HTTP %d", response.StatusCode), d.token)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("app: прочитать файл: %w", err)
	}
	return data, nil
}

// ScanCheckin — CLI-смоук распознавания текстового чек-ина без Telegram.
func ScanCheckin(ctx context.Context, text string) (intake.Extracted, error) {
	cfg, err := config.Load()
	if err != nil {
		return intake.Extracted{}, err
	}

	logger, closer, err := logging.New(cfg.LogDir, cfg.BotToken, cfg.LLMAPIKey)
	if err != nil {
		return intake.Extracted{}, err
	}
	defer func() {
		if cerr := closer.Close(); cerr != nil {
			logger.Error(logging.Msg("app:ScanCheckin", "не удалось закрыть лог-файл"), "error", cerr)
		}
	}()

	provider, err := buildChatProvider(cfg, logger)
	if err != nil {
		return intake.Extracted{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

	extractor := intake.NewExtractor(provider, nil, logger)
	return extractor.Extract(ctx, intake.Input{Text: text})
}

// ScanImage — CLI-смоук распознавания скриншотов без Telegram (FR-6.8.6).
func ScanImage(ctx context.Context, paths []string) (intake.Extracted, error) {
	if len(paths) == 0 {
		return intake.Extracted{}, errors.New("app: не переданы пути к скриншотам")
	}

	cfg, err := config.Load()
	if err != nil {
		return intake.Extracted{}, err
	}

	logger, closer, err := logging.New(cfg.LogDir, cfg.BotToken, cfg.LLMAPIKey)
	if err != nil {
		return intake.Extracted{}, err
	}
	defer func() {
		if cerr := closer.Close(); cerr != nil {
			logger.Error(logging.Msg("app:ScanImage", "не удалось закрыть лог-файл"), "error", cerr)
		}
	}()

	visionProvider, err := buildVisionProvider(cfg, logger)
	if err != nil {
		return intake.Extracted{}, err
	}
	if visionProvider == nil {
		return intake.Extracted{}, errors.New("app: vision не настроен (задай LLM_VISION_MODE/LLM_VISION_MODEL)")
	}

	images := make([][]byte, 0, len(paths))
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return intake.Extracted{}, fmt.Errorf("app: прочитать %q: %w", path, readErr)
		}
		images = append(images, data)
	}

	ctx, cancel := context.WithTimeout(ctx, visionCallTimeout)
	defer cancel()

	extractor := intake.NewExtractor(nil, visionProvider, logger)
	return extractor.Extract(ctx, intake.Input{Images: images})
}

func llmModelName(cfg config.Config) string {
	if cfg.LLMMode == config.ModeAPI {
		return cfg.LLMModel
	}
	return cfg.OllamaModel
}

// Ask — CLI-смоук без Telegram: один вопрос активному LLM, ответ в stdout
// (FR-6.7.4). Используется для проверки модели и инструментов на живой Ollama.
func Ask(ctx context.Context, prompt string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	logger, closer, err := logging.New(cfg.LogDir, cfg.BotToken, cfg.LLMAPIKey)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := closer.Close(); cerr != nil {
			logger.Error(logging.Msg("app:Ask", "не удалось закрыть лог-файл"), "error", cerr)
		}
	}()

	provider, err := buildChatProvider(cfg, logger)
	if err != nil {
		return "", err
	}
	agent := buildAgent(ctx, provider, cfg, logger, store.NewCache())

	answer, err := agent.Ask(ctx, prompt)
	if err != nil {
		return "", logging.SanitizeError(err, cfg.BotToken, cfg.LLMAPIKey)
	}
	return answer, nil
}

// getBotInfo проверяет токен и прокси через getMe; без сети бот не запустится.
func getBotInfo(ctx context.Context, bot *telego.Bot) (*telego.User, error) {
	ctx, cancel := context.WithTimeout(ctx, getMeTimeout)
	defer cancel()

	info, err := bot.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("проверить токен (getMe): %w", err)
	}
	return info, nil
}

// botAccount возвращает человекочитаемое имя аккаунта бота для стартового лога.
func botAccount(info *telego.User) string {
	switch {
	case info.Username != "":
		return "@" + info.Username
	case info.FirstName != "":
		return info.FirstName
	default:
		return "без имени"
	}
}

// newTelegramHTTPClient создаёт HTTP-клиент для Telegram API с опциональным
// прокси (обход блокировок из РФ). Без BOT_PROXY_URL — прямое подключение:
// переменные HTTP(S)_PROXY намеренно игнорируются, источник истины — конфиг бота.
func newTelegramHTTPClient(proxyURL *url.URL) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: transport}
}

// runLongPolling запускает long polling и работает до отмены ctx
// (SIGINT/SIGTERM в main): канал обновлений закрывается, бот останавливается.
func runLongPolling(ctx context.Context, bot *telego.Bot, handler *router.Router, logger *slog.Logger) error {
	// если ранее был включён webhook — удаляем его, иначе getUpdates даст 409
	deleteCtx, cancelDelete := context.WithTimeout(ctx, 15*time.Second)
	if deleteErr := bot.DeleteWebhook(deleteCtx, nil); deleteErr != nil {
		logger.Warn(logging.Msg("app:runLongPolling", "не удалось удалить webhook перед polling"), "error", deleteErr)
	}
	cancelDelete()

	updates, err := bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("long polling: %w", err)
	}

	logger.Info(logging.Msg("app:runLongPolling", "long polling запущен, ожидаю обновления"))
	for update := range updates {
		handleSafely(ctx, handler, logger, update)
	}
	logger.Info(logging.Msg("app:runLongPolling", "long polling остановлен, бот завершает работу"))
	return nil
}

// handleSafely изолирует панику одного апдейта, чтобы бот продолжал работу.
func handleSafely(ctx context.Context, handler *router.Router, logger *slog.Logger, update telego.Update) {
	start := time.Now()
	status := "ok"
	defer func() {
		if rec := recover(); rec != nil {
			status = "panic"
			metrics.Default().IncError("update_panic")
			logger.Error(logging.Msg("app:handleSafely", "паника при обработке обновления"),
				"update_id", update.UpdateID, "panic", fmt.Sprint(rec))
		}
		metrics.Default().ObserveUpdate(updateCommand(update), status, time.Since(start))
		metrics.Default().SetLastUpdate(time.Now())
	}()
	handler.HandleUpdate(ctx, update)
}

// updateCommand даёт короткую метку команды для метрик.
func updateCommand(update telego.Update) string {
	if update.Message == nil {
		return "empty"
	}
	if len(update.Message.Photo) > 0 {
		return "photo"
	}
	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return "empty"
	}
	if strings.HasPrefix(text, "/") {
		if command := router.Command(text); command != "" {
			return command
		}
		return "command"
	}
	return "text"
}

// selfCheck проверяет связку router + /help без сети.
func selfCheck(logger *slog.Logger) error {
	sender := &memorySender{}
	handler := router.New(sender, logger, router.Allowlist{})

	handler.HandleUpdate(context.Background(), telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			Chat: telego.Chat{ID: 1},
			Text: "/help",
		},
	})

	if len(sender.texts) != 1 {
		return fmt.Errorf("self-check: ожидался 1 ответ на /help, получено %d", len(sender.texts))
	}
	if strings.TrimSpace(sender.texts[0]) == "" {
		return errors.New("self-check: пустой ответ на /help")
	}
	return nil
}

type memorySender struct {
	texts []string
}

func (m *memorySender) SendMessage(_ context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	m.texts = append(m.texts, params.Text)
	return &telego.Message{}, nil
}
