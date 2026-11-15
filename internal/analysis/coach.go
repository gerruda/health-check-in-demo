package analysis

import (
	"fmt"
	"strings"
)

// ZoneCounts — сколько дней в каждой зоне за окно (обычно 7 дней).
type ZoneCounts struct {
	Green  int
	Yellow int
	Red    int
}

// CheckinReplyInput — данные для ответа на чек-ин (контракт §6.1).
type CheckinReplyInput struct {
	Zone       Result
	HRV        HRVTrend
	ZoneCounts ZoneCounts
	Plan       string
	Focus      string
	Flag       string
}

// CheckinReply собирает ответ на чек-ин (≤10 строк, контракт §6.1).
func CheckinReply(input CheckinReplyInput) string {
	zone := input.Zone.Zone
	if zone == "" {
		zone = ZoneUnknown
	}
	reason := strings.TrimSpace(input.Zone.Reason)
	if reason == "" {
		reason = "причина не указана"
	}

	lines := []string{
		"🌿 Доброе утро.",
		fmt.Sprintf("%s Зона: %s — %s", zone.Emoji(), zoneName(zone), reason),
		hrvTrendLine(input.HRV, input.ZoneCounts),
	}

	plan := strings.TrimSpace(input.Plan)
	if plan == "" {
		plan = PlanForZone(zone)
	}
	lines = append(lines, "Ориентир: "+plan)

	if len(input.Zone.Notes) > 0 {
		lines = append(lines, "Заметки: "+strings.Join(input.Zone.Notes, "; "))
	}
	if focus := strings.TrimSpace(input.Focus); focus != "" {
		lines = append(lines, "Фокус: "+focus)
	}
	if flag := strings.TrimSpace(input.Flag); flag != "" {
		lines = append(lines, "Сигнал: "+flag)
	}
	return strings.Join(lines, "\n")
}

// NutritionReplyInput — данные для ответа на питание (контракт §6.3).
type NutritionReplyInput struct {
	Result   NutritionResult
	Averages string
	Alcohol  string
}

// NutritionReply собирает ответ на питание (≤8 строк, контракт §6.3).
func NutritionReply(input NutritionReplyInput) string {
	lines := []string{"🌿 Питание: " + input.Result.Summary}

	flags := make([]string, 0, len(input.Result.Checks))
	for _, check := range input.Result.Checks {
		switch {
		case check.Value == nil:
			flags = append(flags, "— "+check.Name+" нет данных")
		case check.OK:
			flags = append(flags, "✅ "+check.Name)
		default:
			flags = append(flags, "⚠️ "+check.Name)
		}
	}
	lines = append(lines, strings.Join(flags, " · "))

	if averages := strings.TrimSpace(input.Averages); averages != "" {
		lines = append(lines, "Средние "+averages)
	}
	if alcohol := strings.TrimSpace(input.Alcohol); alcohol != "" {
		lines = append(lines, "Алкоголь: "+alcohol)
	}
	if fix := strings.TrimSpace(input.Result.Fix); fix != "" {
		lines = append(lines, fix)
	}
	return strings.Join(lines, "\n")
}

func hrvTrendLine(trend HRVTrend, counts ZoneCounts) string {
	if trend.Count == 0 {
		return "HRV_7: нет данных"
	}

	line := fmt.Sprintf("HRV_7: %.1f мс · %s %s",
		trend.Average, trend.Direction.Arrow(), directionName(trend.Direction))
	if total := counts.Green + counts.Yellow + counts.Red; total > 0 {
		line += fmt.Sprintf(" · 🟢%d / 🟡%d / 🔴%d", counts.Green, counts.Yellow, counts.Red)
	}
	return line
}

func zoneName(zone Zone) string {
	switch zone {
	case ZoneGreen:
		return "зелёная"
	case ZoneYellow:
		return "жёлтая"
	case ZoneRed:
		return "красная"
	default:
		return "не определена"
	}
}

func directionName(direction Direction) string {
	switch direction {
	case DirectionUp:
		return "рост"
	case DirectionDown:
		return "снижение"
	default:
		return "стабильно"
	}
}

// PlanForZone возвращает ориентир по умолчанию для зоны (FR-3.1).
func PlanForZone(zone Zone) string {
	switch zone {
	case ZoneGreen:
		return "сегодня по плану"
	case ZoneYellow:
		return "одна лёгкая тренировка, сдвоенные убрать"
	case ZoneRed:
		return "восстановление: отдых или прогулка"
	default:
		return "нужно больше данных — пришли HRV и пульс"
	}
}
