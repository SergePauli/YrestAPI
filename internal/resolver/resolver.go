package resolver

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"

	"YrestAPI/internal/model"
)

type ResolverResult struct {
	data []map[string]any
	Err  error
}

// CastToMapSlice пытается привести []any к []map[string]any с безопасной проверкой.
// Возвращает ошибку, если хотя бы один элемент не является map[string]any.
func CastToMapSlice(raw []any) ([]map[string]any, error) {
	results := make([]map[string]any, len(raw))
	for i, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("CastToMapSlice: element at index %d is not map[string]any", i)
		}
		results[i] = m
	}
	return results, nil
}

// extractPrimaryIDsAndCache считывает все строки один раз,
// возвращает ID-шники и кэшированные строки для повторного использования
// hasFields - это карта полей, которые имеют has_many отношения
// Возвращает:
// - map[string][]any: ID-шники для каждого has_many поля
// - []map[string]any: кэшированные строки с полными данными


func extractPrimaryIDsAndCache(
	rows pgx.Rows,
	hasFields map[string]*model.Field,
) (map[string][]any, []map[string]any, error) {
	defer rows.Close()

	// Подготовка
	pkSets := make(map[string]map[any]bool)      // для уникальных ID по каждому PKField
	pkLists := make(map[string][]any)            // итоговые ID
	cachedRows := make([]map[string]any, 0)

	for key := range hasFields {
		pkSets[key] = make(map[any]bool)
	}

	for rows.Next() {
		values, err := rows.Values()		
		if err != nil {
			return nil, nil, fmt.Errorf("rows.Values: %w", err)
		}

		fields := rows.FieldDescriptions()
		row := make(map[string]any)

		// Построим row и соберем PK-значения
		for i, fd := range fields {
			col := string(fd.Name)
			val := values[i]
			row[col] = val
		}
		
		// Обработка всех PKField
		for alias, _ := range hasFields {			
			pk := "id"    	
    	if val, ok := row[pk]; ok && val != nil {
        if !pkSets[alias][val] {
            pkSets[alias][val] = true
            pkLists[alias] = append(pkLists[alias], val)
        }
    	}
		}

		cachedRows = append(cachedRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("rows.Err: %w", err)
	}

	return pkLists, cachedRows, nil
}

// groupBy группирует данные по значению ключа
func groupBy(data []map[string]any, key string) map[string][]map[string]any {
	grouped := make(map[string][]map[string]any)
	for _, item := range data {
		val, ok := item[key]
		if !ok {
			continue
		}
		strKey := fmt.Sprintf("%v", val)
		grouped[strKey] = append(grouped[strKey], item)
	}
	return grouped
}


// Resolver выполняет запрос к базе данных с использованием пресета и возвращает результат
// Использует параллельную загрузку has_many полей
func Resolver(ctx context.Context, req IndexRequest) ([]map[string]any, error) {
	// 0. Получаем модель
	m, ok := model.Registry[req.Model]
	if !ok {
		return nil, fmt.Errorf("resolver: model not found: %s", req.Model)
	}
	log.Printf("Resolver: model %s found", m)
	// aliasMap, err := m.GetAliasMapFromRedisOrBuild(ctx, req.Model)
	// if err != nil {
	// 	return nil, fmt.Errorf("resolver: alias map error: %w", err)
	// }

	
	// // 1. Строим SQL (включая JOIN и has_one пресеты)
	// main_query, hasFields:= p.BuildQuery(req.Filters, req.Sorts, req.Offset, req.Limit) 
	// // 2. Выполняем запрос
	// sqlStr, args, err := main_query.ToSql()
	// if err != nil {		
	// 	return nil, fmt.Errorf("resolver: Failed to build SQL: %w", err)
	// }
	// log.Printf("Executing query: %s\nARGS: %#v\n", sqlStr, args)
	
	// rows, err := db.Conn.Query(context.Background(), sqlStr, args...)
	// if err != nil {		
	// 	return nil, fmt.Errorf("resolver: DB error: %w", err)
	// }
	// defer rows.Close()

	
	// if len(hasFields) == 0 {
	// 	// Если нет has_many полей, просто сканируем в JSON
	// 	raw, err := p.ParseRowsToJSON(rows)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("resolver: scan error: %w", err)
	// 	}		
	// 	return raw, nil
	// } else {
	// 		// Если есть has_many поля, то:			
	// 		// 1. Сканируем родительскую выборку в кэш + собираем ID-шники
	// 		idMap, cachedRows, err := extractPrimaryIDsAndCache(rows, hasFields)								
	// 		if err != nil {
	// 			return nil, fmt.Errorf("extractPrimaryIDsAndCache: %w", err)
	// 		}
			
	// 		var (
	// 			wg           sync.WaitGroup
	// 			mu           sync.Mutex
	// 			nestedErr    error
	// 			hasManyData  = make(map[string]map[string][]map[string]any)
	// 		)

	// 		// 2. Параллельно загружаем has_many данные
	// 		for _, field := range hasFields {
	// 			nestedPresetName := field.NestedPreset
	// 			if nestedPresetName == "" {
	// 				continue
	// 			}

	// 			ids := idMap[field.Alias]
	// 			if len(ids) == 0 {
	// 				continue
	// 			}
	// 			parts := strings.SplitN(nestedPresetName, ".", 2)
	// 			if len(parts) != 2 {
	// 				log.Printf("Некорректный NestedPreset: %s", nestedPresetName)
	// 				continue
	// 			}				
	// 			wg.Add(1)
	// 			// Запускаем горутину для загрузки has_many данных
	// 			go func() {
	// 				defer wg.Done()// Завершаем горутину
	// 				// Формируем фильтры для вложенного resolver'а
	// 				nestedReq := IndexRequest{
	// 					Model: 		 parts[0],
	// 					Preset:  	parts[1],
	// 					Filters: map[string]any{
	// 						field.FKField+"__in": ids,
	// 					},					
	// 					Offset: 0,
	// 				}				
	// 				if field.Sorts != nil {
	// 					nestedReq.Sorts = field.Sorts
	// 				} 
				
	// 				// Вызов рекурсивного Resolver
	// 				nestedResult, err := Resolver(ctx, nestedReq)
	// 				if err != nil {
	// 					mu.Lock()// Лок для безопасного доступа к shared переменной
	// 					nestedErr = fmt.Errorf("resolver: nested preset '%s' error: %w", nestedPresetName, err)
	// 					mu.Unlock()
	// 					return
	// 				}
	// 				grouped := groupBy(nestedResult, field.FKField)
	// 				mu.Lock()
	// 				// Сохраняем в hasManyData по JSON alias-ключу
	// 				hasManyData[field.Alias] = grouped
	// 				mu.Unlock()// Освобождаем лок
	// 			}()	
	// 		}
	// 		wg.Wait()// Ждем завершения всех горутин
	// 		if nestedErr != nil {
	// 			return nil, nestedErr
	// 		}
	// 		// 3. Преобразуем кэшированные строки в JSON с учетом has_many данных
	// 		raw, err := p.DataToJSON(cachedRows, hasManyData)
	// 		if err != nil {
	// 			return nil, fmt.Errorf("resolver: scan error: %w", err)
	// 		}
	// 		// Приводим []any → []map[string]any
	// 		results, err := CastToMapSlice(raw)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 	return results, nil
	// }
	return nil, fmt.Errorf("resolver: not implemented yet")
}
