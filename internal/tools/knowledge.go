package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"healthbot/internal/kb"
)

// NameSearchKnowledge — имя инструмента поиска по базе знаний.
const NameSearchKnowledge = "search_knowledge"

// KnowledgeSearch ищет ответы в локальной базе знаний (Markdown).
type KnowledgeSearch struct {
	index *kb.Index
}

// NewKnowledgeSearch создаёт инструмент поверх готового индекса.
func NewKnowledgeSearch(index *kb.Index) *KnowledgeSearch {
	return &KnowledgeSearch{index: index}
}

// Name реализует Tool.
func (t *KnowledgeSearch) Name() string { return NameSearchKnowledge }

// Description реализует Tool.
func (t *KnowledgeSearch) Description() string {
	return "Ищет в локальной базе знаний проекта выдержки по тренировкам, питанию, сну, восстановлению, добавкам, анализам. Используй для любых вопросов о здоровье/ЗОЖ; отвечай только по найденному и citing файл/PMID."
}

// Schema реализует Tool.
func (t *KnowledgeSearch) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Поисковый запрос на русском, 2–8 слов"},
			"person": {"type": "string", "enum": ["athlete", "wife"], "description": "Чей профиль: athlete — атлет, wife — жена"},
			"limit": {"type": "integer", "minimum": 1, "maximum": 6, "description": "Сколько фрагментов вернуть (по умолчанию 4)"}
		},
		"required": ["query", "person"]
	}`)
}

// Execute реализует Tool.
func (t *KnowledgeSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var params struct {
		Query  string `json:"query"`
		Person string `json:"person"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("search_knowledge: разобрать аргументы: %w", err)
	}
	person, _, ok := parsePerson(params.Person)
	if !ok {
		return "", fmt.Errorf("search_knowledge: неизвестный person %q (ожидается athlete или wife)", params.Person)
	}
	if strings.TrimSpace(params.Query) == "" {
		return "", fmt.Errorf("search_knowledge: пустой query")
	}
	if t.index == nil {
		return "база знаний ещё не загружена. Скажи, что локальная база недоступна, и дай общую практику без ссылок.", nil
	}
	results := t.index.SearchHybrid(ctx, params.Query, person, params.Limit)
	if len(results) == 0 {
		return "в базе знаний ничего не найдено по этому запросу. Не придумывай источники — скажи, что в базе пока нет, и дай общую практику без ссылок.", nil
	}
	lines := make([]string, 0, len(results)*3)
	for index, result := range results {
		lines = append(lines, fmt.Sprintf("%d. %s — %s / %s", index+1, result.Path, result.Title, result.Heading))
		if len(result.PMIDs) > 0 {
			lines = append(lines, "   PMID: "+strings.Join(result.PMIDs, ", "))
		}
		lines = append(lines, "   "+strings.ReplaceAll(result.Snippet, "\n", " "))
	}
	return strings.Join(lines, "\n"), nil
}

var _ Tool = (*KnowledgeSearch)(nil)
