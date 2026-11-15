package typing_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/typing"
)

func TestStartSendsRepeatedlyAndStops(t *testing.T) {
	var calls atomic.Int64
	send := func(context.Context, int64) error {
		calls.Add(1)
		return nil
	}

	stop := typing.Start(context.Background(), 1, send, typing.WithInterval(10*time.Millisecond))
	require.NotNil(t, stop)

	assert.Eventually(t, func() bool { return calls.Load() >= 2 }, time.Second, 5*time.Millisecond)

	stop()
	time.Sleep(30 * time.Millisecond)
	after := calls.Load()

	time.Sleep(40 * time.Millisecond)
	assert.Equal(t, after, calls.Load(), "после stop новых action быть не должно")
}

func TestStartNilSenderIsNoop(t *testing.T) {
	stop := typing.Start(context.Background(), 1, nil)
	assert.NotNil(t, stop)
	stop()
}

func TestStartRespectsContext(t *testing.T) {
	var calls atomic.Int64
	send := func(context.Context, int64) error {
		calls.Add(1)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	typing.Start(ctx, 1, send, typing.WithInterval(10*time.Millisecond))

	assert.Eventually(t, func() bool { return calls.Load() >= 1 }, time.Second, 5*time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond)
	after := calls.Load()
	time.Sleep(40 * time.Millisecond)
	assert.Equal(t, after, calls.Load())
}
