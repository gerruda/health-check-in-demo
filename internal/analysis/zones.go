// Package analysis содержит чистые функции анализа данных проекта (ТЗ §5):
// зоны готовности и цели питания. Без сети и файловой системы — тестируется
// без Telegram; интеграция в ответы выполняется в router/coach.
package analysis

import "fmt"

// Zone — зона готовности (контракт §7).
type Zone string

// Возможные зоны.
const (
	ZoneGreen   Zone = "green"
	ZoneYellow  Zone = "yellow"
	ZoneRed     Zone = "red"
	ZoneUnknown Zone = "unknown"
)

// Name возвращает русское название зоны для ответа в чат.
func (z Zone) Name() string {
	switch z {
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

// Emoji возвращает эмодзи зоны для ответа в чат.
func (z Zone) Emoji() string {
	switch z {
	case ZoneGreen:
		return "🟢"
	case ZoneYellow:
		return "🟡"
	case ZoneRed:
		return "🔴"
	default:
		return "⚪"
	}
}

// Thresholds — пороги зоны для конкретного человека.
type Thresholds struct {
	HRVBase        int     // база HRV, мс (7-дневное среднее или override)
	GreenMargin    int     // зелёная: HRV ≥ база − margin
	RedMargin      int     // красная: HRV < база − margin
	RHRGreenMax    int     // зелёная: пульс ≤ max
	RHRRedMin      int     // красная: пульс > min
	SleepYellowMax float64 // жёлтая: сон < max (часы)
	MoodYellowMax  int     // жёлтая: настроение ≤ max
}

// AthleteThresholds возвращает пороги атлета (контракт §7).
func AthleteThresholds(hrvBase int) Thresholds {
	return Thresholds{
		HRVBase:        hrvBase,
		GreenMargin:    2,
		RedMargin:      5,
		RHRGreenMax:    60,
		RHRRedMin:      62,
		SleepYellowMax: 6.5,
		MoodYellowMax:  4,
	}
}

// WifeThresholds возвращает пороги жены (контракт §7).
func WifeThresholds(hrvBase int) Thresholds {
	return Thresholds{
		HRVBase:        hrvBase,
		GreenMargin:    2,
		RedMargin:      5,
		RHRGreenMax:    62,
		RHRRedMin:      65,
		SleepYellowMax: 7.0,
		MoodYellowMax:  4,
	}
}

// Input — утренние показатели (nil = нет данных).
type Input struct {
	HRV      *int
	RHR      *int
	SleepH   *float64
	Mood     *int
	KneePain *int // жена: боль в коленях 0–10
	CycleDay *int // жена: день МЦ
}

// Result — зона, причина и заметки (замены упражнений, фаза МЦ).
type Result struct {
	Zone   Zone
	Reason string
	Notes  []string
}

// Evaluate определяет зону готовности. Приоритет: 🔴 > 🟡 > 🟢 (FR-3.2).
func Evaluate(input Input, thresholds Thresholds) Result {
	notes := buildNotes(input)
	if thresholds.HRVBase <= 0 {
		return Result{Zone: ZoneUnknown, Reason: "не задана база HRV", Notes: notes}
	}

	// 🔴 — сначала опасные сигналы.
	if input.HRV != nil && *input.HRV < thresholds.HRVBase-thresholds.RedMargin {
		return Result{
			Zone:   ZoneRed,
			Reason: fmt.Sprintf("HRV %d мс ниже базы %d на %d мс (красная: порог −%d)", *input.HRV, thresholds.HRVBase, thresholds.HRVBase-*input.HRV, thresholds.RedMargin),
			Notes:  notes,
		}
	}
	if input.RHR != nil && *input.RHR > thresholds.RHRRedMin {
		return Result{
			Zone:   ZoneRed,
			Reason: fmt.Sprintf("пульс покоя %d > %d — красная зона", *input.RHR, thresholds.RHRRedMin),
			Notes:  notes,
		}
	}

	// 🟡 — снижаем нагрузку.
	if input.HRV != nil && *input.HRV < thresholds.HRVBase-thresholds.GreenMargin {
		return Result{
			Zone:   ZoneYellow,
			Reason: fmt.Sprintf("HRV %d мс ниже зелёной границы %d", *input.HRV, thresholds.HRVBase-thresholds.GreenMargin),
			Notes:  notes,
		}
	}
	if input.RHR != nil && *input.RHR > thresholds.RHRGreenMax && *input.RHR <= thresholds.RHRRedMin {
		return Result{
			Zone:   ZoneYellow,
			Reason: fmt.Sprintf("пульс покоя %d в жёлтой зоне (%d–%d)", *input.RHR, thresholds.RHRGreenMax+1, thresholds.RHRRedMin),
			Notes:  notes,
		}
	}
	if input.SleepH != nil && *input.SleepH < thresholds.SleepYellowMax {
		return Result{
			Zone:   ZoneYellow,
			Reason: fmt.Sprintf("сон %.2f ч < %.1f ч — недосып", *input.SleepH, thresholds.SleepYellowMax),
			Notes:  notes,
		}
	}
	if input.Mood != nil && *input.Mood <= thresholds.MoodYellowMax {
		return Result{
			Zone:   ZoneYellow,
			Reason: fmt.Sprintf("настроение %d/10 ≤ %d — контр-маркер", *input.Mood, thresholds.MoodYellowMax),
			Notes:  notes,
		}
	}

	// 🟢 — только при наличии HRV и пульса.
	switch {
	case input.HRV == nil && input.RHR == nil:
		return Result{Zone: ZoneUnknown, Reason: "нет HRV и пульса — зона не определена", Notes: notes}
	case input.HRV == nil:
		return Result{Zone: ZoneUnknown, Reason: "нет HRV — зона не определена", Notes: notes}
	case input.RHR == nil:
		return Result{Zone: ZoneUnknown, Reason: "нет пульса — зона не определена", Notes: notes}
	}

	return Result{
		Zone: ZoneGreen,
		Reason: fmt.Sprintf("HRV %d мс (≥ %d) и пульс %d (≤ %d) — по плану",
			*input.HRV, thresholds.HRVBase-thresholds.GreenMargin, *input.RHR, thresholds.RHRGreenMax),
		Notes: notes,
	}
}

func buildNotes(input Input) []string {
	var notes []string
	if input.KneePain != nil && *input.KneePain > 3 {
		notes = append(notes, fmt.Sprintf("колени %d/10 — убрать нагрузку на колени", *input.KneePain))
	}
	if input.CycleDay != nil && *input.CycleDay >= 1 && *input.CycleDay <= 5 {
		notes = append(notes, fmt.Sprintf("день МЦ %d — интенсивность ниже на 20–30%%", *input.CycleDay))
	}
	return notes
}
