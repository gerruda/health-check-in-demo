package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Path возвращает путь YAML-файла настроек человека.
func (s *Store) Path(person string) (string, error) {
	if strings.TrimSpace(s.dir) == "" {
		return "", errors.New("settings: пуст каталог настроек")
	}
	switch person {
	case PersonAthlete, PersonWife:
		return filepath.Join(s.dir, person+".yaml"), nil
	default:
		return "", fmt.Errorf("settings: неизвестный person %q (ожидается %q или %q)", person, PersonAthlete, PersonWife)
	}
}

func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("settings: прочитать %q: %w", path, err)
	}
	return data, nil
}

func unmarshalYAML(data []byte, target any) error {
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("yaml: %w", err)
	}
	return nil
}

func saveSettings(path string, settings Settings) error {
	data, err := yaml.Marshal(settings)
	if err != nil {
		return fmt.Errorf("settings: сериализовать YAML: %w", err)
	}

	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("settings: создать каталог %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("settings: создать временный файл: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("settings: записать временный файл: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: закрыть временный файл: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("settings: переименовать в %q: %w", path, err)
	}
	return nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
