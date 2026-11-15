package intake_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

// fakeSaver фиксирует сохранения и возвращает пути для ответа.
type fakeSaver struct {
	mu           sync.Mutex
	checkins     []nlu.Checkin
	nutrition    []nlu.Nutrition
	trainings    []nlu.Training
	measurements []nlu.Measurement
	report       string
	err          error
}

func (f *fakeSaver) SaveCheckin(checkin nlu.Checkin) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkins = append(f.checkins, checkin)
	return "данные_" + string(checkin.Person) + "/ежедневные_чекины/" + checkin.Date + ".md", nil
}

func (f *fakeSaver) SaveTrainingReport(training nlu.Training, markdown string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.report = markdown
	return "данные_" + string(training.Person) + "/тренировки/" + training.Date + "_" + training.Kind + ".md", nil
}

func (f *fakeSaver) SaveTraining(training nlu.Training) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trainings = append(f.trainings, training)
	return "данные_" + string(training.Person) + "/тренировки/" + training.Date + "_" + training.Kind + ".md", nil
}

func (f *fakeSaver) SaveMeasurement(measurement nlu.Measurement) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.measurements = append(f.measurements, measurement)
	return "данные_" + string(measurement.Person) + "/замеры/" + measurement.Date + ".md", nil
}

func (f *fakeSaver) SaveNutrition(nutrition nlu.Nutrition) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nutrition = append(f.nutrition, nutrition)
	return "данные_" + string(nutrition.Person) + "/ежедневные_чекины/" + nutrition.Date + ".md", nil
}

func newSavingService(provider *fakeProvider, downloader *fakeDownloader, saver *fakeSaver, resolver func(int64) nlu.Person, now time.Time, extra ...intake.Option) (*intake.Service, chan string) {
	replies := make(chan string, 8)
	extractor := intake.NewExtractor(provider, provider, discardLogger())
	send := func(_ context.Context, _ int64, text string) error {
		replies <- text
		return nil
	}
	opts := []intake.Option{
		intake.WithDebounce(10 * time.Millisecond),
		intake.WithClock(func() time.Time { return now }),
	}
	if saver != nil {
		opts = append(opts, intake.WithSaver(saver))
	}
	if resolver != nil {
		opts = append(opts, intake.WithPersonResolver(resolver))
	}
	opts = append(opts, extra...)
	return intake.NewService(extractor, downloader, send, discardLogger(), opts...), replies
}

func TestServiceSavesCheckinFromText(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)

	svc.SubmitText(context.Background(), 1, "сон 7:33, HRV 38, пульс 58, настроение 7")

	reply := waitReply(t, replies)
	require.Len(t, saver.checkins, 1)
	checkin := saver.checkins[0]
	assert.Equal(t, "2026-11-12", checkin.Date)
	assert.Equal(t, nlu.Athlete, checkin.Person)
	require.NotNil(t, checkin.HRV)
	assert.Equal(t, 38, *checkin.HRV)
	require.NotNil(t, checkin.SleepH)
	assert.InDelta(t, 7.55, *checkin.SleepH, 0.01)

	assert.Contains(t, reply, "Сохранено:")
	assert.NotContains(t, reply, "M1")
}

func TestServiceSavesNutritionFromImage(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{visionResponse: json.RawMessage(`{
		"kind": "nutrition",
		"nutrition": {"kcal": 2480, "protein_g": 114, "fat_g": 97, "carbs_g": 291}
	}`)}
	saver := &fakeSaver{}
	downloader := &fakeDownloader{files: map[string][]byte{"photo": []byte("img")}}
	svc, replies := newSavingService(provider, downloader, saver, nil, now)

	svc.SubmitPhoto(context.Background(), 1, "", "photo", "")

	reply := waitReply(t, replies)
	require.Len(t, saver.nutrition, 1)
	nutrition := saver.nutrition[0]
	require.NotNil(t, nutrition.Kcal)
	assert.Equal(t, 2480, *nutrition.Kcal)
	assert.Equal(t, "2026-11-12", nutrition.Date)
	assert.Contains(t, reply, "Сохранено:")
}

func TestServiceSavesWife(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	resolver := func(chatID int64) nlu.Person {
		if chatID == 2 {
			return nlu.Wife
		}
		return nlu.Athlete
	}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, resolver, now)

	svc.SubmitText(context.Background(), 2, "сон 7:10, HRV 35, пульс 60")

	waitReply(t, replies)
	require.Len(t, saver.checkins, 1)
	assert.Equal(t, nlu.Wife, saver.checkins[0].Person)
}

func TestServiceSaveDateFromProvider(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"date": "2026-11-11",
		"checkin": {"hrv_ms": 30}
	}`)}
	saver := &fakeSaver{}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now)

	// текст без чисел → LLM-путь (не правила)
	svc.SubmitText(context.Background(), 1, "самочувствие странное, не выспался")

	waitReply(t, replies)
	require.Len(t, saver.checkins, 1)
	assert.Equal(t, "2026-11-11", saver.checkins[0].Date)
}

func TestServiceSaveError(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{err: os.ErrPermission}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)

	svc.SubmitText(context.Background(), 1, "сон 7:33, HRV 38, пульс 58")

	reply := waitReply(t, replies)
	assert.Contains(t, reply, "Не удалось сохранить")
	assert.Contains(t, reply, "38", "распознанные данные всё равно показываем")
}

func TestServiceNoSaverKeepsOldReply(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, nil, nil, now)

	svc.SubmitText(context.Background(), 1, "сон 7:33, HRV 38")

	reply := waitReply(t, replies)
	assert.NotContains(t, reply, "Сохранено:")
	assert.Contains(t, reply, "Всё верно")
}

func TestFormatReplySaved(t *testing.T) {
	extracted := intake.Extracted{
		Date:    "2026-11-11",
		Checkin: &intake.Checkin{HRV: intPtr(38)},
	}

	reply := intake.FormatReply(extracted, "данные_пользователя/ежедневные_чекины/2026-11-11.md")
	assert.Contains(t, reply, "Сохранено: данные_пользователя/ежедневные_чекины/2026-11-11.md")
	assert.Contains(t, reply, "11.11.2026")
	assert.NotContains(t, reply, "M1")
}

// storeSaver — реальный адаптер к store.Writer для интеграционного теста.
type storeSaver struct {
	writer *store.Writer
	root   string
}

func (s storeSaver) SaveCheckin(checkin nlu.Checkin) (string, error) {
	path, err := s.writer.WriteCheckin(checkin)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return path, nil
	}
	return filepath.ToSlash(relative), nil
}

func (s storeSaver) SaveTraining(training nlu.Training) (string, error) {
	path, err := s.writer.WriteTraining(training)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return path, nil
	}
	return filepath.ToSlash(relative), nil
}

func (s storeSaver) SaveNutrition(nutrition nlu.Nutrition) (string, error) {
	path, err := s.writer.WriteNutrition(nutrition)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return path, nil
	}
	return filepath.ToSlash(relative), nil
}

func TestServiceSavesTraining(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)

	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5, жим 40x8")

	reply := waitReply(t, replies)
	require.Len(t, saver.trainings, 1)
	training := saver.trainings[0]
	assert.Equal(t, "sila-A", training.Kind)
	assert.Equal(t, "2026-11-12", training.Date)
	assert.Equal(t, nlu.Athlete, training.Person)
	assert.Len(t, training.Exercises, 2)
	assert.Contains(t, reply, "Отметил тренировку")
	assert.Contains(t, reply, "Сохранено:")
}

func TestServiceTrainingNaturalLanguage(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)

	text := `Вчера была тренировка. Уличная. Суставная разминка затем подтягивания узким хватом 3 раза, отжимания на брусьях 10 раз, зашагивания на платформу с эспандером 15 раз на каждую ногу. Все упражнения по 3 подхода. RPE 8. Затем кардио подъем по лестнице на 32 этаж.`
	svc.SubmitTraining(context.Background(), 1, text)

	reply := waitReply(t, replies)
	require.Len(t, saver.trainings, 1)
	training := saver.trainings[0]
	assert.Equal(t, "2026-11-11", training.Date)
	assert.Equal(t, "ulichnaya", training.Kind)
	assert.Len(t, training.Exercises, 4)
	require.NotNil(t, training.RPESession)
	assert.Equal(t, 8, *training.RPESession)
	assert.Contains(t, training.Cardio, "лестниц")
	assert.Contains(t, reply, "Отметил тренировку")
	assert.Contains(t, reply, "Сохранено:")
}

func TestServiceTrainingWithRealStore(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := storeSaver{writer: store.NewWriter(store.NewPaths(root)), root: root}
	replies := make(chan string, 4)
	extractor := intake.NewExtractor(&fakeProvider{}, &fakeProvider{}, discardLogger())
	send := func(_ context.Context, _ int64, text string) error {
		replies <- text
		return nil
	}
	svc := intake.NewService(extractor, &fakeDownloader{}, send, discardLogger(),
		intake.WithSaver(saver),
		intake.WithClock(func() time.Time { return now }),
	)

	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5, жим гантелей 14 кг 4×10 RPE 7")

	reply := waitReply(t, replies)
	assert.Contains(t, reply, "Отметил тренировку")
	path := filepath.Join(root, "данные_пользователя", "тренировки", "2026-11-12_sila-A.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "присед · 60 кг · 5×5")
	assert.Contains(t, content, "жим гантелей · 14 кг · 4×10 · RPE 7")
}

func TestServiceSaveIdempotent(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := storeSaver{writer: store.NewWriter(store.NewPaths(root)), root: root}

	replies := make(chan string, 8)
	extractor := intake.NewExtractor(&fakeProvider{}, &fakeProvider{}, discardLogger())
	send := func(_ context.Context, _ int64, text string) error {
		replies <- text
		return nil
	}
	svc := intake.NewService(extractor, &fakeDownloader{}, send, discardLogger(),
		intake.WithSaver(saver),
		intake.WithClock(func() time.Time { return now }),
	)

	text := "сон 7:33, HRV 38, пульс 58, настроение 7"
	svc.SubmitText(context.Background(), 1, text)
	waitReply(t, replies)
	path := filepath.Join(root, "данные_пользователя", "ежедневные_чекины", "2026-11-12.md")
	first, err := os.ReadFile(path)
	require.NoError(t, err)

	svc.SubmitText(context.Background(), 1, text)
	waitReply(t, replies)
	second, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second), "повтор без изменений не должен менять файл")
	content := string(second)
	assert.Equal(t, 1, strings.Count(content, "- HRV: 38 мс"), "без дубликатов")
	assert.NotContains(t, content, "Температура", "некритичные поля не сохраняются")
	assert.NotContains(t, content, "Цель нагрузки", "некритичные поля не сохраняются")
}

func TestServiceSavesNutritionFromText(t *testing.T) {
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	provider := &fakeProvider{textResponse: []byte(`{"kind":"nutrition","nutrition":{"kcal":2300,"protein_g":130,"fat_g":82,"carbs_g":210}}`)}
	saver := &fakeSaver{}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now)

	svc.SubmitText(context.Background(), 1, "питание: 2300 ккал, Б 130, Ж 82, У 210")

	_ = waitReply(t, replies)
	require.Len(t, saver.nutrition, 1)
	assert.Equal(t, 2300, *saver.nutrition[0].Kcal)
	assert.Equal(t, 130, *saver.nutrition[0].ProteinG)
}

func TestNutritionErrorUsesNutritionHint(t *testing.T) {
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	provider := &fakeProvider{err: errors.New("llm down")}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now)

	svc.SubmitText(context.Background(), 1, "питание: 2300 ккал, Б 130, Ж 82, У 210")
	reply := waitReply(t, replies)
	assert.Contains(t, reply, "разобрать питание")
	assert.NotContains(t, reply, "чек-ин")
}
