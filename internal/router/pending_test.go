package router_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

func TestRouterPendingAnswerConsumedBeforeTraining(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{pending: true, training: true}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = "RPE 7, 45 минут, средний пульс 120"
	r.HandleUpdate(context.Background(), update)

	require.Len(t, intake.pendingCalls, 1)
	assert.Empty(t, intake.trainingCalls)
	assert.Empty(t, sender.sent())
}
