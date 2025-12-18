package model

import (
	"fmt"
)

// ScanColumns формирует список колонок для SELECT:
//   - обычные поля: <alias>.<field> (без AS)
//   - belongs_to: рекурсивный проход по nested preset связанной модели
//   - has_one/has_many: добавить РОВНО ОДИН ключ родителя — <parentAlias>.<rel.PK>
//     (этого достаточно, чтобы затем догружать has_* по IDs)
func (m *Model) ScanColumns(preset *DataPreset, aliasMap *AliasMap, prefix string) ([]string, map[string]string) {

	if preset == nil {
		return nil, nil
	}
	cols := make([]string, 0)
	types := make(map[string]string)
	seen := make(map[string]struct{})

	aliasFor := func(path string) string {
		if path == "" {
			return "main"
		}
		if a, ok := aliasMap.PathToAlias[path]; ok && a != "" {
			return a
		}
		return "main" // безопасный дефолт: алиас корня
	}
	addCol := func(expr string, fType string) {
		if _, ok := seen[expr]; ok {
			return
		}
		seen[expr] = struct{}{}
		types[expr] = fType
		cols = append(cols, expr)
	}

	for _, f := range preset.Fields {
		switch f.Type {
		case "preset":
			relKey := f.Source
			rel, ok := m.Relations[relKey]
			if !ok || rel == nil {
				continue
			}

			switch rel.Type {
			case "belongs_to":
				if rel._ModelRef == nil {
					continue
				}
				// найти nested preset: поддерживаем "Model.Preset" и "Preset"
				var nested *DataPreset
				if f._PresetRef != nil {
					nested = f._PresetRef
				} else if f.NestedPreset != "" {
					nested = rel._ModelRef.Presets[f.NestedPreset]
				}
				if nested == nil {
					continue
				}

				// рекурсивно, prefix расширяем ключом связи (а не alias-именем поля)
				nextPrefix := relKey
				if prefix != "" {
					nextPrefix = prefix + "." + relKey
				}
				sub, subtypes := rel._ModelRef.ScanColumns(nested, aliasMap, nextPrefix)
				for _, c := range sub {
					addCol(c, subtypes[c])
				}

			case "has_one", "has_many":
				// ДОБАВЛЯЕМ РОВНО ОДИН ключ родителя для дальнейшей догрузки has_ по ID
				parentAlias := aliasFor(prefix)
				pk := rel.PK
				expr := fmt.Sprintf("%s.%s", parentAlias, pk)
				// не добавлять, если уже есть (сравнение по точному expr)
				if _, ok := seen[expr]; !ok {
					addCol(expr, "int")
				}
			}
		case "formatter":
			// форматтер в SELECT не добавляем — он считается в finalizeItems
			continue
		case "int", "string", "bool", "float", "UUID", "time", "datetime", "date":
			// обычные SQL-колонки
			a := aliasFor(prefix)
			addCol(fmt.Sprintf("%s.%s", a, f.Source), f.Type)
		default:
			// на всякий — ничего не добавляем
			continue
		}
	}

	return cols, types
}
