// Package nlu содержит модель данных домена (ТЗ §6): чек-ины, тренировки,
// питание, замеры. Пакет без сети и файловой системы — только структуры.
package nlu

// Person — чей это набор данных (ТЗ §2).
type Person string

// Люди проекта.
const (
	Athlete Person = "athlete"
	Wife    Person = "wife"
)

// Valid сообщает, известен ли person.
func (p Person) Valid() bool {
	return p == Athlete || p == Wife
}

// Checkin — ежедневный чек-ин (ТЗ §6).
type Checkin struct {
	Date             string   `json:"date"`
	Person           Person   `json:"person"`
	SleepH           *float64 `json:"sleep_h,omitempty"`
	SleepQuality     *int     `json:"sleep_quality,omitempty"`
	HRV              *int     `json:"hrv,omitempty"`
	RHR              *int     `json:"rhr,omitempty"`
	Mood             *int     `json:"mood,omitempty"`
	AlcoholDoses     *float64 `json:"alcohol_doses,omitempty"`
	AlcoholNote      string   `json:"alcohol_note,omitempty"`
	PlannedTraining  string   `json:"planned_training,omitempty"`
	CycleDay         *int     `json:"cycle_day,omitempty"`
	CyclePhase       string   `json:"cycle_phase,omitempty"`
	KneesPain        *int     `json:"knees_pain,omitempty"`
	Craving          *int     `json:"craving,omitempty"`
	PostprandialWalk *bool    `json:"postprandial_walk,omitempty"`
	WeightKG         *float64 `json:"weight_kg,omitempty"`
	Steps            *int     `json:"steps,omitempty"`
	Context          string   `json:"context,omitempty"`
}

// Exercise — упражнение в отчёте о тренировке.
type Exercise struct {
	Name     string   `json:"name"`
	WeightKG *float64 `json:"weight_kg,omitempty"`
	Sets     *int     `json:"sets,omitempty"`
	Reps     *int     `json:"reps,omitempty"`
	RPE      *int     `json:"rpe,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// Training — тренировка или пропуск (ТЗ §6, FR-2.7).
type Training struct {
	Date        string         `json:"date"`
	Person      Person         `json:"person"`
	Kind        string         `json:"kind"`
	DurationMin *int           `json:"duration_min,omitempty"`
	AvgHR       *int           `json:"avg_hr,omitempty"`
	Calories    *int           `json:"calories,omitempty"`
	RPESession  *int           `json:"rpe_session,omitempty"`
	Exercises   []Exercise     `json:"exercises"`
	Cardio      string         `json:"cardio,omitempty"`
	Felt        string         `json:"felt,omitempty"`
	Pain        map[string]int `json:"pain,omitempty"`
	Done        bool           `json:"done"`
}

// Nutrition — питание за день (ТЗ §6): 4 числа.
type Nutrition struct {
	Date        string `json:"date"`
	Person      Person `json:"person"`
	Kcal        *int   `json:"kcal,omitempty"`
	ProteinG    *int   `json:"protein_g,omitempty"`
	FatG        *int   `json:"fat_g,omitempty"`
	CarbsG      *int   `json:"carbs_g,omitempty"`
	Description string `json:"description,omitempty"`
}

// Measurement — недельный замер (ТЗ §6).
type Measurement struct {
	Date      string             `json:"date"`
	Person    Person             `json:"person"`
	WeightKG  *float64           `json:"weight_kg,omitempty"`
	FatPct    *float64           `json:"fat_pct,omitempty"`
	MusclePct *float64           `json:"muscle_pct,omitempty"`
	Visceral  *int               `json:"visceral,omitempty"`
	WaterPct  *float64           `json:"water_pct,omitempty"`
	WaistCM   *float64           `json:"waist_cm,omitempty"`
	Tape      map[string]float64 `json:"tape,omitempty"`
	Flagship  *float64           `json:"flagship,omitempty"`
}
