package analysis_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/analysis"
)

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func athleteInput(hrv, rhr int) analysis.Input {
	return analysis.Input{HRV: intPtr(hrv), RHR: intPtr(rhr)}
}

func TestZoneAthlete(t *testing.T) {
	thresholds := analysis.AthleteThresholds(33)

	tests := []struct {
		name      string
		input     analysis.Input
		want      analysis.Zone
		reasonHas string
	}{
		{
			name:      "U3 красная: HRV 27, пульс 68",
			input:     athleteInput(27, 68),
			want:      analysis.ZoneRed,
			reasonHas: "hrv",
		},
		{
			name:  "U4 зелёная: HRV 35, пульс 57",
			input: athleteInput(35, 57),
			want:  analysis.ZoneGreen,
		},
		{
			name: "U5 жёлтая: HRV 31, пульс 58, сон 6:00",
			input: analysis.Input{
				HRV: intPtr(31), RHR: intPtr(58), SleepH: floatPtr(6.0),
			},
			want:      analysis.ZoneYellow,
			reasonHas: "сон",
		},
		{
			name:  "HRV ниже базы на 3 — жёлтая",
			input: athleteInput(30, 58),
			want:  analysis.ZoneYellow,
		},
		{
			name:  "HRV ровно база−5 — ещё не красная",
			input: athleteInput(28, 58),
			want:  analysis.ZoneYellow,
		},
		{
			name:  "HRV база−6 — красная",
			input: athleteInput(27, 55),
			want:  analysis.ZoneRed,
		},
		{
			name:  "пульс 61 — жёлтая",
			input: athleteInput(35, 61),
			want:  analysis.ZoneYellow,
		},
		{
			name:  "пульс 63 — красная",
			input: athleteInput(35, 63),
			want:  analysis.ZoneRed,
		},
		{
			name:  "сон 6:00 — жёлтая",
			input: analysis.Input{HRV: intPtr(35), RHR: intPtr(57), SleepH: floatPtr(6.0)},
			want:  analysis.ZoneYellow,
		},
		{
			name:  "настроение 4 — жёлтая",
			input: analysis.Input{HRV: intPtr(35), RHR: intPtr(57), Mood: intPtr(4)},
			want:  analysis.ZoneYellow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analysis.Evaluate(tt.input, thresholds)
			assert.Equal(t, tt.want, result.Zone)
			if tt.reasonHas != "" {
				assert.Contains(t, strings.ToLower(result.Reason), tt.reasonHas,
					"причина: %q", result.Reason)
			}
		})
	}
}

func TestZoneWife(t *testing.T) {
	thresholds := analysis.WifeThresholds(34)

	assert.Equal(t, analysis.ZoneGreen, analysis.Evaluate(athleteInput(35, 62), thresholds).Zone)
	assert.Equal(t, analysis.ZoneYellow, analysis.Evaluate(athleteInput(35, 63), thresholds).Zone)
	assert.Equal(t, analysis.ZoneRed, analysis.Evaluate(athleteInput(35, 66), thresholds).Zone)
	assert.Equal(t, analysis.ZoneRed, analysis.Evaluate(athleteInput(28, 60), thresholds).Zone, "HRV < база−5")
	assert.Equal(t, analysis.ZoneYellow, analysis.Evaluate(analysis.Input{
		HRV: intPtr(35), RHR: intPtr(60), SleepH: floatPtr(6.5),
	}, thresholds).Zone, "сон <7 ч — жёлтая")

	withKnees := analysis.Evaluate(analysis.Input{
		HRV: intPtr(35), RHR: intPtr(60), KneePain: intPtr(5), CycleDay: intPtr(3),
	}, thresholds)
	assert.Equal(t, analysis.ZoneGreen, withKnees.Zone)
	assert.Contains(t, strings.Join(withKnees.Notes, " "), "колени")
	assert.Contains(t, strings.Join(withKnees.Notes, " "), "МЦ")
}

func TestZoneUnknown(t *testing.T) {
	thresholds := analysis.AthleteThresholds(33)

	result := analysis.Evaluate(analysis.Input{}, thresholds)
	assert.Equal(t, analysis.ZoneUnknown, result.Zone)

	result = analysis.Evaluate(analysis.Input{RHR: intPtr(58)}, thresholds)
	assert.Equal(t, analysis.ZoneUnknown, result.Zone)
	assert.Contains(t, strings.ToLower(result.Reason), "hrv")
}

func TestThresholdDefaults(t *testing.T) {
	athlete := analysis.AthleteThresholds(33)
	assert.Equal(t, 33, athlete.HRVBase)
	assert.Equal(t, 2, athlete.GreenMargin)
	assert.Equal(t, 5, athlete.RedMargin)
	assert.Equal(t, 60, athlete.RHRGreenMax)
	assert.Equal(t, 62, athlete.RHRRedMin)
	assert.InDelta(t, 6.5, athlete.SleepYellowMax, 0.001)

	wife := analysis.WifeThresholds(34)
	assert.Equal(t, 62, wife.RHRGreenMax)
	assert.Equal(t, 65, wife.RHRRedMin)
	assert.InDelta(t, 7.0, wife.SleepYellowMax, 0.001)
}
