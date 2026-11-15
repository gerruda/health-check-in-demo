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

func TestServiceDoesNotTakeHallucinatedRPEAndDuration(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"kind": "ulichnaya",
		"duration_min": 4,
		"rpe_session": 3,
		"exercises": [{"name": "подтягивания", "sets": 2, "reps": 2}],
		"done": true,
		"needs_more": ""
	}`)}
	saver := &fakeSaver{}
	svc, replies := newSavingService(provider, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingLLM(provider))

	text := `утром была уличная тренировка, подтягивания 2 раза, все упражнения 2 подхода, затем подъем по лестнице на 32 этаж`
	svc.SubmitTraining(context.Background(), 1, text)

	reply := waitReply(t, replies)
	require.Len(t, saver.trainings, 1)
	training := saver.trainings[0]
	assert.Nil(t, training.DurationMin, "4 мин — галлюцинация LLM, текст не указывал время")
	assert.Nil(t, training.RPESession, "RPE 3 — галлюцинация LLM, текст не указывал RPE")
	assert.Contains(t, reply, "уточни")
	assert.Contains(t, reply, "RPE")
}
