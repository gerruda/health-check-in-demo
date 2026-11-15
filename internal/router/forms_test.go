package router_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

func TestRouterStartsForms(t *testing.T) {
	for command, form := range map[string]string{
		"/checkin": "checkin", "/food": "food", "/measure": "measure", "/training": "training",
	} {
		t.Run(command, func(t *testing.T) {
			sender := &fakeSender{}
			intake := &fakeIntake{}
			r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))
			update := helpUpdate(1)
			update.Message.Text = command
			r.HandleUpdate(context.Background(), update)
			require.Equal(t, []string{form}, intake.formCalls)
		})
	}
}

func TestRouterFormAnswerConsumed(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{formActive: true}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))
	update := helpUpdate(1)
	update.Message.Text = "7:30"
	r.HandleUpdate(context.Background(), update)
	assert.Equal(t, []string{"7:30"}, intake.formAnswers)
	assert.Empty(t, sender.sent())
}

func TestRouterCancelForm(t *testing.T) {
	sender := &fakeSender{}
	intake := &fakeIntake{formActive: true}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithIntake(intake))
	update := helpUpdate(1)
	update.Message.Text = "/cancel"
	r.HandleUpdate(context.Background(), update)
	assert.Empty(t, sender.sent())
}
