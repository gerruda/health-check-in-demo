package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeKnowledgeAnswerRemovesInventedCitations(t *testing.T) {
	context := "- исследования/сон.md — Сон / HRV (PMID: 12345678)\n  Сон и HRV."
	answer := "Вывод по сну. [Источник](https://fake.example/paper) DOI 10.9999/fake PMID: 99999999. Также PMID: 12345678."
	clean := sanitizeKnowledgeAnswer(answer, context)

	assert.NotContains(t, clean, "fake.example")
	assert.NotContains(t, clean, "10.9999/fake")
	assert.NotContains(t, clean, "99999999")
	assert.Contains(t, clean, "12345678")
	assert.Contains(t, clean, "Источники:")
	assert.Contains(t, clean, "исследования/сон.md")
	assert.NotContains(t, clean, "https://")
}

func TestWithoutTool(t *testing.T) {
	items := []Tool{{Name: "a"}, {Name: "search_knowledge"}, {Name: "b"}}
	filtered := withoutTool(items, "search_knowledge")
	assert.Len(t, filtered, 2)
	for _, item := range filtered {
		assert.NotEqual(t, "search_knowledge", item.Name)
	}
}

func TestSanitizeKeepsLineBreaks(t *testing.T) {
	context := "- исследования/сон.md — Сон / HRV (PMID: 12345678)\n  Сон и HRV."
	answer := "Первый абзац.\n\n- пункт один\n- пункт два https://fake.example PMID: 99999999"
	clean := sanitizeKnowledgeAnswer(answer, context)
	assert.Contains(t, clean, "Первый абзац.")
	assert.Contains(t, clean, "\n\n- пункт один")
	assert.Contains(t, clean, "\n- пункт два")
	assert.NotContains(t, clean, "fake.example")
	assert.NotContains(t, clean, "99999999")
	assert.Contains(t, clean, "12345678")
}
