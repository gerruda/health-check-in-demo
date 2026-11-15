// Package textutil содержит общие текстовые эвристики для router/app.
package textutil

import (
	"strings"
	"unicode"
)

// LooksLikeQuestion отсекает мусор вроде «ываыва» и приветствия, но пропускает
// осмысленные вопросы и однословные медицинские/тренировочные запросы.
func LooksLikeQuestion(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 4 {
		return false
	}
	if len(strings.Fields(text)) >= 2 || strings.ContainsAny(text, "?!") {
		return true
	}
	lower := strings.ToLower(text)
	for _, term := range []string{
		"креатин", "белок", "протеин", "сон", "hrv", "вср", "пульс", "тренировк",
		"питани", "жир", "вес", "алкогол", "добавк", "магний", "омега", "витамин",
		"колен", "плеч", "восстанов", "стресс", "инсулин", "печен", "masld", "бжу", "калори",
	} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
