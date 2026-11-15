// Package kb — локальный поиск по базе знаний проекта (Markdown-канон).
// Никаких внешних вызовов: индекс строится из база_знаний/ при старте.
package kb

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"healthbot/internal/nlu"
)

// Result — найденный фрагмент базы знаний.
type Result struct {
	Path     string
	Title    string
	Heading  string
	Category string
	Snippet  string
	PMIDs    []string
	Score    float64
}

type chunk struct {
	path     string
	title    string
	heading  string
	category string
	text     string
	pmids    []string
	terms    map[string]int
	length   int
}

// Index — in-memory BM25-индекс по чанкам базы знаний.
type Index struct {
	chunks   []chunk
	idf      map[string]float64
	avgLen   float64
	embedder Embedder
	vectors  [][]float64
}

var (
	pmidPattern = regexp.MustCompile(`(?i)\bPMID\s*[:#]?\s*(\d{7,9})\b`)
	wordPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)
)

// Load читает все markdown-файлы из root (база_знаний) и строит индекс.
func Load(root string) (*Index, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("kb: пустой путь базы знаний")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("kb: база знаний %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("kb: %q не каталог", root)
	}

	index := &Index{idf: make(map[string]float64)}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			relative = path
		}
		data, readErr := fs.ReadFile(os.DirFS(root), filepath.ToSlash(relative))
		if readErr != nil {
			return readErr
		}
		category := strings.Split(filepath.ToSlash(relative), "/")[0]
		index.addFile(relative, category, string(data))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("kb: обход базы знаний: %w", err)
	}

	totalLength := 0
	for _, item := range index.chunks {
		totalLength += item.length
	}
	if len(index.chunks) > 0 {
		index.avgLen = float64(totalLength) / float64(len(index.chunks))
	}
	for term, count := range index.documentFrequency() {
		n := float64(len(index.chunks))
		index.idf[term] = math.Log(1 + (n-float64(count)+0.5)/(float64(count)+0.5))
	}
	return index, nil
}

// Size — количество чанков (для логов/тестов).
func (i *Index) Size() int {
	if i == nil {
		return 0
	}
	return len(i.chunks)
}

func (i *Index) addFile(relative, category, content string) {
	title := ""
	heading := ""
	var buffer []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(buffer, "\n"))
		if text != "" {
			i.chunks = append(i.chunks, makeChunk(relative, title, heading, category, text))
		}
		buffer = buffer[:0]
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			level := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
			headingText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if level == 1 && title == "" {
				title = headingText
				flush()
				heading = headingText
				buffer = append(buffer, line)
				continue
			}
			flush()
			heading = headingText
			buffer = append(buffer, line)
			continue
		}
		buffer = append(buffer, line)
	}
	flush()
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(relative), filepath.Ext(relative))
	}
}

func makeChunk(path, title, heading, category, text string) chunk {
	terms := tokenize(text)
	frequencies := make(map[string]int, len(terms))
	for _, term := range terms {
		frequencies[term]++
	}
	pmids := make([]string, 0, 2)
	for _, match := range pmidPattern.FindAllStringSubmatch(text, -1) {
		pmids = append(pmids, match[1])
	}
	return chunk{
		path: path, title: title, heading: heading, category: category,
		text: text, pmids: unique(pmids), terms: frequencies, length: len(terms),
	}
}

func (i *Index) documentFrequency() map[string]int {
	frequency := make(map[string]int, len(i.idf))
	for _, item := range i.chunks {
		for term := range item.terms {
			frequency[term]++
		}
	}
	return frequency
}

// Search ищет top-limit чанков под запрос. person влияет на приоритет раздела
// «жена», чтобы ответы не смешивали женский и общий контекст.
func (i *Index) Search(query string, person nlu.Person, limit int) []Result {
	if i == nil || len(i.chunks) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 4
	}
	terms := expandQuery(query)
	if len(terms) == 0 {
		return nil
	}

	results := make([]Result, 0, len(i.chunks))
	for _, item := range i.chunks {
		score := 0.0
		matched := make([]string, 0, len(terms))
		for _, term := range terms {
			tf := item.terms[term]
			if tf == 0 {
				continue
			}
			matched = append(matched, term)
			score += i.idf[term] * float64(tf) * 2.2 / (float64(tf) + 1.2*(1-0.75+0.75*float64(item.length)/math.Max(i.avgLen, 1)))
		}
		if score == 0 {
			continue
		}
		if person == nlu.Wife && item.category == "жена" {
			score *= 1.25
		}
		if person == nlu.Athlete && item.category == "жена" {
			score *= 0.9
		}
		results = append(results, Result{
			Path: item.path, Title: item.title, Heading: item.heading, Category: item.category,
			Snippet: snippet(item.text, matched, 420), PMIDs: item.pmids, Score: score,
		})
	}
	sortResults(results, limit)
	return results
}

func sortResults(results []Result, limit int) {
	sort.SliceStable(results, func(left, right int) bool {
		if results[left].Score == results[right].Score {
			return results[left].Path < results[right].Path
		}
		return results[left].Score > results[right].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		dot += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func snippet(text string, terms []string, limit int) string {
	lower := strings.ToLower(text)
	position := -1
	for _, term := range terms {
		if index := strings.Index(lower, term); index >= 0 && (position < 0 || index < position) {
			position = index
		}
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return strings.TrimSpace(text)
	}
	runePosition := 0
	if position > 0 {
		runePosition = len([]rune(lower[:position]))
	}
	start := runePosition - limit/3
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(runes) {
		end = len(runes)
	}
	value := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		value = "…" + value
	}
	if end < len(runes) {
		value += "…"
	}
	return value
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

var synonymGroups = [][]string{
	{"hrv", "вср", "вариабельность", "вариабильность", "вариабельност"},
	{"пульс", "чсс", "rhr", "пульса", "пульс"},
	{"сон", "сна", "sleep", "сонливость"},
	{"белок", "протеин", "protein", "белка"},
	{"жир", "жиров", "жиросжигание", "похудение", "вес", "масса"},
	{"калори", "ккал", "дефицит", "калорийность"},
	{"тренировк", "силов", "нагрузк", "упражнен"},
	{"колен", "колено", "колени"},
	{"плеч", "плечо", "плечи"},
	{"алкогол", "спирт", "питье"},
	{"стресс", "нервн", "выгоран"},
	{"восстанов", "отдых", "перегруз", "overtraining"},
	{"добавк", "креатин", "омега", "магний", "витамин"},
	{"инсулин", "глюкоз", "homa", "диабет", "инсулинорезистент"},
	{"masld", "печен", "жировая", "алт", "аст", "ггт"},
	{"жим", "жиме", "жима"},
	{"присед", "приседания", "приседе"},
	{"кардио", "лестниц", "бег", "ходьб"},
}

var variants = func() map[string]string {
	result := make(map[string]string)
	for groupIndex, group := range synonymGroups {
		key := fmt.Sprintf("g%d", groupIndex)
		for _, word := range group {
			result[normalize(word)] = key
		}
	}
	return result
}()

func expandQuery(query string) []string {
	base := tokenize(query)
	seen := make(map[string]struct{}, len(base)*2)
	result := make([]string, 0, len(base)*2)
	for _, term := range base {
		if _, ok := seen[term]; !ok {
			seen[term] = struct{}{}
			result = append(result, term)
		}
		if group, ok := variants[term]; ok {
			for _, candidate := range synonymGroups[groupNumber(group)] {
				candidate = normalize(candidate)
				if _, exists := seen[candidate]; !exists {
					seen[candidate] = struct{}{}
					result = append(result, candidate)
				}
			}
		}
	}
	return result
}

func groupNumber(key string) int {
	var number int
	_, _ = fmt.Sscanf(key, "g%d", &number)
	return number
}

func tokenize(text string) []string {
	words := wordPattern.FindAllString(strings.ToLower(text), -1)
	result := make([]string, 0, len(words))
	for _, word := range words {
		if len([]rune(word)) < 2 {
			continue
		}
		result = append(result, normalize(word))
	}
	return result
}

func normalize(word string) string {
	word = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, strings.ToLower(strings.TrimSpace(word)))
	if len([]rune(word)) <= 3 {
		return word
	}
	for _, suffix := range []string{"иями", "ями", "ами", "ов", "ев", "ий", "ый", "ой", "ая", "ое", "ые", "их", "ых", "ам", "ям", "ах", "ях", "ом", "ем", "ии", "ия", "ью", "у", "ю", "а", "я", "ы", "и", "е", "о", "ь"} {
		if strings.HasSuffix(word, suffix) && len([]rune(word))-len([]rune(suffix)) >= 3 {
			return strings.TrimSuffix(word, suffix)
		}
	}
	return word
}
