package store

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// Cache — in-memory кэш разобранной истории (FR-2/масштабирование).
// Ключ — person+вид данных; валидность проверяется по mtime/size каталога,
// который меняется при атомарной записи (tmp→rename). Позже заменяется
// SQLite-индексом (см. docs/architecture/scaling-plan.md).
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	stamp string
	value any
}

// NewCache создаёт пустой кэш.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]cacheEntry)}
}

func (c *Cache) get(key, stamp string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || entry.stamp != stamp {
		return nil, false
	}
	return entry.value, true
}

func (c *Cache) set(key, stamp string, value any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries[key] = cacheEntry{stamp: stamp, value: value}
	c.mu.Unlock()
}

// Invalidate сбрасывает весь кэш (после записи любым writer).
func (c *Cache) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]cacheEntry)
	c.mu.Unlock()
}

// InvalidatePerson сбрасывает кэш конкретного человека.
func (c *Cache) InvalidatePerson(person string) {
	if c == nil {
		return
	}
	prefix := string(person) + ":"
	c.mu.Lock()
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}

// dirStamp возвращает отпечаток каталога по составу файлов (имя/размер/mtime).
// mtime самого каталога на некоторых ФС имеет грубое разрешение и не меняется
// при правке существующего файла внешним процессом — поэтому его недостаточно.
func dirStamp(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var builder strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return "", false
		}
		fmt.Fprintf(&builder, "%s:%d:%d;", entry.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return builder.String(), true
}
