package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/kb"
	"healthbot/internal/tools"
)

func TestKnowledgeSearchTool(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "исследования", "сон.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("# Сон\n\n## HRV\n\nСон и вариабельность ЧСС. PMID: 12345678.\n"), 0o600))
	index, err := kb.Load(root)
	require.NoError(t, err)

	tool := tools.NewKnowledgeSearch(index)
	output, err := tool.Execute(context.Background(), []byte(`{"query":"сон и HRV","person":"athlete","limit":3}`))
	require.NoError(t, err)
	assert.Contains(t, output, "сон.md")
	assert.Contains(t, output, "12345678")

	empty, err := tool.Execute(context.Background(), []byte(`{"query":"квантовая хромодинамика","person":"athlete"}`))
	require.NoError(t, err)
	assert.Contains(t, empty, "ничего не найдено")
}

func TestKnowledgeSearchNilIndex(t *testing.T) {
	tool := tools.NewKnowledgeSearch(nil)
	output, err := tool.Execute(context.Background(), []byte(`{"query":"сон","person":"athlete"}`))
	require.NoError(t, err)
	assert.Contains(t, output, "не загружена")
}
