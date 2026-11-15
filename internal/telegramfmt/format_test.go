package telegramfmt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/telegramfmt"
)

func TestFormatTableBecomesBullets(t *testing.T) {
	input := "**Доказано:**\n| Практика | Эффект | Примечание |\n|---|---|---|\n| Сауна 2–3×/нед | снижение давления | PMID: 33866630 |\n| Дыхание | ↑ HRV | PMID: 35623448 |"
	out := telegramfmt.Format(input)
	assert.NotContains(t, out, "|")
	assert.Contains(t, out, "<b>Доказано:</b>")
	assert.Contains(t, out, "• <b>Сауна 2–3×/нед</b> — снижение давления · PMID: 33866630")
	assert.Contains(t, out, "• <b>Дыхание</b> — ↑ HRV · PMID: 35623448")
}

func TestFormatHeadingsListsAndCode(t *testing.T) {
	input := "# Заголовок\n\n- один\n- два\n\n`сон 7:30` и **важно**\n\n```\nHRV < 30\n```"
	out := telegramfmt.Format(input)
	assert.Contains(t, out, "<b>Заголовок</b>")
	assert.Contains(t, out, "• один")
	assert.Contains(t, out, "<code>сон 7:30</code>")
	assert.Contains(t, out, "<b>важно</b>")
	assert.Contains(t, out, "<pre>HRV &lt; 30</pre>")
}

func TestFormatEscapesHTMLAndLinks(t *testing.T) {
	out := telegramfmt.Format(`HRV < 30 & пульс > 60 [ссылка](https://example.com)`)
	assert.Contains(t, out, "HRV &lt; 30 &amp; пульс &gt; 60")
	assert.Contains(t, out, `<a href="https://example.com">ссылка</a>`)
	assert.NotContains(t, out, "<script>")
}

func TestFormatPlainTextKeepsStructure(t *testing.T) {
	out := telegramfmt.Format("Сводка: атлет\nHRV_7: 31.0 мс\n\nСон: 7:25")
	assert.Contains(t, out, "Сводка: атлет")
	assert.Contains(t, out, "HRV_7: 31.0 мс\n\nСон: 7:25")
}
