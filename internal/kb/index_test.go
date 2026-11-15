package kb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/kb"
	"healthbot/internal/nlu"
)

func writeKB(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestSearchFindsRussianSynonymsAndPMID(t *testing.T) {
	root := t.TempDir()
	writeKB(t, root, "исследования/сон.md", `# Сон и восстановление

## HRV

Низкая вариабельность ЧСС и короткий сон связаны с перегрузом. PMID: 12345678.

## Практика

Ложиться в одно время, смотреть на утренний пульс.
`)
	writeKB(t, root, "методики/калории.md", `# Расчет калорий

## Белок

Протеин 1.6–2.0 г/кг тощей массы при дефиците калорий.
`)
	writeKB(t, root, "жена/железо.md", `# Железо

## Дефицит

Ферритин и железо — обсудить с врачом.
`)

	index, err := kb.Load(root)
	require.NoError(t, err)
	require.GreaterOrEqual(t, index.Size(), 3)

	results := index.Search("вариабельность чсс и сон", nlu.Athlete, 3)
	require.NotEmpty(t, results)
	assert.Contains(t, results[0].Path, "сон")
	require.NotEmpty(t, results[0].PMIDs)
	assert.Equal(t, "12345678", results[0].PMIDs[0])
	assert.NotEmpty(t, results[0].Snippet)

	protein := index.Search("сколько белка при дефиците", nlu.Athlete, 2)
	require.NotEmpty(t, protein)
	assert.Contains(t, protein[0].Path, "калории")
}

func TestWifeCategoryBoost(t *testing.T) {
	root := t.TempDir()
	writeKB(t, root, "жена/железо.md", "# Железо\n\n## Дефицит\n\nФерритин и железо.\n")
	writeKB(t, root, "исследования/железо.md", "# Железо в исследованиях\n\n## Общее\n\nЖелезо и ферритин.\n")
	index, err := kb.Load(root)
	require.NoError(t, err)
	results := index.Search("железо ферритин", nlu.Wife, 2)
	require.NotEmpty(t, results)
	assert.Contains(t, results[0].Path, "жена")
}

func TestSearchNoMatch(t *testing.T) {
	root := t.TempDir()
	writeKB(t, root, "методики/сон.md", "# Сон\n\n## Норма\n\nСемь часов.\n")
	index, err := kb.Load(root)
	require.NoError(t, err)
	assert.Empty(t, index.Search("квантовая хромодинамика", nlu.Athlete, 3))
}

func TestLoadMissingRoot(t *testing.T) {
	_, err := kb.Load(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}
