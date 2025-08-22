package resolver

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"YrestAPI/internal/db"
	"YrestAPI/internal/model"
)

// Главный резолвер
func Resolver(ctx context.Context, req IndexRequest) ([]map[string]any, error) {
    // 0) модель и aliasMap
    m, ok := model.Registry[req.Model]
    if !ok { return nil, fmt.Errorf("resolver: model not found: %s", req.Model) }
    // Получаем карту алиасов из Redis или строим на лету
	err := m.GetAliasMapFromRedisOrBuild(ctx, req.Model)
	if err != nil {
		log.Printf("resolver: alias map error: %v", err)
	 	return nil, fmt.Errorf("alias map error: %s", err)
	}
		var preset *model.DataPreset
		if (req.Preset != "") {
    	preset = m.GetPreset(req.Preset)
		} else {
			preset = req.PresetObj
		}	
    if preset == nil { return nil, fmt.Errorf("preset not found: %s.%s", req.Model, req.Preset) }

    // 1) главный SELECT
    sb, err := m.BuildIndexQuery(req.Filters, req.Sorts, preset, req.Offset, req.Limit)
    if err != nil { return nil, err }

    sqlStr, args, err := sb.ToSql()
    if err != nil { return nil, err }
		//
		log.Println("SQL for index:", sqlStr)
		log.Println("ARGS:", args)

    rows, err := db.Pool.Query(ctx, sqlStr, args...)
    if err != nil { return nil, err }
    defer rows.Close()

		// функция, восстанавливающая поля из aliasMap
    items, err := m.ScanFlatRows(rows, preset) 
    if err != nil { return nil, err }
    if len(items) == 0 { return items, nil }
		

    // 4) определяем хвосты из пресета (рекурсивно по belongs_to)
		tails := collectTails(m, preset)
		if len(tails) == 0 {
				// ⬅️ Хвостов нет — сразу финализируем и выходим
				if err := finalizeItems(m, preset, items); err != nil {
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
        if v, ok := it[pkName]; ok && v != nil {
            if _, exists := seen[v]; !exists {
                seen[v] = struct{}{}
                ids = append(ids, v)
            }
        }
    	}
        parentIDsByTail[t.FieldAlias] = ids
        //log.Printf("parentIDsByTail[%s] = %+v", t.FieldAlias, parentIDsByTail[t.FieldAlias])
	}
        
    type grouped = map[any][]map[string]any
		groupedByAlias := make(map[string]grouped)

		var wg sync.WaitGroup
		var mu sync.Mutex
		var rerr error

		for _, t := range tails {    	
    	ids := parentIDsByTail[t.FieldAlias]
    	if len(ids) == 0 {
        continue
    	}

    	wg.Add(1)
    	go func() {
        defer wg.Done()

        childModel := t.Rel.GetModelRef()       
        if childModel == nil {
            mu.Lock(); rerr = fmt.Errorf("tail '%s': child model '%s' not found", t.FieldAlias, t.Rel.Model); mu.Unlock()
            return
        }

        // вложенный пресет берём ПО ССЫЛКЕ МОДЕЛИ СВЯЗИ
        var childPreset *model.DataPreset
        if t.NestedPreset != ""  {
            childPreset = childModel.Presets[t.NestedPreset]
            if childPreset == nil {
                mu.Lock(); rerr = fmt.Errorf("tail '%s': nested preset '%s' not found in model '%s'",
                    t.FieldAlias, t.NestedPreset, childModel.Table); mu.Unlock()
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
				synthetic := makeSyntheticPreset(childPreset, fk)
        if t.Rel.Through == "" {
            // прямой has_
            childFilters[fk+"__in"] = ids 
        		childReq = IndexRequest{
            	Model:   t.Rel.Model,
            	Preset:  "",
            	Filters: childFilters,
            	Sorts:   nil,
            	Offset:  0,
            	Limit:   limit, // has_one отберём первый после группировки		
							PresetObj: synthetic, 				
        		}
					} else {
						childReq, err = MakeThroughChildRequest(m, t.Rel, t.NestedPreset, ids)
						if err != nil {
							mu.Lock(); rerr = fmt.Errorf("tail '%s': %w", t.FieldAlias, err); mu.Unlock()
							return
						}
					}	
		//log.Printf("Resolver: child request for tail '%s': %+v", t.FieldAlias, childReq)
        childItems, err := Resolver(ctx, childReq)
        if err != nil {
            mu.Lock(); rerr = fmt.Errorf("tail '%s': %w", t.FieldAlias, err); mu.Unlock()
            return
        }
		//log.Printf("Resolver: child items for tail '%s': %+v rows", t.FieldAlias, childItems)
        // сгруппируем дочерние по FK (он указывает на родителя)
        g := make(grouped)
        for _, row := range childItems {
            pid := row[fk]
            g[pid] = append(g[pid], row)
        }

        mu.Lock()
        groupedByAlias[t.FieldAlias] = g
        mu.Unlock()
    	}()
		}

		wg.Wait()
		if rerr != nil {
    return nil, rerr
	}
	// 5) Собираем итоговые элементы, склеивая хвосты по алиасам
	for i := range items {
		for _, t := range tails {
    	pid := items[i][t.Rel.PK]
    	if groups, ok := groupedByAlias[t.FieldAlias][pid]; ok {
				 if t.LimitOne {
                if t.Formatter != "" {
                    items[i][t.FieldAlias] = applyFormatter(t.Formatter, groups[0])
                } else {
                    delete(groups[0], t.Rel.FK)
                    items[i][t.FieldAlias] = groups[0]
                }
            } else {
                if t.Formatter != "" {
                    out := make([]string, len(groups))
                    for idx, row := range groups {
                        out[idx] = applyFormatter(t.Formatter, row)
                    }
                    items[i][t.FieldAlias] = out
                } else {
                    for _, row := range groups {
                        delete(row, t.Rel.FK)
                    }
                    items[i][t.FieldAlias] = groups
                }
          }
    	} else {
        items[i][t.FieldAlias] = nil 
			}
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
    FieldAlias   string        // ключ в JSON (имя поля пресета)
    RelKey       string        // ключ связи в родительской модели
    Rel          *model.ModelRelation
    NestedPreset string        // как в YAML (Model.Preset или Preset)
    LimitOne     bool
		Formatter  string // если задано, то применим formatter к каждому элементу
}
// Собираем хвосты (has_one/has_many) из пресета, рекурсивно проходя belongs_to
func collectTails(m *model.Model, p *model.DataPreset) []TailSpec {
    var out []TailSpec
    if p == nil { return out }
    for _, f := range p.Fields {
        if f.Type != "preset" { continue }
        rel, ok := m.Relations[f.Source]
        if !ok || rel == nil { continue }

        switch rel.Type {
        case "belongs_to":
            // рекурсивно вглубь
            var nested *model.DataPreset
						nestedModel := rel.GetModelRef()
						if nestedModel != nil {            
                nested = nestedModel.Presets[f.NestedPreset]
            }
            if nested != nil  {
                out = append(out, collectTails(nestedModel, nested)...)
            }

        case "has_one", "has_many":
            out = append(out, TailSpec{
                FieldAlias:   f.Alias,
                RelKey:       f.Source,
                Rel:          rel,
                NestedPreset: f.NestedPreset,
								Formatter:    strings.TrimSpace(f.Formatter),
                LimitOne:     rel.Type == "has_one",
            })
        }
    }
    return out
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


