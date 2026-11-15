package intake_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
)

func newTestService(provider *fakeProvider, downloader *fakeDownloader, replies chan string) *intake.Service {
	extractor := intake.NewExtractor(provider, provider, discardLogger())
	send := func(_ context.Context, _ int64, text string) error {
		replies <- text
		return nil
	}
	return intake.NewService(extractor, downloader, send, discardLogger(), intake.WithDebounce(20*time.Millisecond))
}

func waitReply(t *testing.T, replies chan string) string {
	t.Helper()
	select {
	case reply := <-replies:
		return reply
	case <-time.After(3 * time.Second):
		t.Fatal("ответ в чат не пришёл")
		return ""
	}
}

func TestServiceAlbumCombinesPhotos(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{
		"kinds": ["nutrition"],
		"nutrition": {"kcal": 2480, "protein_g": 114, "fat_g": 97, "carbs_g": 291}
	}`)}
	downloader := &fakeDownloader{files: map[string][]byte{
		"photo1": []byte("img1"),
		"photo2": []byte("img2"),
	}}
	replies := make(chan string, 4)
	svc := newTestService(provider, downloader, replies)

	svc.SubmitPhoto(context.Background(), 1, "album-1", "photo1", "")
	svc.SubmitPhoto(context.Background(), 1, "album-1", "photo2", "что тут по питанию?")

	reply := waitReply(t, replies)
	assert.Equal(t, 2, provider.visionCalls, "альбом: по одному vision-вызову на изображение")
	require.Len(t, provider.images, 1, "за вызов передаётся одно изображение")
	assert.Contains(t, provider.prompt, "что тут по питанию?")
	assert.Contains(t, reply, "2480")
}

func TestServiceSinglePhoto(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{"kinds":["checkin"],"checkin":{"hrv_ms":28}}`)}
	downloader := &fakeDownloader{files: map[string][]byte{"photo1": []byte("img1")}}
	replies := make(chan string, 4)
	svc := newTestService(provider, downloader, replies)

	svc.SubmitPhoto(context.Background(), 1, "", "photo1", "")

	reply := waitReply(t, replies)
	assert.Equal(t, 1, provider.visionCalls)
	require.Len(t, provider.images, 1)
	assert.Contains(t, reply, "28")
}

func TestServiceSeparateMessages(t *testing.T) {
	provider := &fakeProvider{visionResponse: json.RawMessage(`{"kinds":["checkin"],"checkin":{"hrv_ms":28}}`)}
	downloader := &fakeDownloader{files: map[string][]byte{"p1": []byte("1"), "p2": []byte("2")}}
	replies := make(chan string, 4)
	svc := newTestService(provider, downloader, replies)

	svc.SubmitPhoto(context.Background(), 1, "", "p1", "")
	svc.SubmitPhoto(context.Background(), 1, "", "p2", "")

	waitReply(t, replies)
	waitReply(t, replies)
	assert.Equal(t, 2, provider.visionCalls, "раздельные сообщения — два вызова")
}

func TestServiceTextCheckin(t *testing.T) {
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"kinds": ["checkin"],
		"checkin": {"sleep": "7:33", "hrv_ms": 38, "rhr": 58, "mood": 7}
	}`)}
	replies := make(chan string, 4)
	svc := newTestService(provider, &fakeDownloader{}, replies)

	svc.SubmitText(context.Background(), 1, "сон 7:33, HRV 38, пульс 58, настроение 7")

	reply := waitReply(t, replies)
	assert.Zero(t, provider.textCalls, "текст с числами разбирается правилами, без LLM")
	assert.Zero(t, provider.visionCalls)
	assert.Contains(t, reply, "38")
	assert.Contains(t, reply, "7:33")
}

func TestServiceVisionError(t *testing.T) {
	provider := &fakeProvider{err: errors.New("модель перегружена")}
	downloader := &fakeDownloader{files: map[string][]byte{"photo1": []byte("img1")}}
	replies := make(chan string, 4)
	svc := newTestService(provider, downloader, replies)

	svc.SubmitPhoto(context.Background(), 1, "", "photo1", "")

	reply := waitReply(t, replies)
	assert.Contains(t, strings.ToLower(reply), "не удалось")
}

func TestServicePhotoWithoutVision(t *testing.T) {
	provider := &fakeProvider{}
	extractor := intake.NewExtractor(provider, nil, discardLogger())
	replies := make(chan string, 4)
	send := func(_ context.Context, _ int64, text string) error {
		replies <- text
		return nil
	}
	svc := intake.NewService(extractor, &fakeDownloader{files: map[string][]byte{"p": []byte("x")}}, send, discardLogger())

	svc.SubmitPhoto(context.Background(), 1, "", "p", "")

	reply := waitReply(t, replies)
	assert.Contains(t, reply, "vision")
}
