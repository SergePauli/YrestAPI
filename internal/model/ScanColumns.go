package model

import (
	"fmt"
	"strings"
)

// ScanColumns формирует список колонок для SELECT:
//   - обычные поля: <alias>.<field>
//   - computable: expr/subquery с alias
//   - belongs_to: рекурсивный проход по nested preset связанной модели
//   - has_one/has_many: добавить РОВНО ОДИН ключ родителя — <parentAlias>.<rel.PK>
func (m *Model) ScanColumns(preset *DataPreset, aliasMap *AliasMap, prefix string) []SelectColumn {

	if preset == nil {
		return nil
	}
	cols := make([]SelectColumn, 0)
	seen := make(map[string]struct{})

	aliasFor := func(path string) string {
		if path == "" {
			return "main"
		}
		if aliasMap != nil {
			if a, ok := aliasMap.PathToAlias[path]; ok && a != "" {
				return a
			}
		}
		return "main" // безопасный дефолт: алиас корня
	}
	addCol := func(expr, key, fType string) {
		if expr == "" || key == "" {
			return
		}
		if _, ok := seen[expr]; ok {
			return
		}
		seen[expr] = struct{}{}
		cols = append(cols, SelectColumn{Expr: expr, Key: key, Type: fType})
	}
	joinKey := func(pref, name string) string {
		if strings.TrimSpace(pref) == "" {
			return name
		}
		return pref + "." + name
	}

	for _, f := range preset.Fields {
		switch f.Type {
		case "preset":
			relKey := f.Source
			rel, ok := m.Relations[relKey]
			if !ok || rel == nil {
				continue
			}

			if rel.Polymorphic {
				parentAlias := aliasFor(prefix)
				addCol(fmt.Sprintf("%s.%s", parentAlias, rel.FK), joinKey(prefix, rel.FK), "int")
				typeCol := rel.TypeColumn
				if strings.TrimSpace(typeCol) == "" {
					typeCol = relKey + "_type"
				}
				addCol(fmt.Sprintf("%s.%s", parentAlias, typeCol), joinKey(prefix, typeCol), "string")
				// не строим вложенные JOIN-ы
				continue
			}

			switch rel.Type {
			case "belongs_to":
				if rel._ModelRef == nil {
					continue
				}
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
				sub := rel._ModelRef.ScanColumns(nested, aliasMap, nextPrefix)
				cols = append(cols, sub...)

			case "has_one", "has_many":
				// ДОБАВЛЯЕМ РОВНО ОДИН ключ родителя для дальнейшей догрузки has_ по ID
				parentAlias := aliasFor(prefix)
				pk := rel.PK
				expr := fmt.Sprintf("%s.%s", parentAlias, pk)
				if _, ok := seen[expr]; !ok {
					addCol(expr, joinKey(prefix, pk), "int")
				}
			}
		case "formatter":
			// форматтер в SELECT не добавляем — он считается в finalizeItems
			continue
		case "computable":
			comp := m.Computable[f.Source]
			if comp == nil {
				continue
			}
			aliasName := strings.TrimSpace(f.Alias)
			if aliasName == "" {
				aliasName = f.Source
			}
			expr := applyAliasPlaceholders(comp.Source, aliasMap, prefix)
			selectExpr := expr
			if isSubquerySource(expr) {
				selectExpr = fmt.Sprintf("%s AS %s", wrapSubquery(expr), quoteIdentifier(aliasName))
			} else {
				selectExpr = fmt.Sprintf("%s AS %s", expr, quoteIdentifier(aliasName))
			}
			colType := comp.Type
			if strings.TrimSpace(colType) == "" {
				colType = f.Type
			}
			addCol(selectExpr, joinKey(prefix, aliasName), colType)
		case "int", "string", "bool", "float", "UUID", "time", "datetime", "date":
			// обычные SQL-колонки
			a := aliasFor(prefix)
			target := strings.TrimSpace(f.Alias)
			if target == "" {
				target = f.Source
			}

			if isSubquerySource(f.Source) {
				expr := fmt.Sprintf("%s AS %s", wrapSubquery(applyAliasPlaceholders(f.Source, aliasMap, prefix)), quoteIdentifier(target))
				addCol(expr, joinKey(prefix, target), f.Type)
				continue
			}
			addCol(fmt.Sprintf("%s.%s", a, f.Source), joinKey(prefix, target), f.Type)
		default:
			// на всякий — ничего не добавляем
			continue
		}
	}

	return cols
}
