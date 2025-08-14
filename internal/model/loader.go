package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadModelsFromDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return err
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// 1. Разбираем в yaml.Node для структурной валидации
		var root yaml.Node
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("YAML parse error in %s: %w", path, err)
		}

		// YAML всегда [0] - документ, [1] - root mapping
		if len(root.Content) == 0 {
			return fmt.Errorf("empty YAML in %s", path)
		}

		if err := validateYAMLNode(root.Content[0], "model"); err != nil {
			return fmt.Errorf("validation error in %s: %w", path, err)
		}

		// 2. Теперь уже Unmarshal в модель
		var model Model
		if err := root.Decode(&model); err != nil {
			return fmt.Errorf("unmarshal error in %s: %w", path, err)
		}

		// 3. Регистрируем модель
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		Registry[name] = &model
		fmt.Printf("✅ Модель %s загружена с %d связями\n", name, len(model.Relations))
	}
	return nil
}

