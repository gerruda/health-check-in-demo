package intake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
)

type fakeCoach struct {
	reply     string
	err       error
	calls     int
	chatID    int64
	extracted intake.Extracted
}

func (f *fakeCoach) CheckinReply(chatID int64, extracted intake.Extracted) (string, error) {
	f.calls++
	f.chatID = chatID
	f.extracted = extracted
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

func TestServiceCoachReplyAfterSave(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	coach := &fakeCoach{reply: "🌿 Доброе утро.\n🟢 Зона: зелёная — по плану"}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now,
		intake.WithCoach(coach))

	svc.SubmitText(context.Background(), 1, "сон 7:33, HRV 38, пульс 58")

	reply := waitReply(t, replies)
	require.Equal(t, 1, coach.calls)
	assert.Equal(t, int64(1), coach.chatID)
	require.NotNil(t, coach.extracted.Checkin)
	assert.Contains(t, reply, "🌿 Доброе утро.")
	assert.Contains(t, reply, "Сохранено:")
	assert.NotContains(t, reply, "Отметил данные", "коуч-ответ заменяет сырую сводку")
}

func TestServiceCoachErrorFallsBack(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	coach := &fakeCoach{err: errors.New("reader: нет данных")}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now,
		intake.WithCoach(coach))

	svc.SubmitText(context.Background(), 1, "сон 7:33, HRV 38")

	reply := waitReply(t, replies)
	assert.Equal(t, 1, coach.calls)
	assert.Contains(t, reply, "Отметил данные", "при ошибке коуча — обычная сводка")
	assert.Contains(t, reply, "Сохранено:")
}

func TestServiceCoachNotCalledForNutritionOnly(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{visionResponse: []byte(`{"kind":"nutrition","nutrition":{"kcal":2480}}`)}
	saver := &fakeSaver{}
	coach := &fakeCoach{reply: "не должен вызываться"}
	downloader := &fakeDownloader{files: map[string][]byte{"photo": []byte("img")}}
	svc, replies := newSavingService(provider, downloader, saver, nil, now,
		intake.WithCoach(coach))

	svc.SubmitPhoto(context.Background(), 1, "", "photo", "")

	reply := waitReply(t, replies)
	assert.Zero(t, coach.calls, "коуч нужен только для чек-ина")
	assert.Contains(t, reply, "Сохранено:")
}
