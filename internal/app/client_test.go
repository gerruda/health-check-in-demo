package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTelegramHTTPClientWithoutProxy(t *testing.T) {
	client := newTelegramHTTPClient(nil)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "ожидался *http.Transport")
	require.Nil(t, transport.Proxy, "без прокси Proxy должен быть nil (прямое подключение)")
}

func TestNewTelegramHTTPClientUsesProxy(t *testing.T) {
	// Канал вместо общей переменной: запись из горутины httptest-сервера и
	// чтение в тесте синхронизированы, без гонок и флейков под -race.
	gotHost := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost <- r.Host
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)

	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	client := newTelegramHTTPClient(proxyURL)
	client.Timeout = 5 * time.Second

	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/check", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case host := <-gotHost:
		assert.Equal(t, "example.invalid", host, "запрос должен уйти через прокси")
	default:
		t.Fatal("прокси не получил запрос")
	}
}
