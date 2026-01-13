package model

import (
	"fmt"

	"github.com/Masterminds/squirrel"
)

func (m *Model) BuildCountQuery(aliasMap *AliasMap, preset *DataPreset, filters map[string]interface{}) (squirrel.SelectBuilder, error) {
	// Разворачиваем алиасы, чтобы requiredJoins и WHERE использовали те же пути, что и aliasMap
	filters = NormalizeFiltersWithAliases(m, filters)

	sb := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)
	sb = sb.From(fmt.Sprintf("%s AS main", m.Table))

	var filterKeys []string
	for key := range filters {
		filterKeys = append(filterKeys, key)
	}
	compPaths := collectComputablePathsForRequest(m, preset, filters, nil)
	// Определим, какие JOIN-ы нужны — на основе ключей в filters
	requiredJoins, err := m.DetectJoins(aliasMap, filterKeys, nil, compPaths)
	if err != nil {
		return sb, err
	}
	cteSpecs, computableOverride, skipAliases := buildHasManyCTEs(m, preset, filters, nil, aliasMap, requiredJoins)
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
	requiredJoins = filterJoinSpecs(requiredJoins, skipAliases)

	hasDistinct := false
	for i := 0; i < len(requiredJoins); i++ {
		join := requiredJoins[i]
		onClause := join.On
		if join.Where != "" {
			onClause = fmt.Sprintf("(%s) AND (%s)", join.On, join.Where)
		}
		sb = sb.LeftJoin(fmt.Sprintf("%s AS %s ON %s", join.Table, join.Alias, onClause))
		if join.Distinct {
			hasDistinct = true
		}
	}
	// Подставляем SELECT COUNT(...) в зависимости от наличия has_many
	if hasDistinct {
		sb = sb.Column(fmt.Sprintf("COUNT(DISTINCT %s.id)", "main"))
	} else {
		sb = sb.Column("COUNT(*)")
	}

	wherePart, havingPart, err := m.buildWhereClause(aliasMap, preset, filters, requiredJoins, computableOverride)
	if err != nil {
		return sb, err
	}
	if havingPart == nil {
		if wherePart != nil {
			sb = sb.Where(wherePart)
		}
		return sb, nil
	}

	inner := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)
	inner = inner.From(fmt.Sprintf("%s AS main", m.Table))
	if len(cteSpecs) > 0 {
		prefixSQL, prefixArgs, err := buildCTEQueries(m, cteSpecs)
		if err != nil {
			return sb, err
		}
		inner = inner.Prefix(prefixSQL, prefixArgs...)
		for _, spec := range cteSpecs {
			inner = inner.LeftJoin(fmt.Sprintf("%s ON %s.id = main.id", spec.Name, spec.Name))
		}
	}
	for i := 0; i < len(requiredJoins); i++ {
		join := requiredJoins[i]
		onClause := join.On
		if join.Where != "" {
			onClause = fmt.Sprintf("(%s) AND (%s)", join.On, join.Where)
		}
		inner = inner.LeftJoin(fmt.Sprintf("%s AS %s ON %s", join.Table, join.Alias, onClause))
	}
	if wherePart != nil {
		inner = inner.Where(wherePart)
	}
	groupByCols := []string{"main.id"}
	if aliasMap != nil && len(computableOverride) == 0 {
		hasManyAliases := make(map[string]struct{})
		for path, alias := range aliasMap.PathToAlias {
			if isHasManyPath(m, path) {
				hasManyAliases[alias] = struct{}{}
			}
		}
		compExprs := collectComputableExprsForRequest(m, preset, filters, nil, aliasMap)
		for _, expr := range compExprs {
			for _, col := range extractQualifiedColumns(expr, hasManyAliases) {
				if !containsString(groupByCols, col) {
					groupByCols = append(groupByCols, col)
				}
			}
		}
	}
	inner = inner.Column("main.id").GroupBy(groupByCols...).Having(havingPart)

	sb = squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)
	sb = sb.Columns("COUNT(*)").FromSelect(inner, "sub")
	return sb, nil
}
