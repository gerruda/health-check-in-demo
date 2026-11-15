package router_test

import (
	"context"
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

type photoCall struct {
	chatID       int64
	mediaGroupID string
	fileID       string
	caption      string
}

type fakeIntake struct {
	photoCalls    []photoCall
	textCalls     []string
	trainingCalls []string
	checkin       bool
	nutrition     bool
	training      bool
	pending       bool
	pendingCalls  []string
	formActive    bool
	formCalls     []string
	formAnswers   []string
}

func (f *fakeIntake) SubmitPhoto(_ context.Context, chatID int64, mediaGroupID, fileID, caption string) {
	f.photoCalls = append(f.photoCalls, photoCall{chatID: chatID, mediaGroupID: mediaGroupID, fileID: fileID, caption: caption})
}

func (f *fakeIntake) SubmitText(_ context.Context, _ int64, text string) {
	f.textCalls = append(f.textCalls, text)
}

func (f *fakeIntake) LooksLikeCheckin(string) bool {
	return f.checkin
}

func (f *fakeIntake) LooksLikeNutrition(string) bool {
	return f.nutrition
}

func (f *fakeIntake) LooksLikeTraining(string) bool {
	return f.training
}

func (f *fakeIntake) SubmitTraining(_ context.Context, _ int64, text string) {
	f.trainingCalls = append(f.trainingCalls, text)
}

func (f *fakeIntake) StartForm(_ context.Context, _ int64, form string) bool {
	f.formCalls = append(f.formCalls, form)
	return true
}

func (f *fakeIntake) HandleFormAnswer(_ context.Context, _ int64, text string) bool {
	if f.formActive {
		f.formAnswers = append(f.formAnswers, text)
		return true
	}
	return false
}

func (f *fakeIntake) CancelForm(_ context.Context, _ int64) bool {
	return f.formActive
}

func (f *fakeIntake) HasActiveForm(_ int64) bool { return f.formActive }

func (f *fakeIntake) TryAnswerPending(_ context.Context, _ int64, text string) bool {
	if f.pending {
		f.pendingCalls = append(f.pendingCalls, text)
		return true
	}
	return false
}

func photoUpdate(chatID int64) telego.Update {
	return telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			Chat: telego.Chat{ID: chatID},
			Photo: []telego.PhotoSize{
				{FileID: "small", FileSize: 10},
				{FileID: "big", FileSize: 100},
			},
			Caption:      "что тут по питанию?",
			MediaGroupID: "group-1",
		},
	}
}

func TestRouterPhotoSubmitsLargest(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	r.HandleUpdate(context.Background(), photoUpdate(1))

	require.Len(t, intake.photoCalls, 1)
	call := intake.photoCalls[0]
	assert.Equal(t, int64(1), call.chatID)
	assert.Equal(t, "big", call.fileID, "нужен самый большой размер фото")
	assert.Equal(t, "group-1", call.mediaGroupID)
	assert.Equal(t, "что тут по питанию?", call.caption)
}

func TestRouterPhotoForeignChatIgnored(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{}
	r := router.New(sender, discardLogger(), router.Allowlist{Athlete: 1}, router.WithIntake(intake))

	r.HandleUpdate(context.Background(), photoUpdate(99))

	assert.Empty(t, intake.photoCalls, "фото от чужого chat_id игнорируется")
	assert.Empty(t, sender.sent())
}

func TestRouterDocumentImageSubmits(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = ""
	update.Message.Document = &telego.Document{FileID: "doc-image", MimeType: "image/jpeg"}

	r.HandleUpdate(context.Background(), update)

	require.Len(t, intake.photoCalls, 1)
	assert.Equal(t, "doc-image", intake.photoCalls[0].fileID)
}

func TestRouterDocumentNonImageIgnored(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = ""
	update.Message.Document = &telego.Document{FileID: "doc-pdf", MimeType: "application/pdf"}

	r.HandleUpdate(context.Background(), update)

	assert.Empty(t, intake.photoCalls)
	assert.Empty(t, sender.sent())
}

func TestRouterTextCheckinSubmits(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{checkin: true}
	chatter := &fakeChatter{answer: "чат не должен вызываться"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithChat(chatter), router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = "сон 7:33, HRV 38, пульс 58, настроение 7"
	r.HandleUpdate(context.Background(), update)

	require.Len(t, intake.textCalls, 1)
	assert.Equal(t, "сон 7:33, HRV 38, пульс 58, настроение 7", intake.textCalls[0])
	assert.Empty(t, chatter.texts)
}

func TestRouterQuestionGoesToChat(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{checkin: false}
	chatter := &fakeChatter{answer: "ответ модели"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithChat(chatter), router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = "привет, как дела?"
	r.HandleUpdate(context.Background(), update)

	assert.Empty(t, intake.textCalls)
	assert.Equal(t, []string{"привет, как дела?"}, chatter.texts)
	require.Len(t, sender.sent(), 1)
}

func TestRouterPhotoWithoutIntakeReplies(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	r.HandleUpdate(context.Background(), photoUpdate(1))

	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "текст")
}

func TestRouterFreeTextNutritionGoesToIntake(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{nutrition: true}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))
	update := helpUpdate(1)
	update.Message.Text = "питание: 2300 ккал, Б 130, Ж 82, У 210"
	r.HandleUpdate(context.Background(), update)
	require.Len(t, intake.textCalls, 1)
	assert.Contains(t, intake.textCalls[0], "2300")
	assert.Empty(t, sender.sent())
}
