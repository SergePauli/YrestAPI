package model

import (
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
)

// DistinctValidationError denotes an invalid client-provided unique_by field.
type DistinctValidationError struct {
	Message string
}

func (e *DistinctValidationError) Error() string { return e.Message }

// ResolveDistinctField validates a field path and resolves it to a SQL expression.
// A has_many path is deliberately rejected: one root row can have several values.
func (m *Model) ResolveDistinctField(aliasMap *AliasMap, field string) (string, string, error) {
	field = strings.TrimSpace(ExpandAliasPath(m, field))
	if field == "" {
		return "", "", &DistinctValidationError{Message: "unique_by field is required"}
	}

	segs := strings.Split(field, ".")
	curr := m
	for i, seg := range segs {
		if !identRe.MatchString(seg) || isSQLKeyword(seg) {
			return "", "", &DistinctValidationError{Message: fmt.Sprintf("invalid unique_by field %q", field)}
		}
		if i == len(segs)-1 {
			break
		}
		rel := curr.Relations[seg]
		if rel == nil || rel.GetModelRef() == nil || rel.Polymorphic {
			return "", "", &DistinctValidationError{Message: fmt.Sprintf("relation %q not found for unique_by", strings.Join(segs[:i+1], "."))}
		}
		if rel.Type == "has_many" {
			return "", "", &DistinctValidationError{Message: fmt.Sprintf("unique_by field %q traverses has_many relation %q", field, strings.Join(segs[:i+1], "."))}
		}
		curr = rel.GetModelRef()
	}

	expr, ok := m.resolveFieldExpression(nil, aliasMap, field)
	if !ok || strings.TrimSpace(expr) == "" || isAggregateExpr(expr) {
		return "", "", &DistinctValidationError{Message: fmt.Sprintf("could not resolve unique_by field %q", field)}
	}
	return field, expr, nil
}

// BuildDistinctValuesQuery builds a scalar list query. Presets are intentionally
// absent: only the requested value is selected, while filters keep their normal
// join/has_many/HAVING semantics.
func (m *Model) BuildDistinctValuesQuery(aliasMap *AliasMap, filters map[string]interface{}, field string, offset, limit uint64) (squirrel.SelectBuilder, error) {
	filters = NormalizeFiltersWithAliases(m, filters)
	field, expr, err := m.ResolveDistinctField(aliasMap, field)
	if err != nil {
		return squirrel.SelectBuilder{}, err
	}

	base := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar).From(fmt.Sprintf("%s AS main", m.Table))
	filterKeys := PathsFromFilters(filters)
	requestSorts := []string{field + " ASC"}
	compPaths := collectComputablePathsForRequest(m, nil, filters, requestSorts)
	joins, err := m.DetectJoins(aliasMap, filterKeys, []string{field}, compPaths)
	if err != nil {
		return base, err
	}

	cteSpecs, computableOverride, skipAliases := buildHasManyCTEs(m, nil, filters, requestSorts, aliasMap, joins)
	if len(cteSpecs) > 0 {
		prefixSQL, prefixArgs, err := buildCTEQueries(m, cteSpecs)
		if err != nil {
			return base, err
		}
		base = base.Prefix(prefixSQL, prefixArgs...)
		for _, spec := range cteSpecs {
			base = base.LeftJoin(fmt.Sprintf("%s ON %s.id = main.id", spec.Name, spec.Name))
		}
	}
	joins = filterJoinSpecs(joins, skipAliases)
	for _, join := range joins {
		onClause := join.On
		if join.Where != "" {
			onClause = fmt.Sprintf("(%s) AND (%s)", join.On, join.Where)
		}
		base = base.LeftJoin(fmt.Sprintf("%s AS %s ON %s", join.Table, join.Alias, onClause))
	}

	wherePart, havingPart, err := m.buildWhereClause(aliasMap, nil, filters, joins, computableOverride)
	if err != nil {
		return base, err
	}
	if wherePart != nil {
		base = base.Where(wherePart)
	}

	var query squirrel.SelectBuilder
	if havingPart == nil {
		query = base.Column(fmt.Sprintf("DISTINCT %s AS value", expr))
	} else {
		groupBy := make([]string, 0, len(m.GetPrimaryKeys())+1)
		for _, key := range m.GetPrimaryKeys() {
			groupBy = append(groupBy, "main."+key)
		}
		groupBy = append(groupBy, expr)
		inner := base.Column(fmt.Sprintf("%s AS value", expr)).GroupBy(groupBy...).Having(havingPart)
		query = squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar).
			Column("DISTINCT distinct_roots.value AS value").
			FromSelect(inner, "distinct_roots")
	}

	query = query.OrderBy("value ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	return query, nil
}

func (m *Model) BuildDistinctCountQuery(aliasMap *AliasMap, filters map[string]interface{}, field string) (squirrel.SelectBuilder, error) {
	values, err := m.BuildDistinctValuesQuery(aliasMap, filters, field, 0, 0)
	if err != nil {
		return squirrel.SelectBuilder{}, err
	}
	return squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar).
		Column("COUNT(*)").FromSelect(values, "distinct_values"), nil
}
