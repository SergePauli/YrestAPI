package model

import "strings"

// ExpandAliasPath разворачивает алиасы в пути, проходя по связям модели.
// Например, contract.contragent.org.name, где org — алиас во вложенной модели Contragent.
func ExpandAliasPath(m *Model, path string) string {
	if m == nil || path == "" {
		return path
	}
	segs := strings.Split(path, ".")
	curr := m
	for i := 0; i < len(segs); i++ {
		seg := segs[i]

		// Подставляем алиас, если он есть на текущей модели
		if full, ok := curr.Aliases[seg]; ok {
			full = strings.TrimSpace(full)
			if full != "" && full != seg {
				aliasSegs := strings.Split(full, ".")
				// заменяем текущий сегмент на развёрнутый, оставляя префикс
				newSegs := append([]string{}, segs[:i]...)
				newSegs = append(newSegs, aliasSegs...)
				newSegs = append(newSegs, segs[i+1:]...)
				segs = newSegs
				i--
				continue
			}
		}

		rel := curr.Relations[seg]
		if rel == nil || rel._ModelRef == nil {
			break
		}
		curr = rel._ModelRef
	}

	return strings.Join(segs, ".")
}

// normalizeFiltersWithAliases разворачивает алиасы в ключах фильтров (часть до "__").
func NormalizeFiltersWithAliases(m *Model, filters map[string]any) map[string]any {
	if m == nil || len(filters) == 0 {
		return filters
	}

	var normalize func(any) (any, bool)
	normalize = func(v any) (any, bool) {
		switch val := v.(type) {
		case map[string]any:
			changed := false
			out := make(map[string]any, len(val))
			for k, raw := range val {
				if k == "or" || k == "and" {
					if subMap, ok := raw.(map[string]any); ok {
						nv, ch := normalize(subMap)
						out[k] = nv
						if ch {
							changed = true
						}
						continue
					}
					if arr, ok := raw.([]any); ok {
						newArr := make([]any, 0, len(arr))
						arrChanged := false
						for _, item := range arr {
							if subMap, ok := item.(map[string]any); ok {
								nv, ch := normalize(subMap)
								newArr = append(newArr, nv)
								if ch {
									arrChanged = true
								}
							} else {
								newArr = append(newArr, item)
							}
						}
						out[k] = newArr
						if arrChanged {
							changed = true
						}
						continue
					}
				}

				field := k
				op := ""
				if i := strings.Index(k, "__"); i >= 0 {
					field = k[:i]
					op = k[i+2:]
				}
				fields, comb := ParseCompositeField(field)
				for i := range fields {
					fields[i] = ExpandAliasPath(m, fields[i])
				}
				newField := strings.Join(fields, comb)
				if newField != field {
					changed = true
				}
				newKey := newField
				if op != "" {
					newKey = newField + "__" + op
				}
				out[newKey] = raw
			}
			return out, changed
		case []any:
			newArr := make([]any, 0, len(val))
			changed := false
			for _, item := range val {
				if subMap, ok := item.(map[string]any); ok {
					nv, ch := normalize(subMap)
					newArr = append(newArr, nv)
					if ch {
						changed = true
					}
				} else {
					newArr = append(newArr, item)
				}
			}
			return newArr, changed
		default:
			return v, false
		}
	}

	nv, changed := normalize(filters)
	if !changed {
		return filters
	}
	if out, ok := nv.(map[string]any); ok {
		return out
	}
	return filters
}

// normalizeSortsWithAliases разворачивает алиасы в сортировках.
func NormalizeSortsWithAliases(m *Model, sorts []string) []string {
	if m == nil || len(sorts) == 0 {
		return sorts
	}
	changed := false
	out := make([]string, len(sorts))
	for i, s := range sorts {
		parts := strings.SplitN(s, " ", 2)
		field := parts[0]
		dir := ""
		if len(parts) > 1 {
			dir = strings.TrimSpace(parts[1])
		}
		fields, comb := ParseCompositeField(field)
		for i := range fields {
			fields[i] = ExpandAliasPath(m, fields[i])
		}
		newField := strings.Join(fields, comb)
		if newField != field {
			changed = true
		}
		if dir != "" {
			out[i] = newField + " " + dir
		} else {
			out[i] = newField
		}
	}
	if !changed {
		return sorts
	}
	return out
}

// ParseCompositeField ищет разделители "_or_" или "_and_" и возвращает список полей и разделитель.
// Если разделителей нет — возвращает одиночное поле и пустой разделитель.
func ParseCompositeField(field string) ([]string, string) {
	if strings.Contains(field, "_or_") {
		return strings.Split(field, "_or_"), "_or_"
	}
	if strings.Contains(field, "_and_") {
		return strings.Split(field, "_and_"), "_and_"
	}
	return []string{field}, ""
}
