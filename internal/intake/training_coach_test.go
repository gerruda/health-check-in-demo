package intake_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
	"healthbot/internal/nlu"
)

type fakeTrainingCoach struct {
	reply  string
	err    error
	calls  int
	passed nlu.Training
}

func (f *fakeTrainingCoach) TrainingReply(_ int64, training nlu.Training) (string, error) {
	f.calls++
	f.passed = training
	return f.reply, f.err
}

func TestServiceTrainingCoachReply(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	coach := &fakeTrainingCoach{reply: "🌿 Следующая: присед 62.5 кг · 5×5"}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingCoach(coach))

	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5")

	reply := waitReply(t, replies)
	require.Equal(t, 1, coach.calls)
	assert.Contains(t, reply, "Следующая")
	assert.Contains(t, reply, "Сохранено:")
	assert.NotContains(t, reply, "🌿 Отметил тренировку")
}
