package intake

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Парсер текстового чек-ина (FR-6.8.7): детерминированные правила — первый
// уровень, LLM — fallback для нестандартного текста. Так маленькая модель не
// теряет числа из «сон 7:33, HRV 38, пульс 58».
var (
	reSleepCompact = regexp.MustCompile(`(?i)(?:сон|спал[аи]?)\D{0,20}(\d{1,2})\s*[:.,]\s*(\d{2})`)
	reSleepWords   = regexp.MustCompile(`(?i)(?:сон|спал[аи]?)\D{0,20}(\d{1,2})\s*(?:ч|ч\.)\s*(\d{1,2})`)
	reSleepHours   = regexp.MustCompile(`(?i)(?:сон|спал[аи]?|длительность сна)\D{0,25}(\d{1,2})\s*(?:час[а-яё]*|ч)(?:\s*(\d{1,2})\s*(?:мин|м))?`)
	reHRV          = regexp.MustCompile(`(?i)(?:hrv|вср|вариабельн[а-яё]*|вариабильн[а-яё]*)\D{0,20}(\d{1,3})`)
	reRHR          = regexp.MustCompile(`(?i)(?:пульс|чсс)\s*(?:в\s+покое|покоя)?\D{0,20}(\d{2,3})`)
	reMood         = regexp.MustCompile(`(?i)настроени\w*\D{0,20}(\d{1,2})`)
	reAlcohol      = regexp.MustCompile(`(?i)алкогол\w*\D{0,20}(\d{1,2}(?:[.,]\d)?)`)
	reWeight       = regexp.MustCompile(`(?i)вес\w*\D{0,10}(\d{2,3}(?:[.,]\d)?)`)
	reSteps        = regexp.MustCompile(`(?i)шаг\w*\D{0,10}(\d{3,6})`)
	reCycle        = regexp.MustCompile(`(?i)(?:мц|цикл\w*)\D{0,10}(\d{1,2})`)
	reKnees        = regexp.MustCompile(`(?i)колен\w*\D{0,10}(\d{1,2})`)
	reCraving      = regexp.MustCompile(`(?i)(?:голод|тяга)\D{0,10}(\d{1,2})`)
	reWalk         = regexp.MustCompile(`(?i)(?:постпрандиальн\w*|прогулк\w* после еды)[^.\n]{0,30}`)
)

// parseCheckinText пробует извлечь чек-ин правилами. ok=false — данных нет,
// вызывающий код идёт в LLM.
func parseCheckinText(text string, now time.Time) (Extracted, bool) {
	checkin := &Checkin{}
	found := false

	if match := reSleepCompact.FindStringSubmatch(text); match != nil {
		if value, ok := sleepValue(match[1], match[2]); ok {
			checkin.SleepH = &value
			found = true
		}
	} else if match := reSleepWords.FindStringSubmatch(text); match != nil {
		if value, ok := sleepValue(match[1], match[2]); ok {
			checkin.SleepH = &value
			found = true
		}
	} else if match := reSleepHours.FindStringSubmatch(text); match != nil {
		if value, ok := sleepHoursValue(match[1], match[2]); ok {
			checkin.SleepH = &value
			found = true
		}
	}

	if value, ok := firstInt(reHRV, text, 10, 150); ok {
		checkin.HRV = &value
		found = true
	}
	if value, ok := firstInt(reRHR, text, 30, 120); ok {
		checkin.RHR = &value
		found = true
	}
	if value, ok := firstInt(reMood, text, 1, 10); ok {
		checkin.Mood = &value
		found = true
	}
	if value, ok := firstFloat(reAlcohol, text, 0, 20); ok {
		checkin.AlcoholDoses = &value
		found = true
	}
	if value, ok := firstFloat(reWeight, text, 30, 250); ok {
		checkin.WeightKG = &value
		found = true
	}
	if value, ok := firstInt(reSteps, text, 0, 200000); ok {
		checkin.Steps = &value
		found = true
	}
	if value, ok := firstInt(reCycle, text, 0, 60); ok {
		checkin.CycleDay = &value
		found = true
	}
	if value, ok := firstInt(reKnees, text, 0, 10); ok {
		checkin.KneesPain = &value
		found = true
	}
	if value, ok := firstInt(reCraving, text, 0, 10); ok {
		checkin.Craving = &value
		found = true
	}
	if match := reWalk.FindString(text); match != "" {
		lower := strings.ToLower(match)
		switch {
		case strings.Contains(lower, "нет") || strings.Contains(lower, "не было") || strings.Contains(lower, "не ходил"):
			value := false
			checkin.PostprandialWalk = &value
			found = true
		case strings.Contains(lower, "да") || strings.Contains(lower, "был") || strings.Contains(lower, "ходил"):
			value := true
			checkin.PostprandialWalk = &value
			found = true
		}
	}

	if !found {
		return Extracted{}, false
	}

	date := now.Format("2006-01-02")
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "позавчера"):
		date = now.AddDate(0, 0, -2).Format("2006-01-02")
	case strings.Contains(lower, "вчера"):
		date = now.AddDate(0, 0, -1).Format("2006-01-02")
	}

	return Extracted{
		Kinds:   []string{"checkin"},
		Date:    date,
		Checkin: checkin,
	}, true
}

func sleepHoursValue(hoursText, minutesText string) (float64, bool) {
	hours, err := strconv.Atoi(strings.TrimSpace(hoursText))
	if err != nil || hours < 0 || hours > 16 {
		return 0, false
	}
	minutes := 0
	if strings.TrimSpace(minutesText) != "" {
		parsed, errM := strconv.Atoi(strings.TrimSpace(minutesText))
		if errM != nil || parsed < 0 || parsed > 59 {
			return 0, false
		}
		minutes = parsed
	}
	return float64(hours) + float64(minutes)/60, true
}

func sleepValue(hoursText, minutesText string) (float64, bool) {
	hours, errH := strconv.Atoi(strings.TrimSpace(hoursText))
	minutes, errM := strconv.Atoi(strings.TrimSpace(minutesText))
	if errH != nil || errM != nil || hours < 0 || hours > 16 || minutes < 0 || minutes > 59 {
		return 0, false
	}
	return float64(hours) + float64(minutes)/60, true
}

func firstInt(pattern *regexp.Regexp, text string, minValue, maxValue int) (int, bool) {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value < minValue || value > maxValue {
		return 0, false
	}
	return value, true
}

func firstFloat(pattern *regexp.Regexp, text string, minValue, maxValue float64) (float64, bool) {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64)
	if err != nil || value < minValue || value > maxValue {
		return 0, false
	}
	return value, true
}
