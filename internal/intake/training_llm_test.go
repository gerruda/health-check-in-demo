package intake_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
	"healthbot/internal/nlu"
)

func TestTrainingExtractor(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"kind": "уличная",
		"date_text": "вчера",
		"rpe_session": 8,
		"exercises": [
			{"name": "подтягивания узким хватом", "sets": 3, "reps": 3},
			{"name": "отжимания на брусьях", "sets": 3, "reps": 10},
			{"name": "зашагивания на платформу", "sets": 3, "reps": 15, "note": "на каждую ногу"}
		],
		"cardio": "подъем по лестнице на 32 этаж",
		"done": true,
		"needs_more": ""
	}`)}
	extractor := intake.NewTrainingExtractor(provider, discardLogger(), func() time.Time { return now })

	training, question, err := extractor.Extract(context.Background(), "вчера тренировка", nlu.Athlete)
	require.NoError(t, err)
	assert.Empty(t, question)
	assert.Equal(t, "ulichnaya", training.Kind)
	assert.Equal(t, "2026-11-11", training.Date)
	require.Len(t, training.Exercises, 3)
	require.NotNil(t, training.Exercises[1].Reps)
	assert.Equal(t, 10, *training.Exercises[1].Reps)
	require.NotNil(t, training.RPESession)
	assert.Equal(t, 8, *training.RPESession)
	assert.Contains(t, training.Cardio, "лестниц")
}

func TestTrainingExtractorNeedsMore(t *testing.T) {
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"done": true,
		"needs_more": "Сколько подходов и повторов было в подтягиваниях?"
	}`)}
	extractor := intake.NewTrainingExtractor(provider, discardLogger(), time.Now)

	training, question, err := extractor.Extract(context.Background(), "что-то делал", nlu.Athlete)
	require.NoError(t, err)
	assert.Contains(t, question, "подход")
	assert.False(t, intake.HasTrainingData(training))
}

func TestServiceTrainingLLMPrimary(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"kind": "sila-A",
		"date_text": "сегодня",
		"rpe_session": 8,
		"exercises": [{"name": "присед", "weight_kg": 60, "sets": 5, "reps": 5}],
		"done": true,
		"needs_more": ""
	}`)}
	saver := &fakeSaver{}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingLLM(provider))

	svc.SubmitTraining(context.Background(), 1, "сделал силовую, приседал 60 кг")

	reply := waitReply(t, replies)
	require.Len(t, saver.trainings, 1)
	training := saver.trainings[0]
	assert.Equal(t, "2026-11-12", training.Date)
	assert.Equal(t, "sila-A", training.Kind)
	require.Len(t, training.Exercises, 1)
	assert.Contains(t, reply, "Отметил тренировку")
}

func TestServiceTrainingLLMQuestion(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"done": true,
		"needs_more": "Сколько подходов было в отжиманиях?"
	}`)}
	saver := &fakeSaver{}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingLLM(provider))

	svc.SubmitTraining(context.Background(), 1, "делал отжимания как-то")

	reply := waitReply(t, replies)
	assert.Empty(t, saver.trainings, "при нехватке данных не сохраняем")
	assert.Contains(t, reply, "Сколько подходов")
	assert.NotContains(t, reply, "Сохранено")
}

func TestServiceTrainingLLMFallbackToRules(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{err: errors.New("модель недоступна")}
	saver := &fakeSaver{}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingLLM(provider))

	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5")

	reply := waitReply(t, replies)
	require.Len(t, saver.trainings, 1, "при ошибке LLM работает детерминированный парсер")
	assert.Contains(t, reply, "Отметил тренировку")
}
