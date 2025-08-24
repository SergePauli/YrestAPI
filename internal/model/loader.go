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

		//2.1 
		if err := resolvePresetInheritance(&model); err != nil {
			return fmt.Errorf("inheritance error: %w", err)
		}

		// 3. Регистрируем модель
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		Registry[name] = &model
		fmt.Printf("✅ Модель %s загружена с %d связями\n", name, len(model.Relations))
	}
	return nil
}


func resolvePresetInheritance(m *Model) error {
	if m == nil || len(m.Presets) == 0 {
		return nil
	}

	cache := make(map[string][]Field)
	stack := make(map[string]bool) // для детекции циклов

	var keyOf = func(f Field) string {
		if strings.TrimSpace(f.Alias) != "" {
			return f.Alias
		}
		return f.Source
	}

	var dfs func(string) ([]Field, error)
	dfs = func(name string) ([]Field, error) {
		if fields, ok := cache[name]; ok {
			return fields, nil
		}
		if stack[name] {
			return nil, fmt.Errorf("cyclic extends detected in model '%s' at preset '%s'", m.Table, name)
		}
		p := m.Presets[name]
		if p == nil {
			return nil, fmt.Errorf("preset '%s' not found in model '%s'", name, m.Table)
		}

		stack[name] = true
		var result []Field

		// 1) Наследуемся
		if parent := strings.TrimSpace(p.Extends); parent != "" {
			parentFields, err := dfs(parent)
			if err != nil {
				return nil, err
			}
			// Копируем, чтобы не портить кэш родителя
			result = append(result, append([]Field(nil), parentFields...)...)
		}

		// 2) Переопределяем/добавляем
		for _, f := range p.Fields {
			k := keyOf(f)
			idx := -1
			for i := range result {
				if keyOf(result[i]) == k {
					idx = i
					break
				}
			}
			if idx >= 0 {
				// замещаем, позицию сохраняем
				result[idx] = f
			} else {
				// новое поле — в конец
				result = append(result, f)
			}
		}

		stack[name] = false
		cache[name] = result

		// записываем обратно в пресет
		p.Fields = result
		return result, nil
	}

	for name := range m.Presets {
		if _, err := dfs(name); err != nil {
			return err
		}
	}
	return nil
}


