package model

import (
	"fmt"

	"strings"

	"github.com/Masterminds/squirrel"
)

// BuildIndexQuery строит SELECT-запрос для /index эндпоинта
func (m *Model) BuildIndexQuery(
	aliasMap *AliasMap, // карта алиасов 
	filters map[string]interface{},
	sorts []string,       // array сортировок: ["field1 ASC", "field2 DESC"]
	preset *DataPreset,        // выбранный пресет
	offset, limit uint64,   // пагинация
) (squirrel.SelectBuilder, error) {

	sb := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)

	// 1. FROM
	sb = sb.From(fmt.Sprintf("%s AS main", m.Table))

	// 2. Определяем список полей для выборки c учётом пресета
	
	
	if preset == nil {
		
			return sb, fmt.Errorf("preset is nil for model '%s'",  m.Table)
		
	}

	// 3. Определяем JOIN-ы
	var filterKeys []string
	for key := range filters {
		filterKeys = append(filterKeys, key)
	}
	
	sortFields := make([]string, len(sorts))
	for i, s := range sorts {
    parts := strings.SplitN(s, " ", 2)
    sortFields[i] = parts[0]
	}	
	presetFieldPaths := m.ScanPresetFields(preset, "")		
	joinSpecs, err := m.DetectJoins(aliasMap, filterKeys, sortFields, presetFieldPaths)
		
	if err != nil {
		return sb, err
	}
	
	hasDistinct := false
	for i := 0; i < len(joinSpecs); i++ {
    join := joinSpecs[i]  		
		onClause := join.On
    if join.Where != "" {
        onClause = fmt.Sprintf("(%s) AND (%s)", join.On, join.Where)
    }
		sb = sb.LeftJoin(fmt.Sprintf("%s AS %s ON %s", join.Table, join.Alias, onClause ))
		if join.Distinct {
			hasDistinct = true
		}
	}
	// 3.1. Добавляем поля из пресета	
	 selectCols,_ := m.ScanColumns(preset, aliasMap, "")	
	
	if hasDistinct {
    pkFields := m.GetPrimaryKeys() // []string, например ["person.id", "person.code"]
		if len(pkFields) == 1 {
       // простой DISTINCT по всей строке
        sb = sb.Distinct()
    } else if len(pkFields) > 1 {
        // Составной ключ — DISTINCT ON
        distinctExpr := fmt.Sprintf("DISTINCT ON (%s)", strings.Join(pkFields, ", "))
        // В PostgreSQL DISTINCT ON должен стоять в начале списка колонок
        selectCols = append([]string{distinctExpr}, selectCols...)
    }
	}	

	sb = sb.Columns(selectCols...)	

	// 4. WHERE фильтры
	whereBuilder, err := m.buildWhereClause(aliasMap, filters, joinSpecs)
	if err != nil {
		return sb, err
	}
	if whereBuilder != nil {
		sb = sb.Where(whereBuilder)
	}

	// 5. ORDER BY
	for _, s := range sorts {
    parts := strings.SplitN(s, " ", 2) // [path, direction?]
    fieldPath := parts[0]
    dir := ""
    if len(parts) > 1 {
        dir = strings.TrimSpace(parts[1])
    }

    // ищем последний "."
    idx := strings.LastIndex(fieldPath, ".")
    if idx == -1 {
        // если точек нет — пропускаем
        continue
    }

    presetPath := fieldPath[:idx]     // всё до последней точки
    fieldName  := fieldPath[idx+1:]  // всё после последней точки

    // Подмена пресета на алиас
    alias, ok := aliasMap.PathToAlias[presetPath]
    if !ok {
        alias = presetPath // fallback — без подмены
    }

    // Финальное выражение для ORDER BY
    orderExpr := fmt.Sprintf("%s.%s", alias, fieldName)
    if dir != "" {
        orderExpr += " " + dir
    }
    sb = sb.OrderBy(orderExpr)
	}

	// 6. LIMIT / OFFSET
	if limit > 0 {
		sb = sb.Limit(limit)
	}
	if offset > 0 {
		sb = sb.Offset(offset)
	}

	return sb, nil
}
