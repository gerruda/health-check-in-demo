package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"healthbot/internal/llm"
)

// Registry — набор инструментов для LLM (FR-6.7.2).
type Registry struct {
	order  []string
	byName map[string]Tool
}

// NewRegistry создаёт реестр из инструментов.
func NewRegistry(items ...Tool) *Registry {
	registry := &Registry{byName: make(map[string]Tool, len(items))}
	for _, tool := range items {
		if tool == nil {
			continue
		}
		name := tool.Name()
		registry.byName[name] = tool
		registry.order = append(registry.order, name)
	}
	sort.Strings(registry.order)
	return registry
}

// Descriptors возвращает описания инструментов для LLM.
func (r *Registry) Descriptors() []llm.Tool {
	descriptors := make([]llm.Tool, 0, len(r.order))
	for _, name := range r.order {
		tool := r.byName[name]
		descriptors = append(descriptors, llm.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Schema(),
		})
	}
	return descriptors
}

// Execute выполняет инструмент по имени.
func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	tool, ok := r.byName[name]
	if !ok {
		return "", fmt.Errorf("tools: неизвестный инструмент %q", name)
	}
	return tool.Execute(ctx, args)
}
