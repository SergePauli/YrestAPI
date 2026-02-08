package resolver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"YrestAPI/internal/db"
	"YrestAPI/internal/logger"
	"YrestAPI/internal/model"
)

// Главный резолвер
func Resolver(ctx context.Context, req IndexRequest) ([]map[string]any, error) {
	// 0) модель и aliasMap
	m, ok := model.Registry[req.Model]
	if !ok {
		return nil, fmt.Errorf("resolver: model not found: %s", req.Model)
	}
	// Получаем карту алиасов из кэша или строим на лету
	var preset *model.DataPreset
	if req.Preset != "" {
		preset = m.GetPreset(req.Preset)
	} else {
		preset = req.PresetObj
	}
	if preset == nil {
		return nil, fmt.Errorf("preset not found: %s.%s", req.Model, req.Preset)
	}
	filters := model.NormalizeFiltersWithAliases(m, req.Filters)
	sorts := model.NormalizeSortsWithAliases(m, req.Sorts)

	aliasMap, err := m.CreateAliasMap(m, preset, filters, sorts)
	if err != nil {
		logger.Error("alias_map_error", map[string]any{
			"model":  req.Model,
			"preset": req.Preset,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("alias map error: %s", err)
	}

	// 1) главный SELECT
	sb, err := m.BuildIndexQuery(aliasMap, filters, sorts, preset, req.Offset, req.Limit)
	if err != nil {
		return nil, err
	}

	sqlStr, args, err := sb.ToSql()
	if err != nil {
		return nil, err
	}
	//
	logger.Debug("sql", map[string]any{
		"endpoint": "/api/index",
		"sql":      sqlStr,
		"args":     args,
	})

	rows, err := db.Pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// функция, восстанавливающая поля из aliasMap
	items, err := m.ScanFlatRows(rows, preset, aliasMap)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}

	// 4) определяем хвосты из пресета (рекурсивно по belongs_to)
	polyTails := collectPolyTails(m, preset, "")
	tails := collectTails(m, preset /*prefix*/, "")
	if len(tails) == 0 && len(polyTails) == 0 {
		// ⬅️ Хвостов нет — сразу финализируем и выходим
		if err := finalizeItems(m, preset, items); err != nil {
			logger.Error("resolver_finalize_error", map[string]any{
				"model":  req.Model,
				"preset": req.Preset,
				"error":  err.Error(),
				"items":  items,
			})
			return nil, fmt.Errorf("resolver: finalize: %w", err)
		}
		return items, nil
	}

	// 3) Собираем parentIDs отдельно для КАЖДОГО хвоста, используя rel.PK
	type idSet map[any]struct{}
	parentIDsByTail := make(map[string][]any) // FieldAlias -> []id

	for _, t := range tails {
		// ключ родителя, который участвует в связи
		pkName := t.Rel.PK
		seen := make(idSet)
		ids := make([]any, 0, len(items))
		for _, it := range items {
			ctx := getTargetContext(it, t.TargetPath)
			if ctx == nil {
				continue
			}
			if v, ok := ctx[pkName]; ok && v != nil {
				if _, exists := seen[v]; !exists {
					seen[v] = struct{}{}
					ids = append(ids, v)
				}
			}
		}
		parentIDsByTail[t.FieldAlias] = ids
	}

	type grouped = map[any][]map[string]any
	groupedByAlias := make(map[string]grouped)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var rerr error

	for _, t := range tails {
		t := t // avoid capturing loop variable in goroutine
		ids := parentIDsByTail[t.FieldAlias]
		if len(ids) == 0 {
			continue
		}
		wg.Add(1)
		go func(ids []any) {
			defer wg.Done()

			childModel := t.Rel.GetModelRef()
			if childModel == nil {
				mu.Lock()
				rerr = fmt.Errorf("tail '%s': child model '%s' not found", t.FieldAlias, t.Rel.Model)
				mu.Unlock()
				return
			}

			// вложенный пресет берём ПО ССЫЛКЕ МОДЕЛИ СВЯЗИ
			var childPreset *model.DataPreset
			if t.NestedPreset != "" {
				childPreset = childModel.Presets[t.NestedPreset]
				if childPreset == nil {
					mu.Lock()
					rerr = fmt.Errorf("tail '%s': nested preset '%s' not found in model '%s'",
						t.FieldAlias, t.NestedPreset, childModel.Table)
					mu.Unlock()
					return
				}
			}

			// формируем фильтр для дочернего резолвера
			childFilters := map[string]any{}
			fk := t.Rel.FK

			// Лимит 1 для has_one, иначе maxLimit
			limit := uint64(maxLimit)
			if t.Rel.Where != "" {
				if key, val, ok := parseCondition(t.Rel.Where); ok {
					childFilters[key] = val
				}
			}

			// рекурсивный вызов того же Resolver
			var childReq IndexRequest
			if t.Rel.Through == "" {
				// прямой has_
				synthetic := makeSyntheticPreset(childPreset, fk)
				childFilters[fk+"__in"] = ids
				childReq = IndexRequest{
					Model:     t.Rel.Model,
					Preset:    "",
					Filters:   childFilters,
					Sorts:     buildOrderSorts(t.Rel.Order /*prefix*/, ""),
					Offset:    0,
					Limit:     limit, // has_one отберём первый после группировки
					PresetObj: synthetic,
				}
			} else {
				childReq, err = MakeThroughChildRequest(m, t.Rel, t.NestedPreset, ids)
				if err != nil {
					mu.Lock()
					rerr = fmt.Errorf("tail '%s': %w", t.FieldAlias, err)
					mu.Unlock()
					return
				}
			}
			childItems, err := Resolver(ctx, childReq)
			if err != nil {
				mu.Lock()
				rerr = fmt.Errorf("tail '%s': %w", t.FieldAlias, err)
				mu.Unlock()
				return
			}
			// сгруппируем дочерние по FK (он указывает на родителя)
			g := make(grouped)
			for _, row := range childItems {
				pid := row[fk]
				g[pid] = append(g[pid], row)
			}

			mu.Lock()
			groupedByAlias[t.FieldAlias] = g
			mu.Unlock()
		}(ids)
	}

	wg.Wait()
	if rerr != nil {
		logger.Error("resolver_tail_error", map[string]any{
			"model":  req.Model,
			"preset": req.Preset,
			"error":  rerr.Error(),
			"items":  items,
		})
		return nil, rerr
	}

	// 5) Собираем итоговые элементы, склеивая хвосты по алиасам
	for i := range items {
		for _, t := range tails {
			ctx := getTargetContext(items[i], t.TargetPath)
			if ctx == nil {
				continue
			}
			pid := ctx[t.Rel.PK]

			// Определяем целевой контекст для записи: либо указанный TargetPath, либо корень item
			target := ensureTargetContext(items[i], t.TargetPath)

			// Получаем группы дочерних записей
			var groups []map[string]any
			if m, ok := groupedByAlias[t.FieldAlias]; ok {
				if g, ok := m[pid]; ok {
					groups = g
				}
			}

			if len(groups) == 0 {
				if t.LimitOne {
					target[t.FieldAlias] = nil
				} else {
					target[t.FieldAlias] = []any{}
				}
				continue
			}

			if t.LimitOne {
				if strings.TrimSpace(t.Formatter) != "" {
					// Легаси: если вдруг задан форматтер — применяем к первой записи
					target[t.FieldAlias] = applyFormatter(t.Formatter, groups[0])
				} else {
					delete(groups[0], t.Rel.FK)
					target[t.FieldAlias] = groups[0]
				}
			} else {
				if strings.TrimSpace(t.Formatter) != "" {
					out := make([]string, len(groups))
					for idx, row := range groups {
						out[idx] = applyFormatter(t.Formatter, row)
					}
					target[t.FieldAlias] = out
				} else {
					for _, row := range groups {
						delete(row, t.Rel.FK)
					}
					target[t.FieldAlias] = groups
				}
			}
		}
	}

	// 5.5) Полиморфные belongs_to: группы запросов по типу
	for _, t := range polyTails {
		typeCol := t.Rel.TypeColumn
		if strings.TrimSpace(typeCol) == "" {
			typeCol = t.FieldAlias + "_type"
		}
		fk := t.Rel.FK

		byType := map[string][]any{}
		seen := map[string]map[any]struct{}{}
		for _, it := range items {
			tv, ok := it[typeCol]
			if !ok || tv == nil {
				continue
			}
			iv, ok := it[fk]
			if !ok || iv == nil {
				continue
			}
			typ := fmt.Sprint(tv)
			if _, ok := seen[typ]; !ok {
				seen[typ] = map[any]struct{}{}
			}
			if _, ok := seen[typ][iv]; ok {
				continue
			}
			seen[typ][iv] = struct{}{}
			byType[typ] = append(byType[typ], iv)
		}

		typeGrouped := map[string]map[any]map[string]any{}
		for typ, ids := range byType {
			childModel := model.Registry[typ]
			if childModel == nil {
				continue
			}
			childPreset := childModel.Presets[t.NestedPreset]
			if childPreset == nil {
				continue
			}
			filters := map[string]any{
				t.Rel.PK + "__in": ids,
			}
			childReq := IndexRequest{
				Model:   typ,
				Preset:  childPreset.Name,
				Filters: filters,
				Limit:   uint64(len(ids)),
			}
			childItems, err := Resolver(ctx, childReq)
			if err != nil {
				return nil, fmt.Errorf("polymorphic tail '%s': %w", t.FieldAlias, err)
			}
			g := map[any]map[string]any{}
			for _, row := range childItems {
				idv := row[t.Rel.PK]
				g[idv] = row
			}
			typeGrouped[typ] = g
		}

		for i := range items {
			tv, ok := items[i][typeCol]
			if !ok || tv == nil {
				continue
			}
			iv, ok := items[i][fk]
			if !ok || iv == nil {
				continue
			}
			typ := fmt.Sprint(tv)

			target := items[i]
			if tp := strings.TrimSpace(t.TargetPath); tp != "" {
				for _, seg := range strings.Split(tp, ".") {
					seg = strings.TrimSpace(seg)
					if seg == "" {
						continue
					}
					if v, ok := target[seg]; ok {
						if m, ok := v.(map[string]any); ok {
							target = m
							continue
						}
					}
					m := map[string]any{}
					target[seg] = m
					target = m
				}
			}

			if g, ok := typeGrouped[typ]; ok {
				if row, ok := g[iv]; ok {
					delete(row, t.Rel.PK)
					target[t.FieldAlias] = row
					continue
				}
			}
			target[t.FieldAlias] = nil
		}
	}

	// 8) финализация formatter/computed уже ПОСЛЕ склейки
	if err := finalizeItems(m, preset, items); err != nil {
		return nil, fmt.Errorf("resolver: finalize: %w", err)
	}

	// 9) for Through: развернём вложенные preset-поля
	if req.PresetObj != nil && req.UnwrapField != "" {
		// fk ты уже знаешь (это Source первого поля синтетического пресета)
		fk := req.PresetObj.Fields[0].Source
		items = unwrapThrough(items, fk, req.UnwrapField)
		return items, nil
	}
	return items, nil
}

type TailSpec struct {
	FieldAlias   string // ключ в JSON (имя поля пресета)
	RelKey       string // ключ связи в родительской модели
	Rel          *model.ModelRelation
	NestedPreset string // как в YAML (Model.Preset или Preset)
	LimitOne     bool
	TargetPath   string // если задано, кладём результат в TargetPathCtx[FieldAlias]
	Formatter    string // возможно мусорное поле,
	// так как форматтеры на has_one/has_many считаются в дочерних вызовах резолвера
}

type PolyTailSpec struct {
	FieldAlias   string
	Rel          *model.ModelRelation
	NestedPreset string
	TargetPath   string
}

// Собираем полиморфные belongs_to из пресета (рекурсивно по обычным belongs_to)
func collectPolyTails(m *model.Model, p *model.DataPreset, prefix string) []PolyTailSpec {
	out := []PolyTailSpec{}
	if p == nil {
		return out
	}
	for _, f := range p.Fields {
		if f.Type != "preset" {
			continue
		}
		rel, ok := m.Relations[f.Source]
		if !ok || rel == nil {
			continue
		}
		if rel.Type == "belongs_to" && rel.Polymorphic {
			alias := f.Alias
			if strings.TrimSpace(alias) == "" {
				alias = f.Source
			}
			out = append(out, PolyTailSpec{
				FieldAlias:   alias,
				Rel:          rel,
				NestedPreset: f.NestedPreset,
				TargetPath:   prefix,
			})
			continue
		}

		if rel.Type == "belongs_to" && rel.GetModelRef() != nil {
			var nested *model.DataPreset
			if f.GetPresetRef() != nil {
				nested = f.GetPresetRef()
			} else if f.NestedPreset != "" {
				nested = rel.GetModelRef().Presets[f.NestedPreset]
			}
			if nested != nil {
				nextPrefix := f.Source
				if prefix != "" {
					nextPrefix = prefix + "." + f.Source
				}
				out = append(out, collectPolyTails(rel.GetModelRef(), nested, nextPrefix)...)
			}
		}
	}
	return out
}

// Собираем хвосты (has_one/has_many) из пресета, рекурсивно проходя belongs_to
func collectTails(m *model.Model, p *model.DataPreset, prefix string) []TailSpec {
	out := []TailSpec{}
	if p == nil {
		return out
	}

	for _, f := range p.Fields {
		if f.Type != "preset" {
			continue
		}
		rel, ok := m.Relations[f.Source]
		if !ok || rel == nil {
			continue
		}

		switch rel.Type {
		case "belongs_to":
			// рекурсия только если есть nested пресет
			nestedModel := rel.GetModelRef()
			var nested *model.DataPreset
			if nestedModel != nil {
				if f.GetPresetRef() != nil {
					nested = f.GetPresetRef()
				} else if f.NestedPreset != "" {
					nested = nestedModel.Presets[f.NestedPreset]
				}
			}
			if nested != nil {
				// углубляемся по source (контейнер belongs_to живёт под source)
				nextPrefix := f.Source
				if prefix != "" {
					nextPrefix = prefix + "." + f.Source
				}
				out = append(out, collectTails(nestedModel, nested, nextPrefix)...)
			}

		case "has_one", "has_many":
			alias := f.Alias
			if strings.TrimSpace(alias) == "" {
				alias = f.Source
			}
			out = append(out, TailSpec{
				FieldAlias:   alias,
				RelKey:       f.Source,
				Rel:          rel,
				NestedPreset: f.NestedPreset,
				LimitOne:     rel.Type == "has_one",
				TargetPath:   prefix,                         // писать в контекст текущей ветки пресета
				Formatter:    strings.TrimSpace(f.Formatter), // не используется здесь, но сохраняем
			})
		}
	}
	return out
}

// getTargetContext returns the nested map by targetPath or the root item if path is empty.
// Returns nil if any segment is missing or not a map.
func getTargetContext(item map[string]any, targetPath string) map[string]any {
	if strings.TrimSpace(targetPath) == "" {
		return item
	}
	ctx := item
	for _, seg := range strings.Split(targetPath, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		v, ok := ctx[seg]
		if !ok {
			return nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		ctx = m
	}
	return ctx
}

// ensureTargetContext walks/creates nested maps for targetPath and returns the map.
func ensureTargetContext(item map[string]any, targetPath string) map[string]any {
	if strings.TrimSpace(targetPath) == "" {
		return item
	}
	target := item
	for _, seg := range strings.Split(targetPath, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if v, ok := target[seg]; ok {
			if m, ok := v.(map[string]any); ok {
				target = m
				continue
			}
		}
		m := map[string]any{}
		target[seg] = m
		target = m
	}
	return target
}

// req.UnwrapField: например, "contact"
// fk: имя FK, которое ты положил в синтетический пресет (например "person_id")

func unwrapThrough(items []map[string]any, fk, unwrap string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		// самый простой путь: ожидаем map у it[unwrap]
		if payload, ok := it[unwrap].(map[string]any); ok && len(payload) > 0 {
			row := make(map[string]any, len(payload)+1)
			row[fk] = it[fk] // оставить ключ для группировки родителем
			for k, v := range payload {
				row[k] = v
			}
			out = append(out, row)
			continue
		}
	}
	return out
}
