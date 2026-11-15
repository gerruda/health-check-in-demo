package router_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

func TestRouterTrainingTextGoesToIntake(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{training: true}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = "присед 60x5x5, жим 40x8"
	r.HandleUpdate(context.Background(), update)

	require.Len(t, intake.trainingCalls, 1)
	assert.Empty(t, intake.textCalls, "не должен уходить в чек-ин")
	assert.Contains(t, intake.trainingCalls[0], "присед")
}

func TestRouterTrainingCommand(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = "/training присед 60x5x5"
	r.HandleUpdate(context.Background(), update)

	require.Len(t, intake.trainingCalls, 1)
	assert.Contains(t, intake.trainingCalls[0], "присед")
}

func TestRouterTrainingCommandUsage(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = "/training"
	r.HandleUpdate(context.Background(), update)

	assert.Empty(t, intake.trainingCalls)
	require.Equal(t, []string{"training"}, intake.formCalls)
	assert.Empty(t, sender.sent(), "форму запускает intake, он же шлёт первый вопрос")
}

func TestRouterCheckinStillWinsForCheckinText(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{checkin: true}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))

	update := helpUpdate(1)
	update.Message.Text = "сон 7:33, HRV 38, пульс 58"
	r.HandleUpdate(context.Background(), update)

	require.Len(t, intake.textCalls, 1)
	assert.Empty(t, intake.trainingCalls)
}
