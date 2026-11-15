package intake_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
	"healthbot/internal/llm"
)

// fakeProvider реализует llm.Provider: ответы задаются в полях.
type fakeProvider struct {
	textResponse    json.RawMessage
	visionResponse  json.RawMessage
	visionResponses []json.RawMessage
	err             error

	textCalls   int
	visionCalls int
	prompt      string
	images      [][]byte
}

func (f *fakeProvider) Chat(context.Context, llm.Request) (llm.Message, error) {
	return llm.Message{}, errors.New("не используется в тесте")
}

func (f *fakeProvider) Extract(_ context.Context, prompt string, _ json.RawMessage) (json.RawMessage, error) {
	f.textCalls++
	f.prompt = prompt
	if f.err != nil {
		return nil, f.err
	}
	return f.textResponse, nil
}

func (f *fakeProvider) ExtractVision(_ context.Context, prompt string, _ json.RawMessage, images [][]byte) (json.RawMessage, error) {
	f.visionCalls++
	f.prompt = prompt
	f.images = images
	if f.err != nil {
		return nil, f.err
	}
	if len(f.visionResponses) > 0 {
		index := f.visionCalls - 1
		if index >= len(f.visionResponses) {
			index = len(f.visionResponses) - 1
		}
		return f.visionResponses[index], nil
	}
	return f.visionResponse, nil
}

type fakeDownloader struct {
	files map[string][]byte
	err   error
	calls []string
}

func (f *fakeDownloader) Download(_ context.Context, fileID string) ([]byte, error) {
	f.calls = append(f.calls, fileID)
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.files[fileID]
	if !ok {
		return nil, errors.New("файл не найден")
	}
	return data, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newExtractor(provider llm.Provider) *intake.Extractor {
	return intake.NewExtractor(provider, provider, discardLogger())
}

func TestExtractNutritionScreenshot(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{
		"kinds": ["nutrition"],
		"date": "2026-11-11",
		"nutrition": {
			"kcal": 2480, "protein_g": 114, "fat_g": 97, "carbs_g": 291,
			"meals": [
				{"name": "завтрак", "kcal": 1140},
				{"name": "обед", "kcal": 648},
				{"name": "ужин", "kcal": 692}
			]
		}
	}`)}

	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{
		Images: [][]byte{[]byte("img")},
		Text:   "что тут по питанию?",
	})
	require.NoError(t, err)

	assert.Contains(t, ex.Kinds, "nutrition")
	assert.Equal(t, "2026-11-11", ex.Date)
	require.NotNil(t, ex.Nutrition)
	require.NotNil(t, ex.Nutrition.Kcal)
	assert.Equal(t, 2480, *ex.Nutrition.Kcal)
	require.NotNil(t, ex.Nutrition.ProteinG)
	assert.Equal(t, 114, *ex.Nutrition.ProteinG)
	require.Len(t, ex.Nutrition.Meals, 3)
	assert.Equal(t, "завтрак", ex.Nutrition.Meals[0].Name)

	assert.Contains(t, provider.prompt, time.Now().Format("2006-01-02"))
	assert.Contains(t, provider.prompt, "что тут по питанию?")
}

func TestExtractReadinessScreenshot(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{
		"kinds": ["checkin"],
		"checkin": {
			"sleep": "06:55", "hrv_ms": 28, "rhr": 62,
			"fatigue": 19, "temperature_c": "+0,4", "load_goal": 13
		}
	}`)}

	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.NoError(t, err)

	require.NotNil(t, ex.Checkin)
	require.NotNil(t, ex.Checkin.SleepH)
	assert.InDelta(t, 6.92, *ex.Checkin.SleepH, 0.01)
	assert.Equal(t, 28, *ex.Checkin.HRV)
	assert.Equal(t, 62, *ex.Checkin.RHR)
	assert.Equal(t, 19, *ex.Checkin.Fatigue)
	assert.InDelta(t, 0.4, *ex.Checkin.TempDelta, 0.001)
	assert.Equal(t, 13, *ex.Checkin.LoadGoal)
}

func TestExtractTextCheckin(t *testing.T) {
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"kinds": ["checkin"],
		"date": "2026-11-12",
		"checkin": {"sleep": "7:33", "hrv_ms": 38, "rhr": 58, "mood": 7, "alcohol_doses": 0}
	}`)}

	ex, err := intake.NewExtractor(provider, nil, discardLogger()).Extract(context.Background(), intake.Input{
		Text: "самочувствие странное: не выспался, пульс высоковат",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, provider.textCalls)
	assert.Zero(t, provider.visionCalls, "текст должен идти через Extract, а не vision")

	require.NotNil(t, ex.Checkin)
	require.NotNil(t, ex.Checkin.SleepH)
	assert.InDelta(t, 7.55, *ex.Checkin.SleepH, 0.01)
	assert.Equal(t, 38, *ex.Checkin.HRV)
	assert.Equal(t, 58, *ex.Checkin.RHR)
	assert.Equal(t, 7, *ex.Checkin.Mood)
	require.NotNil(t, ex.Checkin.AlcoholDoses)
	assert.InDelta(t, 0.0, *ex.Checkin.AlcoholDoses, 0.001)

	assert.Contains(t, provider.prompt, "самочувствие странное")
	assert.Contains(t, provider.prompt, time.Now().Format("2006-01-02"))
}

func TestExtractFlexibleNumbers(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{
		"kinds": ["nutrition"],
		"nutrition": {"kcal": "2480", "protein_g": 114.6, "fat_g": "97", "carbs_g": 291}
	}`)}

	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.NoError(t, err)
	require.NotNil(t, ex.Nutrition)
	assert.Equal(t, 2480, *ex.Nutrition.Kcal)
	assert.Equal(t, 115, *ex.Nutrition.ProteinG, "дробные значения округляются")
	assert.Equal(t, 97, *ex.Nutrition.FatG)
}

func TestExtractOutOfRange(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{
		"kinds": ["checkin"],
		"checkin": {"hrv_ms": 500, "rhr": 5}
	}`)}

	_, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.Error(t, err, "если валидных полей не осталось — просим другой скриншот")
}

func TestExtractLenientInvalidFields(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{
		"kind": "checkin",
		"date_text": "Сегодня",
		"checkin": {"sleep": "70 уд/м", "hrv_ms": 34, "rhr": 65, "sleep_score": 60}
	}`)}

	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.NoError(t, err)
	require.NotNil(t, ex.Checkin)
	assert.Nil(t, ex.Checkin.SleepH, "пульс, ошибочно попавший в сон, отбрасывается")
	require.NotNil(t, ex.Checkin.HRV)
	assert.Equal(t, 34, *ex.Checkin.HRV)
	require.NotNil(t, ex.Checkin.SleepScore)
	assert.Contains(t, ex.Checkin.Context, "качество сна 60/100")
}

func TestExtractNoData(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{"kinds": ["other"], "notes": "не вижу данных"}`)}

	_, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.Error(t, err)
}

func TestExtractImagesWithoutVisionProvider(t *testing.T) {
	provider := &fakeProvider{}
	extractor := intake.NewExtractor(provider, nil, discardLogger())

	_, err := extractor.Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vision")
}

func TestExtractProviderError(t *testing.T) {
	provider := &fakeProvider{err: errors.New("vision недоступна")}

	_, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vision недоступна")
}

func TestExtractInvalidJSON(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`это не json`)}

	_, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.Error(t, err)
}

func TestFormatReplyDateRU(t *testing.T) {
	ex := intake.Extracted{Date: "2026-11-11", Checkin: &intake.Checkin{HRV: intPtr(28)}}

	reply := intake.FormatReply(ex)
	assert.Contains(t, reply, "11.11.2026", "дата в ответе — ДД.ММ.ГГГГ")
	assert.NotContains(t, reply, "2026-11-11")
}

func TestFormatReply(t *testing.T) {
	ex := intake.Extracted{
		Kinds: []string{"nutrition", "checkin"},
		Date:  "2026-11-11",
		Nutrition: &intake.Nutrition{
			Kcal: intPtr(2480), ProteinG: intPtr(114), FatG: intPtr(97), CarbsG: intPtr(291),
		},
		Checkin: &intake.Checkin{
			SleepH: floatPtr(6.92), HRV: intPtr(28), RHR: intPtr(62),
		},
	}

	reply := intake.FormatReply(ex)
	assert.Contains(t, reply, "2480")
	assert.Contains(t, reply, "114")
	assert.Contains(t, reply, "06:55")
	assert.Contains(t, reply, "28")
	assert.True(t, strings.Contains(reply, "верно") || strings.Contains(reply, "Верно"))
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestExtractMergesTwoImages(t *testing.T) {
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind": "nutrition", "date_text": "Вчера, 11 ноября", "nutrition": {"kcal": 2480, "protein_g": 114, "fat_g": 97, "carbs_g": 291}}`),
		json.RawMessage(`{"kind": "checkin", "checkin": {"sleep": "06:55", "hrv_ms": 28, "rhr": 62, "fatigue": 19, "temperature_c": 0.4, "load_goal": 13}}`),
	}}

	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("1"), []byte("2")}})
	require.NoError(t, err)

	assert.Equal(t, 2, provider.visionCalls, "по одному vision-вызову на изображение")
	assert.Contains(t, ex.Kinds, "nutrition")
	assert.Contains(t, ex.Kinds, "checkin")
	require.NotNil(t, ex.Nutrition)
	assert.Equal(t, 2480, *ex.Nutrition.Kcal)
	require.NotNil(t, ex.Checkin)
	assert.Equal(t, 28, *ex.Checkin.HRV)
	assert.Equal(t, time.Now().AddDate(0, 0, -1).Format("2006-01-02"), ex.Date, "«Вчера» считается детерминированно")
}

func extractRules(t *testing.T, text string) (intake.Extracted, bool) {
	t.Helper()
	ex, err := intake.NewExtractor(nil, nil, discardLogger()).Extract(context.Background(), intake.Input{Text: text})
	if err != nil {
		return intake.Extracted{}, false
	}
	return ex, true
}

func TestParseCheckinTextRules(t *testing.T) {
	ex, ok := extractRules(t, "сон 7:33, HRV 38, пульс 58, настроение 7, алкоголь 0")
	require.True(t, ok)
	require.NotNil(t, ex.Checkin)
	require.NotNil(t, ex.Checkin.SleepH)
	assert.InDelta(t, 7.55, *ex.Checkin.SleepH, 0.01)
	assert.Equal(t, 38, *ex.Checkin.HRV)
	assert.Equal(t, 58, *ex.Checkin.RHR)
	assert.Equal(t, 7, *ex.Checkin.Mood)
	require.NotNil(t, ex.Checkin.AlcoholDoses)
	assert.InDelta(t, 0.0, *ex.Checkin.AlcoholDoses, 0.001)
	assert.Equal(t, time.Now().Format("2006-01-02"), ex.Date)

	yesterday, ok := extractRules(t, "вчера: сон 7:10, hrv 35, пульс 60")
	require.True(t, ok)
	assert.Equal(t, time.Now().AddDate(0, 0, -1).Format("2006-01-02"), yesterday.Date)

	wife, ok := extractRules(t, "МЦ 4, колени 2/10, голод 6")
	require.True(t, ok)
	require.NotNil(t, wife.Checkin)
	assert.Equal(t, 4, *wife.Checkin.CycleDay)
	assert.Equal(t, 2, *wife.Checkin.KneesPain)
	assert.Equal(t, 6, *wife.Checkin.Craving)

	_, ok = extractRules(t, "привет, как дела?")
	assert.False(t, ok)
}
