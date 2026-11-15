package persona

import "strings"

// StatusAside — одна короткая фраза «Ритма» к сводке статуса.
func StatusAside(zone string) string {
	lower := strings.ToLower(zone)
	switch {
	case strings.Contains(lower, "зел"):
		return "🌿 Ритм ровный — можно спокойно идти по плану."
	case strings.Contains(lower, "жёлт") || strings.Contains(lower, "желт"):
		return "🌿 Сегодня лучше мягче: ровный темп важнее рывка."
	case strings.Contains(lower, "красн"):
		return "🌿 Сегодня восстановление — это тоже часть тренировки."
	default:
		return "🌿 Пока мало данных — следующий чек-ин добавит ясности."
	}
}

// WeekAside — фраза к недельной сводке по фактическим данным.
func WeekAside(green, yellow, red, trainings, days int) string {
	if days == 0 && trainings == 0 {
		return "🌿 Неделя пока пустая — начнём с одного чек-ина или короткой прогулки."
	}
	if trainings >= 3 && green >= yellow+red {
		return "🌿 Неделя сложилась в понятный ритм — не форсируем."
	}
	if trainings == 0 {
		return "🌿 Без тренировок неделя тоже бывает; тело получило паузу."
	}
	return "🌿 Смотрю на неделю целиком: важен не идеал, а следующий шаг."
}

// ReviewAside — фраза к воскресному разбору.
func ReviewAside() string {
	return "🌿 Собрал неделю в одну картину — смотрю на тренд, а не на отдельный день."
}

// TrainingAside — фраза к разбору тренировки.
func TrainingAside(deload bool) string {
	if deload {
		return "🌿 Сейчас важнее восстановиться, чем дожать."
	}
	return "🌿 Посмотрим, как тело отзовётся завтра."
}

// NutritionAside — короткая фраза к разбору питания.
func NutritionAside(adjustment string) string {
	lower := strings.ToLower(adjustment)
	if strings.Contains(lower, "ниже") || strings.Contains(lower, "выше") || strings.Contains(lower, "перебор") {
		return "🌿 Спокойно вернёмся в ритм — без крайностей и запретов."
	}
	return "🌿 Ритм держится — продолжаем спокойно."
}
