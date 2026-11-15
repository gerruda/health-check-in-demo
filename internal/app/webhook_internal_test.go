package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mymmrac/telego"
)

func webhookTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWebhookHandlerAcceptsValidUpdate(t *testing.T) {
	updates := make(chan telego.Update, 1)
	handler := newWebhookHTTPHandler("/telegram/webhook", "secret", updates, webhookTestLogger())
	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(`{"update_id":42}`))
	request.Header.Set(webhookSecretHeader, "secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, updates, 1)
	assert.Equal(t, 42, (<-updates).UpdateID)
}

func TestWebhookHandlerRejectsWrongSecret(t *testing.T) {
	updates := make(chan telego.Update, 1)
	handler := newWebhookHTTPHandler("/telegram/webhook", "secret", updates, webhookTestLogger())
	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(`{"update_id":42}`))
	request.Header.Set(webhookSecretHeader, "wrong")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Empty(t, updates)
}

func TestWebhookHandlerRejectsBadJSONAndMethod(t *testing.T) {
	updates := make(chan telego.Update, 1)
	handler := newWebhookHTTPHandler("/telegram/webhook", "", updates, webhookTestLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(`{`)))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/telegram/webhook", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
}

func TestWebhookHandlerReturnsBusyWhenQueueFull(t *testing.T) {
	updates := make(chan telego.Update, 1)
	updates <- telego.Update{UpdateID: 1}
	handler := newWebhookHTTPHandler("/telegram/webhook", "", updates, webhookTestLogger())
	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(`{"update_id":2}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestWebhookHealthz(t *testing.T) {
	handler := newWebhookHTTPHandler("/telegram/webhook", "", make(chan telego.Update, 1), webhookTestLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "ok", recorder.Body.String())
}
