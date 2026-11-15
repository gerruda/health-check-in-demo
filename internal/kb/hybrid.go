package kb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"healthbot/internal/nlu"
)

// Embedder — источник embeddings (реализация — internal/embeddings.Ollama).
type Embedder interface {
	Name() string
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

type embeddingCache struct {
	Model   string               `json:"model"`
	Entries map[string][]float64 `json:"entries"`
}

// LoadWithEmbeddings строит BM25-индекс + embeddings чанков с дисковым кэшем.
// При ошибке embeddings вызывающий код должен откатиться на Load (лексика).
func LoadWithEmbeddings(ctx context.Context, root, cachePath string, embedder Embedder) (*Index, error) {
	index, err := Load(root)
	if err != nil {
		return nil, err
	}
	if embedder == nil || len(index.chunks) == 0 {
		return index, nil
	}

	cache := embeddingCache{Model: embedder.Name(), Entries: make(map[string][]float64)}
	if cachePath != "" {
		if data, readErr := os.ReadFile(cachePath); readErr == nil { // #nosec G304 -- путь задаёт приложение (DATA_ROOT)
			var loaded embeddingCache
			if json.Unmarshal(data, &loaded) == nil && loaded.Model == cache.Model && loaded.Entries != nil {
				cache = loaded
			}
		}
	}

	keys := make([]string, len(index.chunks))
	missingTexts := make([]string, 0, len(index.chunks))
	missingPositions := make([]int, 0, len(index.chunks))
	for position, item := range index.chunks {
		keys[position] = chunkCacheKey(item)
		if _, ok := cache.Entries[keys[position]]; !ok {
			missingTexts = append(missingTexts, item.text)
			missingPositions = append(missingPositions, position)
		}
	}
	if len(missingTexts) > 0 {
		vectors, embedErr := embedder.Embed(ctx, missingTexts)
		if embedErr != nil {
			return nil, fmt.Errorf("kb: embeddings: %w", embedErr)
		}
		if len(vectors) != len(missingTexts) {
			return nil, fmt.Errorf("kb: embeddings: получено %d векторов на %d текстов", len(vectors), len(missingTexts))
		}
		for offset, position := range missingPositions {
			cache.Entries[keys[position]] = vectors[offset]
		}
		if cachePath != "" {
			if saveErr := saveEmbeddingCache(cachePath, cache); saveErr != nil {
				return nil, saveErr
			}
		}
	}

	index.embedder = embedder
	index.vectors = make([][]float64, len(index.chunks))
	for position := range index.chunks {
		index.vectors[position] = cache.Entries[keys[position]]
	}
	return index, nil
}

// HasEmbeddings сообщает, загружены ли вектора (для логов/тестов).
func (i *Index) HasEmbeddings() bool {
	return i != nil && len(i.vectors) == len(i.chunks) && len(i.vectors) > 0
}

func chunkCacheKey(item chunk) string {
	hash := sha256.Sum256([]byte(item.path + "\x00" + item.heading + "\x00" + item.text))
	return item.path + "|" + item.heading + "|" + hex.EncodeToString(hash[:])
}

func saveEmbeddingCache(path string, cache embeddingCache) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("kb: создать каталог кэша embeddings: %w", err)
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("kb: сериализовать кэш embeddings: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".kb-embeddings-*")
	if err != nil {
		return fmt.Errorf("kb: временный файл кэша: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("kb: записать кэш embeddings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("kb: закрыть кэш embeddings: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("kb: переименовать кэш embeddings: %w", err)
	}
	return nil
}

// SearchHybrid ищет top-limit: BM25 + cosine similarity.
// Если embeddings/вектора недоступны — обычный лексический Search.
func (i *Index) SearchHybrid(ctx context.Context, query string, person nlu.Person, limit int) []Result {
	if i == nil || !i.HasEmbeddings() || i.embedder == nil {
		return i.Search(query, person, limit)
	}
	terms := expandQuery(query)
	if len(terms) == 0 {
		return nil
	}
	queryVectors, err := i.embedder.Embed(ctx, []string{query})
	if err != nil || len(queryVectors) != 1 {
		return i.Search(query, person, limit)
	}

	lexical := make([]float64, len(i.chunks))
	maxLexical := 0.0
	for position, item := range i.chunks {
		score := 0.0
		for _, term := range terms {
			tf := item.terms[term]
			if tf == 0 {
				continue
			}
			score += i.idf[term] * float64(tf) * 2.2 / (float64(tf) + 1.2*(1-0.75+0.75*float64(item.length)/math.Max(i.avgLen, 1)))
		}
		lexical[position] = score
		if score > maxLexical {
			maxLexical = score
		}
	}

	results := make([]Result, 0, len(i.chunks))
	for position, item := range i.chunks {
		lexicalNorm := 0.0
		if maxLexical > 0 {
			lexicalNorm = lexical[position] / maxLexical
		}
		cosine := cosineSimilarity(queryVectors[0], i.vectors[position])
		score := 0.55*lexicalNorm + 0.45*(cosine+1)/2
		if score <= 0 {
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
			Snippet: snippet(item.text, terms, 420), PMIDs: item.pmids, Score: score,
		})
	}
	sortResults(results, limit)
	return results
}
