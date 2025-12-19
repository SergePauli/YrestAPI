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

		if err := applyTemplateIncludes(dir, &model); err != nil {
			return fmt.Errorf("include error in %s: %w", path, err)
		}

		//2.1
		if err := resolvePresetInheritance(&model); err != nil {
			return fmt.Errorf("inheritance error: %w", err)
		}

		// 3. Регистрируем модель
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		model.Name = name
		Registry[name] = &model
		fmt.Printf("✅ Модель %s загружена с %d связями\n", name, len(model.Relations))
	}
	return nil
}

// applyTemplateIncludes merges relations/presets from template files located in dir/templates.
// Model fields override template fields; preset fields from the model override/extend template preset fields.
func applyTemplateIncludes(baseDir string, m *Model) error {
	if m == nil || len(m.Includes) == 0 {
		return nil
	}

	tplDir := filepath.Join(baseDir, "templates")
	mergeFields := func(dst []Field, src []Field) []Field {
		filtered := make([]Field, 0, len(dst))
		for _, f := range dst {
			if strings.TrimSpace(f.Alias) == "skip" {
				continue
			}
			filtered = append(filtered, f)
		}
		dst = filtered

		keyOf := func(f Field) string {
			if strings.TrimSpace(f.Alias) != "" {
				return f.Alias
			}
			return f.Source
		}
		index := make(map[string]int, len(dst))
		for i := range dst {
			index[keyOf(dst[i])] = i
		}
		for _, f := range src {
			if strings.TrimSpace(f.Alias) == "skip" {
				continue
			}
			k := keyOf(f)
			if pos, ok := index[k]; ok {
				dst[pos] = f
			} else {
				index[k] = len(dst)
				dst = append(dst, f)
			}
		}
		return dst
	}

	mergeRelation := func(dst, src *ModelRelation) {
		if dst == nil || src == nil {
			return
		}
		if dst.Type == "" {
			dst.Type = src.Type
		}
		if dst.Model == "" {
			dst.Model = src.Model
		}
		if dst.Table == "" {
			dst.Table = src.Table
		}
		if dst.FK == "" {
			dst.FK = src.FK
		}
		if dst.PK == "" {
			dst.PK = src.PK
		}
		if dst.Through == "" {
			dst.Through = src.Through
		}
		if dst.Where == "" {
			dst.Where = src.Where
		}
		if dst.ThroughWhere == "" {
			dst.ThroughWhere = src.ThroughWhere
		}
		if dst.Order == "" {
			dst.Order = src.Order
		}
		if !dst.Reentrant {
			dst.Reentrant = src.Reentrant
		}
		if dst.MaxDepth == 0 {
			dst.MaxDepth = src.MaxDepth
		}
		if !dst.Polymorphic {
			dst.Polymorphic = src.Polymorphic
		}
		if dst.TypeColumn == "" {
			dst.TypeColumn = src.TypeColumn
		}
	}

	for _, inc := range m.Includes {
		tplPath := filepath.Join(tplDir, inc+".yml")
		data, err := os.ReadFile(tplPath)
		if err != nil {
			return fmt.Errorf("read template %s: %w", tplPath, err)
		}
		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err != nil {
			return fmt.Errorf("parse template %s: %w", tplPath, err)
		}
		if len(node.Content) == 0 {
			return fmt.Errorf("template %s is empty", tplPath)
		}
		if err := validateYAMLNode(node.Content[0], "model"); err != nil {
			return fmt.Errorf("template validation %s: %w", tplPath, err)
		}
		var tpl Model
		if err := node.Decode(&tpl); err != nil {
			return fmt.Errorf("unmarshal template %s: %w", tplPath, err)
		}
		if err := resolvePresetInheritance(&tpl); err != nil {
			return fmt.Errorf("inheritance in template %s: %w", tplPath, err)
		}

		// merge relations: model wins on conflicts; missing fields can be filled from template
		if tpl.Relations != nil {
			if m.Relations == nil {
				m.Relations = map[string]*ModelRelation{}
			}
			for k, v := range tpl.Relations {
				if existing, exists := m.Relations[k]; !exists {
					cp := *v
					m.Relations[k] = &cp
				} else {
					mergeRelation(existing, v)
				}
			}
		}

		// merge presets
		if tpl.Presets != nil {
			if m.Presets == nil {
				m.Presets = map[string]*DataPreset{}
			}
			for name, tp := range tpl.Presets {
				if existing, ok := m.Presets[name]; ok {
					existing.Fields = mergeFields(tp.Fields, existing.Fields)
					m.Presets[name] = existing
				} else {
					// copy to avoid sharing template struct
					cp := *tp
					m.Presets[name] = &cp
				}
			}
		}
	}

	return nil
}

// resolvePresetInheritance поддерживает множественное наследование:
//
//	extends: "base, head"
//
// Родители применяются слева направо: поля первого родителя добавляются первыми,
// последующие родители переопределяют совпадающие поля, но НЕ меняют их позицию.
// Затем применяются локальные поля пресета — тоже с переопределением и сохранением позиции.
func resolvePresetInheritance(m *Model) error {
	if m == nil || len(m.Presets) == 0 {
		return nil
	}

	cache := make(map[string][]Field)
	stack := make(map[string]bool) // для детекции циклов

	// Ключ поля для сравнения — alias, иначе source
	keyOf := func(f Field) string {
		if strings.TrimSpace(f.Alias) != "" {
			return f.Alias
		}
		return f.Source
	}

	// Парсим список родителей из строки вида "base, head"
	parseParents := func(s string) []string {
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		seen := make(map[string]struct{}, len(parts))
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			// уберём дубли в extends
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
		return out
	}

	// Слияние src в dst c переопределением по key, но без смены позиции.
	mergeFields := func(dst []Field, src []Field) []Field {
		index := make(map[string]int, len(dst))
		for i := range dst {
			index[keyOf(dst[i])] = i
		}
		for _, f := range src {
			k := keyOf(f)
			if pos, ok := index[k]; ok {
				// переопределяем значение, позиция сохраняется
				dst[pos] = f
			} else {
				index[k] = len(dst)
				dst = append(dst, f)
			}
		}
		return dst
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

		// 1) Наследуемся от каждого родителя слева направо
		if parents := parseParents(strings.TrimSpace(p.Extends)); len(parents) > 0 {
			for _, parent := range parents {
				parentFields, err := dfs(parent)
				if err != nil {
					return nil, err
				}
				// ВАЖНО: копируем parentFields, чтобы не трогать кэш
				result = mergeFields(result, append([]Field(nil), parentFields...))
			}
		}

		// 2) Применяем собственные поля (переопределение + добавление)
		if len(p.Fields) > 0 {
			result = mergeFields(result, p.Fields)
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
