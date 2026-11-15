package kb_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/kb"
	"healthbot/internal/nlu"
)

type fakeEmbedder struct {
	calls int
	fail  bool
}

func (f *fakeEmbedder) Name() string { return "fake/model" }

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	f.calls++
	if f.fail {
		return nil, assert.AnError
	}
	vectors := make([][]float64, len(texts))
	for index, text := range texts {
		lower := strings.ToLower(text)
		switch {
		case strings.Contains(lower, "сон") || strings.Contains(lower, "восстановл"):
			vectors[index] = []float64{1, 0}
		case strings.Contains(lower, "белок") || strings.Contains(lower, "протеин"):
			vectors[index] = []float64{0, 1}
		default:
			vectors[index] = []float64{0.2, 0.2}
		}
	}
	return vectors, nil
}

func writeHybridKB(t *testing.T, root string) {
	t.Helper()
	writeKB(t, root, "исследования/сон.md", "# Сон и отдых\n\n## Восстановление\n\nНочной сон и отдых важны.\n")
	writeKB(t, root, "методики/белок.md", "# Белок\n\n## Норма\n\nПротеин 1.6 г/кг.\n")
	writeKB(t, root, "методики/калории.md", "# Калории\n\n## Дефицит\n\nСчитаем ккал.\n")
}

func TestHybridFindsSemanticMatchWithoutLexical(t *testing.T) {
	root := t.TempDir()
	writeHybridKB(t, root)
	embedder := &fakeEmbedder{}
	index, err := kb.LoadWithEmbeddings(context.Background(), root, "", embedder)
	require.NoError(t, err)
	require.True(t, index.HasEmbeddings())

	// «восстановление» лексически не встречается в сон.md (там «отдых»),
	// но embedding-вектор совпадает — гибрид должен поднять сон.
	results := index.SearchHybrid(context.Background(), "восстановление", nlu.Athlete, 2)
	require.NotEmpty(t, results)
	assert.Contains(t, results[0].Path, "сон")
}

func TestEmbeddingCacheAvoidsRecompute(t *testing.T) {
	root := t.TempDir()
	writeHybridKB(t, root)
	cachePath := t.TempDir() + "/embeddings.json"
	first := &fakeEmbedder{}
	_, err := kb.LoadWithEmbeddings(context.Background(), root, cachePath, first)
	require.NoError(t, err)
	require.Positive(t, first.calls)

	second := &fakeEmbedder{}
	_, err = kb.LoadWithEmbeddings(context.Background(), root, cachePath, second)
	require.NoError(t, err)
	assert.Zero(t, second.calls, "кэш должен исключить повторные embeddings")
}

func TestHybridFallsBackToLexicalOnEmbedError(t *testing.T) {
	root := t.TempDir()
	writeHybridKB(t, root)
	_, err := kb.LoadWithEmbeddings(context.Background(), root, "", &fakeEmbedder{fail: true})
	require.Error(t, err)

	index, err := kb.Load(root)
	require.NoError(t, err)
	results := index.SearchHybrid(context.Background(), "белок", nlu.Athlete, 2)
	require.NotEmpty(t, results)
	assert.Contains(t, results[0].Path, "белок")
}
