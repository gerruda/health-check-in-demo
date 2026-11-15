package textutil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/textutil"
)

func TestLooksLikeQuestion(t *testing.T) {
	for _, text := range []string{"почему вес встал?", "как дела?", "креатинин", "что с сном"} {
		assert.True(t, textutil.LooksLikeQuestion(text), text)
	}
	for _, text := range []string{"ываыва", "привет", "", "12345"} {
		assert.False(t, textutil.LooksLikeQuestion(text), text)
	}
}
