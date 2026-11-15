package intake

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"healthbot/internal/llm"
	"healthbot/internal/logging"
	"healthbot/internal/nlu"
	"healthbot/internal/typing"
)

// TrainingReporter собирает markdown-отчёт тренировки по шаблону дневника.
type TrainingReporter interface {
	TrainingReport(chatID int64, training nlu.Training) (string, error)
}

// WithTrainingReporter подключает генератор отчёта (утро/таблица/анализ/прогрессия).
func WithTrainingReporter(reporter TrainingReporter) Option {
	return func(s *Service) {
		s.trainingReporter = reporter
	}
}

// TrainingCoach собирает разбор тренировки/следующую тренировку (FR-3.3).
type TrainingCoach interface {
	TrainingReply(chatID int64, training nlu.Training) (string, error)
}

// WithTrainingCoach подключает разбор тренировки после сохранения.
func WithTrainingCoach(coach TrainingCoach) Option {
	return func(s *Service) {
		s.trainingCoach = coach
	}
}

// WithTrainingLLM подключает LLM-извлечение тренировок под шаблон
// (fallback — детерминированный парсер).
func WithTrainingLLM(provider llm.Provider) Option {
	return func(s *Service) {
		if provider == nil {
			return
		}
		s.trainingLLM = NewTrainingExtractor(provider, s.log, s.now)
	}
}

// LooksLikeTraining — реализация эвристики для роутера.
func (s *Service) LooksLikeTraining(text string) bool {
	return LooksLikeTraining(text)
}

// SubmitTraining принимает текстовый отчёт о тренировке (FR-4).
func (s *Service) SubmitTraining(ctx context.Context, chatID int64, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	go s.processTraining(context.WithoutCancel(ctx), chatID, text)
}

func (s *Service) processTraining(baseCtx context.Context, chatID int64, text string) {
	s.procMu.Lock()
	defer s.procMu.Unlock()

	ctx, cancel := context.WithTimeout(baseCtx, processTimeout)
	defer cancel()

	stopTyping := typing.Start(ctx, chatID, s.typing)
	defer stopTyping()

	person := s.personFor(chatID)
	extraPaths := s.saveCheckinFromText(ctx, chatID, person, text)

	rulesTraining, rulesOK := ParseTraining(text, person, s.now())
	training, ok := rulesTraining, rulesOK

	if s.trainingLLM != nil {
		llmTraining, question, err := s.trainingLLM.Extract(ctx, text, person)
		switch {
		case err != nil:
			s.log.Warn(logging.Msg("intake:processTraining", "LLM-разбор недоступен, остаёмся на правилах"),
				"chat_id", chatID, "error", err)
		case HasTrainingData(llmTraining):
			training, ok = mergeRuleAndLLMTraining(rulesTraining, llmTraining, text, rulesOK), true
		case strings.TrimSpace(question) != "" && !rulesOK:
			s.sendText(ctx, chatID, "🌿 Похоже на тренировку, но не хватает деталей. "+question)
			return
		}
	}

	if ok {
		s.log.Info(logging.Msg("intake:processTraining", "тренировка собрана"),
			"chat_id", chatID,
			"rules_ok", rulesOK,
			"rules_exercises", len(rulesTraining.Exercises),
			"merged_exercises", len(training.Exercises),
			"has_rpe", training.RPESession != nil,
			"has_cardio", strings.TrimSpace(training.Cardio) != "")
	}
	if !ok {
		s.sendText(ctx, chatID, "Не понял тренировку. Формат: «силовая A: присед 60x5x5, жим 40x8, RPE 7» или /training с описанием.")
		return
	}
	if training.Date == "" {
		training.Date = s.now().Format("2006-01-02")
	}

	path, saveErr := s.persistTraining(ctx, chatID, training)

	reply := FormatTrainingReply(training, "")
	if saveErr == nil && s.trainingCoach != nil {
		if advice, coachErr := s.trainingCoach.TrainingReply(chatID, training); coachErr != nil {
			s.log.Warn(logging.Msg("intake:processTraining", "не удалось собрать разбор тренировки"),
				"chat_id", chatID, "error", coachErr)
		} else if strings.TrimSpace(advice) != "" {
			reply = strings.TrimSpace(advice)
		}
	}
	allPaths := append(extraPaths, path)
	allPaths = compactPaths(allPaths)
	if len(allPaths) > 0 {
		reply += "\nСохранено: " + strings.Join(allPaths, "; ")
	}
	if saveErr != nil {
		reply += "\n⚠️ Не удалось сохранить: " + saveErr.Error()
	}
	if saveErr == nil {
		if missing := missingTrainingFields(training); len(missing) > 0 {
			s.rememberPending(chatID, training)
			reply += "\n\n🌿 Чтобы отчёт был точнее, уточни: " + strings.Join(missing, ", ") + "."
		} else {
			s.clearPending(chatID)
		}
	}
	s.sendText(ctx, chatID, reply)
}

func (s *Service) persistTraining(ctx context.Context, chatID int64, training nlu.Training) (string, error) {
	if s.saver == nil {
		return "", nil
	}
	path, saveErr := s.saver.SaveTraining(training)
	if saveErr != nil {
		s.log.Error(logging.Msg("intake:processTraining", "не удалось сохранить тренировку"),
			"chat_id", chatID, "error", saveErr)
		return "", saveErr
	}
	if s.trainingReporter != nil {
		if reportSaver, ok := s.saver.(ReportSaver); ok {
			report, reportErr := s.trainingReporter.TrainingReport(chatID, training)
			if reportErr != nil {
				s.log.Warn(logging.Msg("intake:processTraining", "не удалось собрать отчёт тренировки"),
					"chat_id", chatID, "error", reportErr)
			} else if report != "" {
				reportPath, reportErr := reportSaver.SaveTrainingReport(training, report)
				if reportErr != nil {
					s.log.Warn(logging.Msg("intake:processTraining", "не удалось сохранить отчёт тренировки"),
						"chat_id", chatID, "error", reportErr)
				} else {
					path = reportPath
				}
			}
		}
	}
	return path, nil
}

func missingTrainingFields(training nlu.Training) []string {
	missing := make([]string, 0, 3)
	if training.RPESession == nil {
		missing = append(missing, "RPE (1–10)")
	}
	if training.DurationMin == nil {
		missing = append(missing, "длительность (мин)")
	}
	if training.AvgHR == nil {
		missing = append(missing, "средний пульс")
	}
	return missing
}

func (s *Service) rememberPending(chatID int64, training nlu.Training) {
	s.pendingMu.Lock()
	s.pending[chatID] = training
	s.pendingMu.Unlock()
}

func (s *Service) clearPending(chatID int64) {
	s.pendingMu.Lock()
	delete(s.pending, chatID)
	s.pendingMu.Unlock()
}

// TryAnswerPending обрабатывает ответ на уточнение RPE/длительности/пульса.
// Возвращает true, если сообщение было ответом и обработано.
func (s *Service) TryAnswerPending(ctx context.Context, chatID int64, text string) bool {
	s.pendingMu.Lock()
	training, ok := s.pending[chatID]
	s.pendingMu.Unlock()
	if !ok {
		return false
	}
	lowerText := strings.ToLower(text)
	// не перехватываем новый чек-ин/тренировку, пока ждём уточнение
	if strings.Contains(lowerText, "сон") || strings.Contains(lowerText, "hrv") ||
		strings.Contains(lowerText, "вариаб") {
		return false
	}
	if strings.TrimSpace(training.Date) == "" {
		date, dateOK := parsePendingDate(text, s.now())
		if !dateOK {
			return false
		}
		training.Date = date.Format("2006-01-02")
		path, saveErr := s.persistTraining(ctx, chatID, training)
		if saveErr != nil {
			s.sendText(ctx, chatID, "🌿 Не удалось сохранить тренировку: "+saveErr.Error())
			s.clearPending(chatID)
			return true
		}
		reply := FormatTrainingReply(training, "")
		if s.trainingCoach != nil {
			if advice, coachErr := s.trainingCoach.TrainingReply(chatID, training); coachErr == nil && strings.TrimSpace(advice) != "" {
				reply = strings.TrimSpace(advice)
			}
		}
		if path != "" {
			reply += "\nСохранено: " + path
		}
		if missing := missingTrainingFields(training); len(missing) > 0 {
			s.rememberPending(chatID, training)
			reply += "\n\n🌿 Чтобы отчёт был точнее, уточни: " + strings.Join(missing, ", ") + "."
		} else {
			s.clearPending(chatID)
		}
		s.sendText(ctx, chatID, reply)
		return true
	}

	rpe, duration, avgHR := parseTrainingDetails(text)
	if rpe == nil && duration == nil && avgHR == nil {
		lower := strings.ToLower(text)
		if strings.Contains(lower, "не знаю") || strings.Contains(lower, "неизвестно") ||
			strings.Contains(lower, "нет данных") || strings.Contains(lower, "пропусти") {
			s.clearPending(chatID)
			s.sendText(ctx, chatID, "🌿 Ок, оставил как есть. Если вспомнишь — пришли одним сообщением.")
			return true
		}
		return false
	}
	if rpe != nil {
		training.RPESession = rpe
	}
	if duration != nil {
		training.DurationMin = duration
	}
	if avgHR != nil {
		training.AvgHR = avgHR
	}
	if cardioDuration, cardioHR := parseCardioDetails(text); cardioDuration != nil || cardioHR != nil {
		detail := "кардио"
		if cardioDuration != nil {
			detail += fmt.Sprintf(" %d мин", *cardioDuration)
		}
		if cardioHR != nil {
			detail += fmt.Sprintf(", ср. пульс %d", *cardioHR)
		}
		if strings.TrimSpace(training.Cardio) == "" {
			training.Cardio = detail
		} else if !strings.Contains(strings.ToLower(training.Cardio), "мин") {
			training.Cardio += " · " + detail
		}
	}

	path, saveErr := s.persistTraining(ctx, chatID, training)
	if saveErr != nil {
		s.sendText(ctx, chatID, "🌿 Не удалось обновить отчёт: "+saveErr.Error())
		s.clearPending(chatID)
		return true
	}

	parts := make([]string, 0, 3)
	if rpe != nil {
		parts = append(parts, fmt.Sprintf("RPE %d", *rpe))
	}
	if duration != nil {
		parts = append(parts, fmt.Sprintf("длительность %d мин", *duration))
	}
	if avgHR != nil {
		parts = append(parts, fmt.Sprintf("средний пульс %d", *avgHR))
	}
	reply := "🌿 Обновил: " + strings.Join(parts, ", ") + "."
	if path != "" {
		reply += "\nСохранено: " + path
	}
	s.clearPending(chatID)
	s.sendText(ctx, chatID, reply)
	return true
}

var (
	pendingRPEPattern      = regexp.MustCompile(`(?i)(?:\brpe\b|рпе)\s*[:=]?\s*(\d{1,2})`)
	pendingDurationPattern = regexp.MustCompile(`(?i)(\d{1,3})\s*(?:мин|минут)`)
	pendingAvgHRPattern    = regexp.MustCompile(`(?i)(?:средн[а-яё]*\s*)?(?:пульс|чсс)\D{0,10}(\d{2,3})`)
)

var (
	pendingCardioDurationPattern = regexp.MustCompile(`(?i)кардио\D{0,20}(\d{1,3})\s*(?:мин|минут)`)
	pendingCardioHRPattern       = regexp.MustCompile(`(?i)кардио[^.]{0,40}?(?:средн[а-яё]*\s*)?(?:пульс|чсс)\D{0,10}(\d{2,3})`)
)

func parseCardioDetails(text string) (duration, avgHR *int) {
	if match := pendingCardioDurationPattern.FindStringSubmatch(text); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil && value > 0 && value <= 600 {
			duration = &value
		}
	}
	if match := pendingCardioHRPattern.FindStringSubmatch(text); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil && value >= 30 && value <= 220 {
			avgHR = &value
		}
	}
	return duration, avgHR
}

var pendingDateNumeric = regexp.MustCompile(`(?i)\b(\d{1,2})[./](\d{1,2})(?:[./](\d{2,4}))?\b`)

// parsePendingDate разбирает ответ на уточнение даты: вчера/сегодня,
// 13.11, 13.11.2026 или «13 ноября».
func parsePendingDate(text string, now time.Time) (time.Time, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(lower, "позавчера"):
		return now.AddDate(0, 0, -2), true
	case strings.Contains(lower, "вчера"):
		return now.AddDate(0, 0, -1), true
	case strings.Contains(lower, "сегодня"):
		return now, true
	}
	if parsed := parseDateText(text, now); parsed != "" {
		if date, err := time.Parse("2006-01-02", parsed); err == nil {
			return date, true
		}
	}
	if match := pendingDateNumeric.FindStringSubmatch(lower); match != nil {
		day, errDay := strconv.Atoi(match[1])
		month, errMonth := strconv.Atoi(match[2])
		year := now.Year()
		if match[3] != "" {
			parsedYear, errYear := strconv.Atoi(match[3])
			if errYear != nil {
				return time.Time{}, false
			}
			if parsedYear < 100 {
				parsedYear += 2000
			}
			year = parsedYear
		}
		if errDay != nil || errMonth != nil || day < 1 || day > 31 || month < 1 || month > 12 {
			return time.Time{}, false
		}
		candidate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
		if match[3] == "" && candidate.After(now.AddDate(0, 0, 1)) {
			candidate = candidate.AddDate(-1, 0, 0)
		}
		return candidate, true
	}
	return time.Time{}, false
}

func parseTrainingDetails(text string) (*int, *int, *int) {
	var rpe, duration, avgHR *int
	if match := pendingRPEPattern.FindStringSubmatch(text); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil && value >= 1 && value <= 10 {
			rpe = &value
		}
	}
	if match := pendingDurationPattern.FindStringSubmatch(text); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil && value > 0 && value <= 600 {
			duration = &value
		}
	}
	if match := pendingAvgHRPattern.FindStringSubmatch(text); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil && value >= 30 && value <= 220 {
			avgHR = &value
		}
	}
	return rpe, duration, avgHR
}

// saveCheckinFromText сохраняет чек-ин, если сообщение содержит метрики сна/HRV/пульса
// (комбинированное сообщение «чек-ин + тренировка»).
func (s *Service) saveCheckinFromText(ctx context.Context, chatID int64, person nlu.Person, text string) []string {
	if s.saver == nil {
		return nil
	}
	parsed, ok := parseCheckinText(text, s.now())
	if !ok || parsed.Checkin == nil {
		return nil
	}
	date := s.now()
	if parsed.Date != "" {
		if value, err := time.Parse("2006-01-02", parsed.Date); err == nil {
			date = value
		}
	}
	path, err := s.saver.SaveCheckin(toCheckin(*parsed.Checkin, person, date))
	if err != nil {
		s.log.Warn(logging.Msg("intake:processTraining", "не удалось сохранить чек-ин из комбинированного сообщения"),
			"chat_id", chatID, "error", err)
		return nil
	}
	return []string{path}
}

func compactPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}
