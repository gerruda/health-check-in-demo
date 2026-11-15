package persona_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/persona"
)

func TestFlavorLines(t *testing.T) {
	assert.Contains(t, persona.StatusAside("зелёная"), "Ритм ровный")
	assert.Contains(t, persona.StatusAside("жёлтая"), "мягче")
	assert.Contains(t, persona.StatusAside("красная"), "восстановление")
	assert.Contains(t, persona.StatusAside("не определена"), "мало данных")
	assert.Contains(t, persona.WeekAside(1, 1, 0, 3, 4), "ритм")
	assert.Contains(t, persona.WeekAside(0, 0, 0, 0, 0), "пустая")
	assert.NotEmpty(t, persona.ReviewAside())
	assert.Contains(t, persona.TrainingAside(true), "восстанов")
}
