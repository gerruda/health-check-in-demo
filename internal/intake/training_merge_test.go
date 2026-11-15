package intake_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
)

func TestServiceTrainingKeepsRulesWhenLLMIncomplete(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	// LLM отдаёт неполный список и ложную боль 0/10.
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"kind": "ulichnaya",
		"date_text": "вчера",
		"exercises": [
			{"name": "Суставная разминка", "sets": 3},
			{"name": "подтягивания узким хватом", "sets": 3, "reps": 3},
			{"name": "отжимания на брусьях", "sets": 3, "reps": 10}
		],
		"pain": {"сустав": 0},
		"done": true,
		"needs_more": ""
	}`)}
	saver := &fakeSaver{}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingLLM(provider))

	text := `Вчера была тренировка. Уличная. Суставная разминка затем подтягивания узким хватом 3 раза, отжимания на брусьях 10 раз, зашагивания на платформу с эспандером 15 раз на каждую ногу. Все упражнения по 3 подхода. RPE 8. Затем кардио подъем по лестнице на 32 этаж.`
	svc.SubmitTraining(context.Background(), 1, text)

	reply := waitReply(t, replies)
	require.Len(t, saver.trainings, 1)
	training := saver.trainings[0]
	assert.Len(t, training.Exercises, 4, "правила не должны терять зашагивания")
	require.NotNil(t, training.RPESession)
	assert.Equal(t, 8, *training.RPESession, "RPE из правил сохраняется")
	assert.Empty(t, training.Pain, "боль 0/10 не сохраняется")
	assert.Contains(t, reply, "Сохранено:")
}
