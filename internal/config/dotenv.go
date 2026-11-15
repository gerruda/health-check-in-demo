package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv читает KEY=VALUE из файла path и выставляет переменные окружения,
// не перезаписывая уже заданные. Строки с # в начале пропускаются; поддерживаются
// префикс export и значения в одинарных/двойных кавычках.
//
// Отсутствие файла не считается ошибкой: конфигурацию можно задавать только
// переменными окружения (как в CI).
func LoadDotEnv(path string) error {
	//nolint:gosec // путь задаёт оператор (.env в рабочем каталоге)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("открыть %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			first, last := value[0], value[len(value)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("установить %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("читать %s: %w", path, err)
	}
	return nil
}
