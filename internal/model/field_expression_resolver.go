package model

import (
	"fmt"
	"strings"
)

// resolveFieldExpression ищет expression для поля:
// 1) computable по имени (global)
// 2) простые поля через aliasMap
func (m *Model) resolveFieldExpression(preset *DataPreset, aliasMap *AliasMap, fieldPath string) (string, bool) {
	if m == nil {
		return "", false
	}

	// computable (только если нет точек в имени)
	if comp, ok := m.Computable[fieldPath]; ok && comp != nil {
		expr := comp.Where
		if strings.TrimSpace(expr) == "" {
			expr = comp.Source
		}
		expr = applyAliasPlaceholders(expr, aliasMap, "")
		return expr, true
	}

	// вложенные пути: пытаемся найти computable в конечной модели
	segs := strings.Split(fieldPath, ".")
	if len(segs) > 1 {
		field := segs[len(segs)-1]
		prefix := strings.Join(segs[:len(segs)-1], ".")

		targetModel := m
		for i := 0; i < len(segs)-1; i++ {
			rel := targetModel.Relations[segs[i]]
			if rel == nil || rel.GetModelRef() == nil {
				targetModel = nil
				break
			}
			targetModel = rel.GetModelRef()
		}

		if targetModel != nil {
			if comp, ok := targetModel.Computable[field]; ok && comp != nil {
				expr := comp.Where
				if strings.TrimSpace(expr) == "" {
					expr = comp.Source
				}
				expr = applyAliasPlaceholders(expr, aliasMap, prefix)
				return expr, true
			}
		}

		if aliasMap != nil {
			if alias, ok := aliasMap.PathToAlias[prefix]; ok {
				return fmt.Sprintf("%s.%s", alias, field), true
			}
		}
		return fmt.Sprintf("%s.%s", prefix, field), true
	}

	// поле корня
	return fmt.Sprintf("main.%s", fieldPath), true
}
