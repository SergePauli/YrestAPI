package model

import (
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
)

// expandPathWithAliases разворачивает алиасы в пути, двигаясь по связям модели.
func expandPathWithAliases(m *Model, path string) string {
	if m == nil || strings.TrimSpace(path) == "" {
		return path
	}
	segs := strings.Split(path, ".")
	curr := m
	for i := 0; i < len(segs); i++ {
		seg := segs[i]
		rel := curr.Relations[seg]
		if rel == nil || rel.Polymorphic || rel._ModelRef == nil {
			remaining := strings.Join(segs[i:], ".")
			expanded := ExpandAliasPath(curr, remaining)
			if expanded != remaining {
				newSegs := append([]string{}, segs[:i]...)
				newSegs = append(newSegs, strings.Split(expanded, ".")...)
				segs = newSegs
				i--
				continue
			}
			break
		}
		curr = rel._ModelRef
	}
	return strings.Join(segs, ".")
}

// BuildIndexQuery строит SELECT-запрос для /index эндпоинта
func (m *Model) BuildIndexQuery(
	aliasMap *AliasMap, // карта алиасов
	filters map[string]interface{},
	sorts []string, // array сортировок: ["field1 ASC", "field2 DESC"]
	preset *DataPreset, // выбранный пресет
	offset, limit uint64, // пагинация
) (squirrel.SelectBuilder, error) {

	sb := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)

	// 1. FROM
	sb = sb.From(fmt.Sprintf("%s AS main", m.Table))

	// 2. Определяем список полей для выборки c учётом пресета
	if preset == nil {
		return sb, fmt.Errorf("preset is nil for model '%s'", m.Table)
	}

	// 3. Определяем JOIN-ы по всем фильтрам, включая вложенные or/and-группы.
	filterKeys := PathsFromFilters(filters)

	sortFields := make([]string, len(sorts))
	for i, s := range sorts {
		parts := strings.SplitN(s, " ", 2)
		sortFields[i] = parts[0]
	}
	presetFieldPaths := m.ScanPresetFields(preset, "")
	compPaths := collectComputablePathsForRequest(m, preset, filters, sorts)
	if len(compPaths) > 0 {
		presetFieldPaths = append(presetFieldPaths, compPaths...)
	}
	joinSpecs, err := m.DetectJoins(aliasMap, filterKeys, sortFields, presetFieldPaths)

	if err != nil {
		return sb, err
	}
	cteSpecs, computableOverride, skipAliases := buildHasManyCTEs(m, preset, filters, sorts, aliasMap, joinSpecs)
	if len(cteSpecs) > 0 {
		prefixSQL, prefixArgs, err := buildCTEQueries(m, cteSpecs)
		if err != nil {
			return sb, err
		}
		sb = sb.Prefix(prefixSQL, prefixArgs...)
		for _, spec := range cteSpecs {
			sb = sb.LeftJoin(fmt.Sprintf("%s ON %s.id = main.id", spec.Name, spec.Name))
		}
	}
	joinSpecs = filterJoinSpecs(joinSpecs, skipAliases)

	hasDistinct := false
	for i := 0; i < len(joinSpecs); i++ {
		join := joinSpecs[i]
		onClause := join.On
		if join.Where != "" {
			onClause = fmt.Sprintf("(%s) AND (%s)", join.On, join.Where)
		}
		sb = sb.LeftJoin(fmt.Sprintf("%s AS %s ON %s", join.Table, join.Alias, onClause))
		if join.Distinct {
			hasDistinct = true
		}
	}
	// 3.1. Добавляем поля из пресета
	selectCols := m.ScanColumns(preset, aliasMap, "")
	if len(computableOverride) > 0 && preset != nil {
		overrideByAlias := make(map[string]string, len(computableOverride))
		for _, f := range preset.Fields {
			if f.Type != "computable" {
				continue
			}
			aliasName := strings.TrimSpace(f.Alias)
			if aliasName == "" {
				aliasName = f.Source
			}
			if expr, ok := computableOverride[f.Source]; ok {
				overrideByAlias[aliasName] = fmt.Sprintf("%s AS %s", expr, quoteIdentifier(aliasName))
			}
		}
		for i := range selectCols {
			if expr, ok := overrideByAlias[selectCols[i].Key]; ok {
				selectCols[i].Expr = expr
			}
		}
	}

	colExprs := make([]string, 0, len(selectCols))
	for _, c := range selectCols {
		colExprs = append(colExprs, c.Expr)
	}
	// если есть has_many JOIN — будем группировать по простым колонкам, чтобы агрегаты в computable работали
	groupByCols := make([]string, 0)
	if hasDistinct {
		for _, expr := range colExprs {
			if isSimpleColumnExpr(expr) {
				groupByCols = append(groupByCols, baseColumnExpr(expr))
			}
		}
		if aliasMap != nil && len(computableOverride) == 0 {
			hasManyAliases := make(map[string]struct{})
			for path, alias := range aliasMap.PathToAlias {
				if isHasManyPath(m, path) {
					hasManyAliases[alias] = struct{}{}
				}
			}
			compExprs := collectComputableExprsForRequest(m, preset, filters, sorts, aliasMap)
			for _, expr := range compExprs {
				for _, col := range extractQualifiedColumns(expr, hasManyAliases) {
					if !containsString(groupByCols, col) {
						groupByCols = append(groupByCols, col)
					}
				}
			}
		}
	}

	// 4. WHERE фильтры
	whereBuilder, havingBuilder, err := m.buildWhereClause(aliasMap, preset, filters, joinSpecs, computableOverride)
	if err != nil {
		return sb, err
	}
	if whereBuilder != nil {
		sb = sb.Where(whereBuilder)
	}
	if havingBuilder != nil {
		sb = sb.Having(havingBuilder)
	}

	// 5. ORDER BY
	existingSelect := make(map[string]struct{}, len(colExprs))
	normalizeExpr := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}
	for _, expr := range colExprs {
		existingSelect[normalizeExpr(expr)] = struct{}{}
	}

	orderExprs := make([]string, 0, len(sorts))
	for _, s := range sorts {
		parts := strings.SplitN(s, " ", 2) // [path, direction?]
		fieldPath := expandPathWithAliases(m, parts[0])
		dir := ""
		if len(parts) > 1 {
			dir = strings.TrimSpace(parts[1])
		}

		addSelectExpr := func(expr string) {
			if !hasDistinct || expr == "" {
				return
			}
			key := normalizeExpr(expr)
			if _, ok := existingSelect[key]; ok {
				return
			}
			selectCols = append(selectCols, SelectColumn{Expr: expr, Key: "", Type: ""})
			colExprs = append(colExprs, expr)
			existingSelect[key] = struct{}{}
			if isSimpleColumnExpr(expr) {
				col := baseColumnExpr(expr)
				if !containsString(groupByCols, col) {
					groupByCols = append(groupByCols, col)
				}
			}
		}

		if expr, ok := computableOverride[fieldPath]; ok {
			orderExpr := expr
			if dir != "" {
				orderExpr += " " + dir
			}
			orderExprs = append(orderExprs, orderExpr)
			addSelectExpr(expr)
			continue
		}

		handled := false

		// если сортировка совпадает с alias поля пресета — сортируем по нему
		if preset != nil {
			for _, f := range preset.Fields {
				aliasName := strings.TrimSpace(f.Alias)
				if aliasName == "" {
					aliasName = f.Source
				}
				if aliasName == fieldPath {
					if expr, ok := m.resolveFieldExpression(preset, aliasMap, f.Source); ok {
						baseExpr := strings.TrimSpace(expr)
						if baseExpr == "" {
							continue
						}
						orderExpr := baseExpr
						if dir != "" {
							orderExpr += " " + dir
						}
						orderExprs = append(orderExprs, orderExpr)
						addSelectExpr(baseExpr)
						handled = true
						break
					}
				}
			}
		}
		if handled {
			continue
		}

		if expr, ok := m.resolveFieldExpression(preset, aliasMap, fieldPath); ok {
			baseExpr := strings.TrimSpace(expr)
			orderExpr := baseExpr
			if dir != "" {
				orderExpr += " " + dir
			}
			orderExprs = append(orderExprs, orderExpr)
			addSelectExpr(baseExpr)
			continue
		}

		// ищем последний "."
		idx := strings.LastIndex(fieldPath, ".")
		if idx == -1 {
			// если точек нет — пропускаем
			continue
		}

		presetPath := fieldPath[:idx]  // всё до последней точки
		fieldName := fieldPath[idx+1:] // всё после последней точки

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
		orderExprs = append(orderExprs, orderExpr)
		addSelectExpr(fmt.Sprintf("%s.%s", alias, fieldName))
	}

	sb = sb.Columns(colExprs...)
	if hasDistinct && len(groupByCols) > 0 {
		sb = sb.GroupBy(groupByCols...)
	} else if hasDistinct {
		sb = sb.Distinct()
	}

	for _, expr := range orderExprs {
		sb = sb.OrderBy(expr)
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

// isSimpleColumnExpr определяет, является ли выражение простым обращением к колонке alias.column (с кавычками/без).
func isSimpleColumnExpr(expr string) bool {
	e := strings.TrimSpace(expr)
	if e == "" {
		return false
	}
	// убираем возможный AS alias хвост
	if idx := strings.LastIndex(strings.ToLower(e), " as "); idx > 0 {
		e = strings.TrimSpace(e[:idx])
	}
	// простейшие варианты: alias.col, "alias".col, alias."col", "alias"."col"
	if strings.ContainsAny(e, " ()+") {
		return false
	}
	dot := strings.IndexByte(e, '.')
	if dot <= 0 || dot >= len(e)-1 {
		return false
	}
	return true
}

func baseColumnExpr(expr string) string {
	e := strings.TrimSpace(expr)
	if e == "" {
		return e
	}
	if idx := strings.LastIndex(strings.ToLower(e), " as "); idx > 0 {
		e = strings.TrimSpace(e[:idx])
	}
	return e
}
