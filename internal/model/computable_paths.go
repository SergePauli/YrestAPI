package model

import "strings"

// collectComputablePathsForRequest собирает relation-пути, используемые в computable-полях,
// которые задействованы в заданном пресете/фильтрах/сортировках.
func collectComputablePathsForRequest(m *Model, preset *DataPreset, filters map[string]interface{}, sorts []string) []string {
	if m == nil || len(m.Computable) == 0 {
		return nil
	}

	used := make(map[string]struct{})

	if preset != nil {
		for _, f := range preset.Fields {
			if _, ok := m.Computable[f.Source]; ok {
				used[f.Source] = struct{}{}
			}
		}
	}

	for key := range filters {
		base := key
		if i := strings.Index(key, "__"); i >= 0 {
			base = key[:i]
		}
		if _, ok := m.Computable[base]; ok {
			used[base] = struct{}{}
		}
	}

	for _, s := range sorts {
		parts := strings.Fields(s)
		if len(parts) == 0 {
			continue
		}
		field := parts[0]
		if _, ok := m.Computable[field]; ok {
			used[field] = struct{}{}
		}
	}

	pathSet := make(map[string]struct{})
	for name := range used {
		c := m.Computable[name]
		for _, p := range extractPathsFromExpr(c.Source) {
			pathSet[p] = struct{}{}
		}
		for _, p := range extractPathsFromExpr(c.Where) {
			pathSet[p] = struct{}{}
		}
	}

	out := make([]string, 0, len(pathSet))
	for p := range pathSet {
		out = append(out, p)
	}
	return out
}

// collectComputableExprsForRequest возвращает выражения computable полей, используемых в запросе.
func collectComputableExprsForRequest(m *Model, preset *DataPreset, filters map[string]interface{}, sorts []string, aliasMap *AliasMap) []string {
	if m == nil || len(m.Computable) == 0 {
		return nil
	}

	used := make(map[string]struct{})

	if preset != nil {
		for _, f := range preset.Fields {
			if _, ok := m.Computable[f.Source]; ok {
				used[f.Source] = struct{}{}
			}
		}
	}

	for key := range filters {
		base := key
		if i := strings.Index(key, "__"); i >= 0 {
			base = key[:i]
		}
		if _, ok := m.Computable[base]; ok {
			used[base] = struct{}{}
		}
	}

	for _, s := range sorts {
		parts := strings.Fields(s)
		if len(parts) == 0 {
			continue
		}
		field := parts[0]
		if _, ok := m.Computable[field]; ok {
			used[field] = struct{}{}
		}
	}

	out := make([]string, 0, len(used))
	for name := range used {
		if expr, ok := m.resolveFieldExpression(preset, aliasMap, name); ok && strings.TrimSpace(expr) != "" {
			out = append(out, expr)
		}
	}
	return out
}
