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

type fakeNutritionCoach struct {
	reply  string
	err    error
	calls  int
	chatID int64
	last   nlu.Nutrition
}

func (f *fakeNutritionCoach) NutritionReply(chatID int64, nutrition nlu.Nutrition) (string, error) {
	f.calls++
	f.chatID = chatID
	f.last = nutrition
	return f.reply, f.err
}

func TestNutritionCoachReplacesSimpleConfirmation(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	provider := &fakeProvider{visionResponse: []byte(`{"kind":"nutrition","nutrition":{"kcal":2480,"protein_g":95}}`)}
	saver := &fakeSaver{}
	coach := &fakeNutritionCoach{reply: "🌿 Питание: 2480/2300 ккал · Б 95/108–135.\nПравка: белка ниже цели."}
	downloader := &fakeDownloader{files: map[string][]byte{"photo": []byte("img")}}
	svc, replies := newSavingService(provider, downloader, saver, nil, now,
		intake.WithNutritionCoach(coach))

	svc.SubmitPhoto(context.Background(), 1, "", "photo", "")

	reply := waitReply(t, replies)
	require.Equal(t, 1, coach.calls)
	assert.Contains(t, reply, "Питание: 2480")
	assert.Contains(t, reply, "Сохранено:")
	assert.NotContains(t, reply, "Отметил питание")
}

func TestFoodFormUsesNutritionCoach(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	coach := &fakeNutritionCoach{reply: "🌿 Питание: в цели."}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now,
		intake.WithNutritionCoach(coach))
	require.True(t, svc.StartForm(context.Background(), 1, "food"))
	_ = waitReply(t, replies)
	for _, value := range []string{"2300", "130", "82", "210"} {
		require.True(t, svc.HandleFormAnswer(context.Background(), 1, value))
		_ = waitReply(t, replies)
	}
	assert.Equal(t, 1, coach.calls)
	require.Len(t, saver.nutrition, 1)
}
