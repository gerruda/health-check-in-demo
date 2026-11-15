package intake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
)

func TestMergeKeepsRulesAndAddsGroundedLLMExercises(t *testing.T) {
	sets := 3
	reps := 3
	rpe := 8
	rules := nlu.Training{
		Kind: "ulichnaya", RPESession: &rpe,
		Exercises: []nlu.Exercise{
			{Name: "Суставная разминка", Note: "разминка"},
			{Name: "подтягивания узким хватом", Sets: &sets, Reps: &reps},
		},
	}
	source := "Вчера была тренировка. Уличная. Суставная разминка затем подтягивания узким хватом 3 раза, отжимания на брусьях 10 раз, зашагивания на платформу с эспандером 15 раз на каждую ногу. Все упражнения по 3 подхода. RPE 8."
	llm := nlu.Training{
		Kind: "ulichnaya",
		Exercises: []nlu.Exercise{
			{Name: "отжимания на брусьях"},
			{Name: "зашагивания на платформу с эспандером"},
			{Name: "усталочная разминка"}, // галлюцинация, нет в тексте
		},
	}

	merged := mergeRuleAndLLMTraining(rules, llm, source, false)
	assert.Len(t, merged.Exercises, 4)
	assert.Equal(t, 8, *merged.RPESession)
	for _, exercise := range merged.Exercises {
		assert.NotEqual(t, "усталочная разминка", exercise.Name)
	}
}

func TestTrustRulesDoesNotCopyLLMNotes(t *testing.T) {
	rules := nlu.Training{Exercises: []nlu.Exercise{{Name: "подтягивания"}}}
	llm := nlu.Training{Exercises: []nlu.Exercise{{Name: "подтягивания", Note: "разминка"}}}
	merged := mergeRuleAndLLMTraining(rules, llm, "подтягивания", true)
	require.Len(t, merged.Exercises, 1)
	require.Empty(t, merged.Exercises[0].Note)
}
