package intake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"healthbot/internal/logging"
	"healthbot/internal/nlu"
	"healthbot/internal/typing"
)

const (
	defaultDebounce  = 1200 * time.Millisecond
	defaultMaxImages = 10
	processTimeout   = 120 * time.Second
)

// Downloader скачивает файл Telegram по file_id (реализация — в app).
type Downloader interface {
	Download(ctx context.Context, fileID string) ([]byte, error)
}

// SendFunc отправляет текст в чат.
type SendFunc func(ctx context.Context, chatID int64, text string) error

// Option настраивает сервис.
type Option func(*Service)

// WithDebounce меняет окно сборки альбома (по умолчанию 1.2 с).
func WithDebounce(debounce time.Duration) Option {
	return func(s *Service) {
		if debounce > 0 {
			s.debounce = debounce
		}
	}
}

// WithTyping подключает индикатор «печатает» на время обработки (UX).
func WithTyping(action typing.ActionFunc) Option {
	return func(s *Service) {
		s.typing = action
	}
}

// WithMaxImages ограничивает число изображений в одном альбоме.
func WithMaxImages(maxImages int) Option {
	return func(s *Service) {
		if maxImages > 0 {
			s.maxImages = maxImages
		}
	}
}

// Service принимает фото (альбомы) и текст, извлекает данные и отвечает в чат.
type Service struct {
	extractor        *Extractor
	downloader       Downloader
	send             SendFunc
	log              *slog.Logger
	debounce         time.Duration
	maxImages        int
	typing           typing.ActionFunc
	saver            Saver
	coach            Coach
	nutritionCoach   NutritionCoach
	trainingCoach    TrainingCoach
	trainingReporter TrainingReporter
	trainingLLM      *TrainingExtractor
	personResolver   func(chatID int64) nlu.Person
	now              func() time.Time

	mu     sync.Mutex
	groups map[string]*photoGroup

	procMu sync.Mutex // один vision/LLM-вызов за раз (память, NFR)

	pendingMu sync.Mutex
	pending   map[int64]nlu.Training

	formMu sync.Mutex
	forms  map[int64]*formState

	seenMu sync.Mutex
	seen   map[string]time.Time
}

type photoGroup struct {
	chatID  int64
	baseCtx context.Context
	fileIDs []string
	caption string
	timer   *time.Timer
}

// NewService создаёт сервис приёма данных.
func NewService(extractor *Extractor, downloader Downloader, send SendFunc, log *slog.Logger, opts ...Option) *Service {
	if log == nil {
		log = slog.Default()
	}
	service := &Service{
		extractor:  extractor,
		downloader: downloader,
		send:       send,
		log:        log,
		debounce:   defaultDebounce,
		maxImages:  defaultMaxImages,
		now:        time.Now,
		groups:     make(map[string]*photoGroup),
		pending:    make(map[int64]nlu.Training),
		forms:      make(map[int64]*formState),
		seen:       make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(service)
	}
	// единые часы: WithClock должен влиять и на экстрактор (иначе дата
	// «сегодня» у правил/LLM расходится с датой сохранения в тестах и replay)
	if service.extractor != nil {
		service.extractor.now = service.now
	}
	return service
}

// SubmitPhoto принимает фото: одиночное обрабатывается сразу, альбом
// (media_group_id) собирается в окне debounce и обрабатывается одним вызовом.
func (s *Service) SubmitPhoto(ctx context.Context, chatID int64, mediaGroupID, fileID, caption string) {
	if strings.TrimSpace(fileID) == "" {
		return
	}

	if mediaGroupID == "" {
		go s.processImages(context.WithoutCancel(ctx), chatID, []string{fileID}, caption)
		return
	}

	key := fmt.Sprintf("%d:%s", chatID, mediaGroupID)

	s.mu.Lock()
	group, ok := s.groups[key]
	if !ok {
		group = &photoGroup{chatID: chatID, baseCtx: context.WithoutCancel(ctx)}
		s.groups[key] = group
	}
	if len(group.fileIDs) < s.maxImages {
		group.fileIDs = append(group.fileIDs, fileID)
	}
	if strings.TrimSpace(caption) != "" {
		group.caption = caption
	}
	if group.timer != nil {
		group.timer.Stop()
	}
	group.timer = time.AfterFunc(s.debounce, func() { s.flush(key) })
	s.mu.Unlock()
}

// LooksLikeCheckin сообщает, похож ли текст на чек-ин (FR-6.8.8).
func (s *Service) LooksLikeCheckin(text string) bool {
	return LooksLikeCheckin(text)
}

// LooksLikeNutrition — эвристика маршрутизации питания.
func (s *Service) LooksLikeNutrition(text string) bool {
	return LooksLikeNutrition(text)
}

// SubmitText принимает текстовый чек-ин и отвечает подтверждением.
func (s *Service) SubmitText(ctx context.Context, chatID int64, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	go s.processText(context.WithoutCancel(ctx), chatID, text)
}

func (s *Service) flush(key string) {
	s.mu.Lock()
	group, ok := s.groups[key]
	if ok {
		delete(s.groups, key)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	go s.processImages(group.baseCtx, group.chatID, group.fileIDs, group.caption)
}

func (s *Service) processImages(baseCtx context.Context, chatID int64, fileIDs []string, caption string) {
	s.procMu.Lock()
	defer s.procMu.Unlock()

	ctx, cancel := context.WithTimeout(baseCtx, processTimeout)
	defer cancel()

	stopTyping := typing.Start(ctx, chatID, s.typing)
	defer stopTyping()

	images := make([][]byte, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		data, err := s.downloader.Download(ctx, fileID)
		if err != nil {
			s.log.Error(logging.Msg("intake:processImages", "не удалось скачать скриншот"),
				"chat_id", chatID, "error", err)
			s.sendText(ctx, chatID, "Не удалось скачать скриншот. Попробуй прислать ещё раз или текстом.")
			return
		}
		images = append(images, data)
	}

	fingerprint := imageFingerprint(images, caption)
	if s.alreadySeen(chatID, fingerprint) {
		s.sendText(ctx, chatID, "Этот скриншот уже обрабатывал — дубль не сохраняю. Если нужно, пришли другой или текст.")
		return
	}

	extracted, err := s.extractor.Extract(ctx, Input{Images: images, Text: caption})
	if err != nil {
		s.forgetImage(chatID, fingerprint)
		s.log.Error(logging.Msg("intake:processImages", "не удалось распознать скриншот"),
			"chat_id", chatID, "error", err)
		if !s.extractor.HasVision() {
			s.sendText(ctx, chatID, "Не могу обработать изображение: vision не настроен. Пришли данные текстом.")
			return
		}
		s.sendText(ctx, chatID, "Не удалось распознать данные на скриншоте. Пришли текстом или другим скриншотом.")
		return
	}

	s.log.Info(logging.Msg("intake:processImages", "скриншот обработан"),
		"chat_id", chatID, "images", len(images))

	if extracted.Training != nil && strings.TrimSpace(extracted.Training.Date) == "" {
		// не сохраняем тренировку молча «на сегодня»: уточняем дату
		person := s.personFor(chatID)
		training := toVisionTraining(*extracted.Training, person, s.now())
		training.Date = ""
		s.rememberPending(chatID, training)
		withoutTraining := extracted
		withoutTraining.Training = nil
		paths, saveErrs := s.save(withoutTraining, chatID)
		for _, saveErr := range saveErrs {
			s.log.Error(logging.Msg("intake:processImages", "не удалось сохранить данные"),
				"chat_id", chatID, "error", saveErr)
		}
		reply := "🌿 Распознал тренировку, но не увидел дату на скриншоте.\nНапиши дату (например «13.11» или «вчера») — сохраню с разбором."
		if len(paths) > 0 {
			reply += "\nСохранено: " + strings.Join(paths, "; ")
		}
		s.sendText(ctx, chatID, reply)
		return
	}

	paths, saveErrs := s.save(extracted, chatID)
	for _, saveErr := range saveErrs {
		s.log.Error(logging.Msg("intake:processImages", "не удалось сохранить данные"),
			"chat_id", chatID, "error", saveErr)
	}
	if len(saveErrs) > 0 {
		s.forgetImage(chatID, fingerprint)
	}
	s.sendText(ctx, chatID, s.buildReply(extracted, chatID, paths, saveErrs))
}

func imageFingerprint(images [][]byte, caption string) string {
	hash := sha256.New()
	for _, image := range images {
		_, _ = hash.Write(image)
	}
	_, _ = hash.Write([]byte(caption))
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Service) alreadySeen(chatID int64, fingerprint string) bool {
	s.seenMu.Lock()
	defer s.seenMu.Unlock()
	now := s.now()
	key := fmt.Sprintf("%d:%s", chatID, fingerprint)
	if seen, ok := s.seen[key]; ok && now.Sub(seen) < 10*time.Minute {
		return true
	}
	for item, seen := range s.seen {
		if now.Sub(seen) > time.Hour {
			delete(s.seen, item)
		}
	}
	s.seen[key] = now
	return false
}

func (s *Service) forgetImage(chatID int64, fingerprint string) {
	s.seenMu.Lock()
	defer s.seenMu.Unlock()
	delete(s.seen, fmt.Sprintf("%d:%s", chatID, fingerprint))
}

func (s *Service) processText(baseCtx context.Context, chatID int64, text string) {
	s.procMu.Lock()
	defer s.procMu.Unlock()

	ctx, cancel := context.WithTimeout(baseCtx, processTimeout)
	defer cancel()

	stopTyping := typing.Start(ctx, chatID, s.typing)
	defer stopTyping()

	extracted, err := s.extractor.Extract(ctx, Input{Text: text})
	if err != nil {
		s.log.Error(logging.Msg("intake:processText", "не удалось распознать чек-ин"),
			"chat_id", chatID, "error", err)
		if LooksLikeNutrition(text) {
			s.sendText(ctx, chatID, "Не удалось разобрать питание. Напиши в формате: «2300 ккал, Б 130, Ж 82, У 210».")
		} else {
			s.sendText(ctx, chatID, "Не удалось распознать чек-ин. Напиши в формате: «сон 7:33, HRV 38, пульс 58, настроение 7».")
		}
		return
	}

	s.log.Info(logging.Msg("intake:processText", "чек-ин обработан"), "chat_id", chatID)

	paths, saveErrs := s.save(extracted, chatID)
	for _, saveErr := range saveErrs {
		s.log.Error(logging.Msg("intake:processText", "не удалось сохранить данные"),
			"chat_id", chatID, "error", saveErr)
	}
	s.sendText(ctx, chatID, s.buildReply(extracted, chatID, paths, saveErrs))
}

// buildReply собирает ответ: сохранённый чек-ин → коуч-ответ (зона/HRV_7/
// ориентир), иначе обычная сводка; ошибки сохранения добавляются предупреждением.
func (s *Service) buildReply(extracted Extracted, chatID int64, paths []string, saveErrs []error) string {
	checkinAdvice := ""
	if s.coach != nil && extracted.Checkin != nil && len(paths) > 0 {
		advice, err := s.coach.CheckinReply(chatID, extracted)
		if err != nil {
			s.log.Error(logging.Msg("intake:buildReply", "не удалось собрать коуч-ответ"),
				"chat_id", chatID, "error", err)
		} else {
			checkinAdvice = strings.TrimSpace(advice)
		}
	}

	nutritionAdvice := ""
	if s.nutritionCoach != nil && extracted.Nutrition != nil && len(paths) > 0 {
		nutrition := toNutrition(*extracted.Nutrition, s.personFor(chatID), s.now())
		if extracted.Date != "" {
			if parsed, parseErr := time.Parse("2006-01-02", extracted.Date); parseErr == nil {
				nutrition.Date = parsed.Format("2006-01-02")
			}
		}
		advice, err := s.nutritionCoach.NutritionReply(chatID, nutrition)
		if err != nil {
			s.log.Error(logging.Msg("intake:buildReply", "не удалось собрать разбор питания"),
				"chat_id", chatID, "error", err)
		} else {
			nutritionAdvice = strings.TrimSpace(advice)
		}
	}

	trainingAdvice := ""
	if s.trainingCoach != nil && extracted.Training != nil && len(paths) > 0 {
		training := s.trainingFromExtracted(extracted, chatID)
		advice, err := s.trainingCoach.TrainingReply(chatID, training)
		if err != nil {
			s.log.Error(logging.Msg("intake:buildReply", "не удалось собрать разбор тренировки"),
				"chat_id", chatID, "error", err)
		} else {
			trainingAdvice = strings.TrimSpace(advice)
		}
	}

	reply := ""
	switch {
	case checkinAdvice != "" && trainingAdvice != "":
		reply = checkinAdvice + "\n\n" + trainingAdvice
	case checkinAdvice != "":
		reply = checkinAdvice
		if line := formatTraining(extracted.Training); line != "" {
			reply += "\n" + line
		}
	case trainingAdvice != "":
		reply = trainingAdvice
	default:
		if nutritionAdvice == "" {
			return withSaveErrors(FormatReply(extracted, paths...), saveErrs)
		}
		reply = nutritionAdvice
	}
	if nutritionAdvice != "" && !strings.Contains(reply, nutritionAdvice) {
		reply += "\n\n" + nutritionAdvice
	}

	if len(paths) > 0 {
		reply += "\nСохранено: " + strings.Join(paths, "; ")
	}
	return withSaveErrors(reply, saveErrs)
}

func (s *Service) trainingFromExtracted(extracted Extracted, chatID int64) nlu.Training {
	date := s.now()
	if extracted.Training != nil && extracted.Training.Date != "" {
		if parsed, err := time.Parse("2006-01-02", extracted.Training.Date); err == nil {
			date = parsed
		}
	}
	return toVisionTraining(*extracted.Training, s.personFor(chatID), date)
}

func withSaveErrors(reply string, saveErrs []error) string {
	if len(saveErrs) > 0 {
		reply += "\n⚠️ Не удалось сохранить: " + saveErrs[0].Error()
	}
	return reply
}

func (s *Service) sendText(ctx context.Context, chatID int64, text string) {
	if err := s.send(ctx, chatID, text); err != nil {
		s.log.Error(logging.Msg("intake:sendText", "не удалось отправить ответ"),
			"chat_id", chatID, "error", err)
	}
}
