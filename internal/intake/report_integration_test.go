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

type fakeReporter struct {
	markdown string
	calls    int
}

func (f *fakeReporter) TrainingReport(_ int64, _ nlu.Training) (string, error) {
	f.calls++
	return f.markdown, nil
}

func TestServiceSavesRichTrainingReport(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	reporter := &fakeReporter{markdown: "# Уличная тренировка\n\n## Анализ"}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingReporter(reporter))

	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5, RPE 8, 45 мин, средний пульс 120")

	_ = waitReply(t, replies)
	assert.Equal(t, 1, reporter.calls)
	assert.Contains(t, saver.report, "Уличная тренировка")
}

func TestPendingRegeneratesReport(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	reporter := &fakeReporter{markdown: "# Отчёт\n"}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now,
		intake.WithTrainingReporter(reporter))

	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5, все упражнения 3 подхода")
	_ = waitReply(t, replies)
	require.Equal(t, 1, reporter.calls)

	handled := svc.TryAnswerPending(context.Background(), 1, "рпе 8, время 40 минут, средний пульс 135")
	require.True(t, handled)
	_ = waitReply(t, replies)
	assert.Equal(t, 2, reporter.calls, "после уточнения отчёт пересобран")
}
