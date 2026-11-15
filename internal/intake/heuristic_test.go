package intake_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/intake"
)

func TestLooksLikeCheckin(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "сон и HRV с числами", text: "сон 7:33, HRV 38, пульс 58, настроение 7", want: true},
		{name: "только HRV с числом", text: "HRV 38", want: true},
		{name: "вес", text: "вес 102 кг", want: true},
		{name: "шаги", text: "шаги 8000", want: true},
		{name: "вчера", text: "вчера: сон 7:10, hrv 35", want: true},
		{name: "алкоголь", text: "алкоголь 2 дозы", want: true},
		{name: "жена: цикл", text: "МЦ 4, колени 2/10", want: true},
		{name: "привет", text: "привет", want: false},
		{name: "вопрос без чисел", text: "какой HRV в последнем чек-ине?", want: false},
		{name: "рассказ без чисел", text: "расскажи про сон", want: false},
		{name: "пусто", text: "", want: false},
		{name: "команда", text: "/help", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, intake.LooksLikeCheckin(tt.text))
		})
	}
}

func TestLooksLikeNutrition(t *testing.T) {
	assert.True(t, intake.LooksLikeNutrition("питание: 2300 ккал, Б 130, Ж 82, У 210"))
	assert.True(t, intake.LooksLikeNutrition("2300 ккал Б 130 Ж 82 У 210"))
	assert.False(t, intake.LooksLikeNutrition("что приготовить на ужин?"))
	assert.False(t, intake.LooksLikeNutrition("питание"))
}
