package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"healthbot/internal/metrics"
	"healthbot/internal/nlu"
)

var tapeLabels = map[string]string{
	"chest": "Грудь", "biceps": "Бицепс", "hips": "Ягодицы", "thigh": "Бедро", "calf": "Икра",
}

var tapeKeys = func() map[string]string {
	result := make(map[string]string, len(tapeLabels))
	for key, label := range tapeLabels {
		result[label] = key
	}
	return result
}()

// WriteMeasurement сохраняет/мержит недельный замер (FR-2.4).
func (w *Writer) WriteMeasurement(measurement nlu.Measurement) (path string, err error) {
	defer func() { metrics.Default().ObserveSave("measurement", saveStatus(err)) }()
	date, err := parseDate(measurement.Date)
	if err != nil {
		return "", err
	}
	path, err = w.paths.MeasurementFile(measurement.Person, date)
	if err != nil {
		return "", err
	}
	if err := w.writeMerged(path, renderMeasurement(measurement)); err != nil {
		return "", err
	}
	w.invalidate(measurement.Person)
	return path, nil
}

func renderMeasurement(measurement nlu.Measurement) document {
	doc := document{header: "# Замер " + measurement.Date}
	body := documentSection{title: "Состав тела"}
	if measurement.WeightKG != nil {
		body.lines = append(body.lines, fmt.Sprintf("- Вес: %.1f кг", *measurement.WeightKG))
	}
	if measurement.FatPct != nil {
		body.lines = append(body.lines, fmt.Sprintf("- Жир: %.1f%%", *measurement.FatPct))
	}
	if measurement.MusclePct != nil {
		body.lines = append(body.lines, fmt.Sprintf("- Мышцы: %.1f%%", *measurement.MusclePct))
	}
	if measurement.Visceral != nil {
		body.lines = append(body.lines, fmt.Sprintf("- Висцеральный индекс: %d", *measurement.Visceral))
	}
	if measurement.WaterPct != nil {
		body.lines = append(body.lines, fmt.Sprintf("- Вода: %.1f%%", *measurement.WaterPct))
	}
	if len(body.lines) > 0 {
		doc.sections = append(doc.sections, body)
	}

	volumes := documentSection{title: "Объёмы"}
	if measurement.WaistCM != nil {
		volumes.lines = append(volumes.lines, fmt.Sprintf("- Талия: %.1f см", *measurement.WaistCM))
	}
	keys := make([]string, 0, len(measurement.Tape))
	for key := range measurement.Tape {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		label := tapeLabels[key]
		if label == "" {
			label = key
		}
		volumes.lines = append(volumes.lines, fmt.Sprintf("- %s: %.1f см", label, measurement.Tape[key]))
	}
	if len(volumes.lines) > 0 {
		doc.sections = append(doc.sections, volumes)
	}

	if measurement.Flagship != nil {
		doc.sections = append(doc.sections, documentSection{
			title: "Флагман",
			lines: []string{fmt.Sprintf("- Бросок медбола: %.1f", *measurement.Flagship)},
		})
	}
	return doc
}

// Measurements возвращает замеры по возрастанию даты (limit — последние N).
func (r *Reader) Measurements(person nlu.Person, limit int) ([]nlu.Measurement, error) {
	dir, err := r.paths.DataDir(person)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, measurementsDirName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: прочитать замеры: %w", err)
	}
	measurements := make([]nlu.Measurement, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		date, err := time.Parse("2006-01-02", strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, measurementsDirName, entry.Name())) // #nosec G304 -- путь из ReadDir
		if err != nil {
			return nil, fmt.Errorf("store: прочитать замер %q: %w", entry.Name(), err)
		}
		measurement := nlu.Measurement{Date: date.Format("2006-01-02"), Person: person, Tape: map[string]float64{}}
		parseMeasurement(string(data), &measurement)
		measurements = append(measurements, measurement)
	}
	sort.SliceStable(measurements, func(left, right int) bool { return measurements[left].Date < measurements[right].Date })
	if limit > 0 && len(measurements) > limit {
		measurements = measurements[len(measurements)-limit:]
	}
	return measurements, nil
}

func parseMeasurement(content string, measurement *nlu.Measurement) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		if line == "" {
			continue
		}
		label, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value := parseMeasurementValue(raw)
		if value == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "вес":
			measurement.WeightKG = value
		case "жир":
			measurement.FatPct = value
		case "мышцы":
			measurement.MusclePct = value
		case "вода":
			measurement.WaterPct = value
		case "талия":
			measurement.WaistCM = value
		case "висцеральный индекс":
			parsed := int(*value)
			measurement.Visceral = &parsed
		case "бросок медбола":
			measurement.Flagship = value
		default:
			if key, ok := tapeKeys[strings.TrimSpace(label)]; ok {
				measurement.Tape[key] = *value
			}
		}
	}
}

func parseMeasurementValue(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, " кг")
	raw = strings.TrimSuffix(raw, " см")
	raw = strings.TrimSuffix(raw, "%")
	raw = strings.ReplaceAll(raw, ",", ".")
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return nil
	}
	return &value
}
