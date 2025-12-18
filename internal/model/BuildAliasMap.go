package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// BuildAliasMap собирает карту алиасов для конкретного запроса:
//   - берёт готовые пути/алиасы из preset.FieldsAliasMap (если preset != nil),
//   - добавляет пути из filters/sorts,
//   - проверяет политику re-энтри (rel.Reentrant, rel.MaxDepth) при добавлении НОВЫХ путей,
//   - возвращает обе проекции Path↔Alias.
//
// Правила:
//   - Разрешённые типы связей для пути: has_one, has_many, belongs_to.
//   - re-entry считается "возвратом в уже встречавшуюся модель по пути":
//     нужен rel.Reentrant == true и repeats+1 <= effMax, где effMax = field.MaxDepth (для полей его тут нет) или rel.MaxDepth,
//     если effMax <= 0 — трактуем как 1 (только одно посещение модели на пути; без возвратов).
func BuildAliasMap(model *Model, preset *DataPreset, filters map[string]interface{}, sorts []string) (*AliasMap, error) {
	if model == nil {
		return nil, fmt.Errorf("BuildAliasMap: model is nil")
	}

	am := &AliasMap{
		PathToAlias: map[string]string{},
		AliasToPath: map[string]string{},
	}

	// 1) База: скопировать уже рассчитанные алиасы из пресета (никакой повторной валидации тут не делаем).
	nextIdx := 0
	if preset != nil && preset.FieldsAliasMap != nil {
		for path, alias := range preset.FieldsAliasMap.PathToAlias {
			am.PathToAlias[path] = alias
			am.AliasToPath[alias] = path
		}
		// подобрать стартовый индекс для новых алиасов, чтобы не конфликтовать с существующими tN
		nextIdx = detectNextAliasIndex(am)
	}

	// 2) Собрать дополнительные пути из filters и sorts.
	extra := mergeAndSortPaths(
		PathsFromFilters(filters),
		PathsFromSorts(sorts),
	)

	// 3) Для каждого нужного пути — зарегистрировать алиасы на всех промежуточных сегментах.
	for _, full := range extra {
		if err := ensureAliasPath(model, am, full, &nextIdx); err != nil {
			return nil, err
		}
	}

	return am, nil
}

// --- helpers ---

// ensureAliasPath гарантирует, что для каждого префикса пути "a", "a.b", "a.b.c"
// есть алиас. При добавлении нового префикса проверяет rel.Reentrant/rel.MaxDepth.
func ensureAliasPath(root *Model, am *AliasMap, fullPath string, nextIdx *int) error {
	if fullPath == "" {
		return nil
	}
	segs := strings.Split(fullPath, ".")
	curr := root
	var stack []*Model
	path := ""

	for i, seg := range segs {
		rel := curr.Relations[seg]
		if rel == nil {
			return fmt.Errorf("relation %q not found in model %s", seg, curr.Name)
		}
		if rel.Polymorphic {
			// не создаём алиасы глубже полиморфной связи
			return nil
		}
		switch rel.Type {
		case "has_one", "has_many", "belongs_to":
			// ok
		default:
			return fmt.Errorf("unsupported relation type %q on %s.%s", rel.Type, rel.Model, seg)
		}
		nextModel := rel._ModelRef
		if path == "" {
			path = seg
		} else {
			path = path + "." + seg
		}

		// проверка re-entry по МОДЕЛИ только если этот префикс ещё не зарегистрирован
		if _, exists := am.PathToAlias[path]; !exists {
			repeats := countModelIn(stack, nextModel)
			if repeats > 0 {
				effMax := rel.MaxDepth
				if effMax <= 0 {
					effMax = 1 // посещений модели на одном пути по умолчанию
				}
				if !rel.Reentrant {
					return fmt.Errorf("not reentrant: returning to model %s via %q at %q", rel.Model, seg, path)
				}
				if repeats+1 > effMax {
					return fmt.Errorf("max_depth exceeded for model %s (eff=%d) at %q", rel.Model, effMax, path)
				}
			}

			// назначаем новый алиас, избегая конфликтов с уже занятыми
			alias := nextAlias(am, nextIdx)
			am.PathToAlias[path] = alias
			am.AliasToPath[alias] = path
		}

		stack = append(stack, nextModel)
		curr = nextModel

		// защитимся от пустых сегментов (хотя split их не даёт) и лимита на всякий случай
		if seg == "" || i >= 1024 {
			return fmt.Errorf("invalid or too deep path %q", fullPath)
		}
	}
	return nil
}

// PathsFromFilters: извлекает relation-префиксы из ключей фильтров.
// "a.b.c__in" -> "a.b"
func PathsFromFilters(filters map[string]interface{}) []string {
	if len(filters) == 0 {
		return nil
	}
	out := make([]string, 0, len(filters))
	for key := range filters {
		base := key
		if idx := strings.Index(key, "__"); idx >= 0 {
			base = key[:idx]
		}
		if i := strings.LastIndex(base, "."); i >= 0 {
			out = append(out, base[:i])
		}
	}
	return dedup(out)
}

// PathsFromSorts: извлекает relation-префиксы из сортировок.
// Поддерживает "a.b.c ASC" / "a.b.c" → "a.b"
func PathsFromSorts(sorts []string) []string {
	if len(sorts) == 0 {
		return nil
	}
	out := make([]string, 0, len(sorts))
	for _, s := range sorts {
		fld := strings.Fields(s)
		if len(fld) == 0 {
			continue
		}
		col := fld[0] // "a.b.c"
		if i := strings.LastIndex(col, "."); i >= 0 {
			out = append(out, col[:i])
		}
	}
	return dedup(out)
}

func mergeAndSortPaths(a, b []string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for _, p := range a {
		if p != "" {
			set[p] = struct{}{}
		}
	}
	for _, p := range b {
		if p != "" {
			set[p] = struct{}{}
		}
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	// сначала мелкие (по глубине), затем лексикографически — чтобы алиасы строились «снизу вверх»
	sort.Slice(paths, func(i, j int) bool {
		di := strings.Count(paths[i], ".")
		dj := strings.Count(paths[j], ".")
		if di != dj {
			return di < dj
		}
		return paths[i] < paths[j]
	})
	return paths
}

func countModelIn(stack []*Model, x *Model) int {
	c := 0
	for _, m := range stack {
		if m == x {
			c++
		}
	}
	return c
}

// detectNextAliasIndex ищет максимальный индекс tN среди уже занятых алиасов и возвращает N+1.
func detectNextAliasIndex(am *AliasMap) int {
	maxN := -1
	for alias := range am.AliasToPath {
		if strings.HasPrefix(alias, "t") {
			if n, err := strconv.Atoi(alias[1:]); err == nil && n > maxN {
				maxN = n
			}
		}
	}
	return maxN + 1
}

// nextAlias возвращает первый свободный "tN", начиная с *nextIdx.
func nextAlias(am *AliasMap, nextIdx *int) string {
	for {
		alias := fmt.Sprintf("t%d", *nextIdx)
		*nextIdx++
		if _, exists := am.AliasToPath[alias]; !exists {
			return alias
		}
	}
}

// dedup — простая дедупликация с сохранением первого вхождения (порядок дальше всё равно нормализуем)
func dedup(ss []string) []string {
	if len(ss) == 0 {
		return ss
	}
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
