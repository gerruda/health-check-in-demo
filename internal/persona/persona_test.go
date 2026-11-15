package persona_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/persona"
)

func TestPromptDescribesPersona(t *testing.T) {
	assert.Equal(t, "Ритм", persona.Name)
	for _, marker := range []string{
		"Ритм",
		"Заземление",
		"Факты без морали",
		"не врач",
		"2–4 предложения",
		"Алкоголь",
		"Пропуск тренировки",
		"🟢 🟡 🔴",
		"🌿",
		"Red flags",
		"search_knowledge",
		"PMID",
		"в базе знаний пока нет",
		"одну короткую живую фразу",
	} {
		assert.Contains(t, persona.SystemPrompt, marker)
	}
}

func TestEmojiPalettes(t *testing.T) {
	assert.NotEmpty(t, persona.CalmEmoji)
	assert.NotEmpty(t, persona.BannedEmoji)

	calm := strings.Join(persona.CalmEmoji, " ")
	for _, banned := range persona.BannedEmoji {
		assert.NotContains(t, calm, banned)
	}
}

func TestBannedWords(t *testing.T) {
	for _, word := range []string{"надо", "должен", "срыв", "провал", "стыдно", "давай"} {
		assert.Contains(t, persona.BannedWords, word)
	}
}
