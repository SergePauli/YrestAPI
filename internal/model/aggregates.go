package model

import (
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
)

type AggregateSpec struct {
	Name  string
	Fn    string
	Field string
}

type ResolvedAggregateSpec struct {
	Name  string
	Fn    string
	Field string
	Type  string
	Expr  string
	Alias string
}

func normalizeAggregateFn(fn string) string {
	return strings.ToLower(strings.TrimSpace(fn))
}

func quoteLiteralAlias(name string) string {
	return quoteIdentifier(name)
}

func (m *Model) ValidateAndResolveAggregates(aliasMap *AliasMap, aggregates []AggregateSpec) ([]ResolvedAggregateSpec, error) {
	if len(aggregates) == 0 {
		return nil, nil
	}
	out := make([]ResolvedAggregateSpec, 0, len(aggregates))
	seenNames := make(map[string]struct{}, len(aggregates))

	for i, spec := range aggregates {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			return nil, fmt.Errorf("aggregate name is required")
		}
		if _, ok := seenNames[name]; ok {
			return nil, fmt.Errorf("duplicate aggregate name %q", name)
		}
		seenNames[name] = struct{}{}

		field := strings.TrimSpace(ExpandAliasPath(m, spec.Field))
		if field == "" {
			return nil, fmt.Errorf("aggregate %q: field is required", name)
		}
		fn := normalizeAggregateFn(spec.Fn)
		switch fn {
		case "sum", "avg", "min", "max":
		default:
			return nil, fmt.Errorf("aggregate %q: unsupported function %q", name, spec.Fn)
		}

		cfg, ok := m.Aggregatable[field]
		if !ok || cfg == nil {
			return nil, fmt.Errorf("aggregate %q: field %q is not aggregatable", name, field)
		}
		if err := m.validateAggregateFieldPath(field); err != nil {
			return nil, fmt.Errorf("aggregate %q: %w", name, err)
		}
		if !aggregateFunctionAllowed(cfg, fn) {
			return nil, fmt.Errorf("aggregate %q: function %q is not allowed for field %q", name, fn, field)
		}

		expr, ok := m.resolveFieldExpression(nil, aliasMap, field)
		if !ok || strings.TrimSpace(expr) == "" {
			return nil, fmt.Errorf("aggregate %q: could not resolve field %q", name, field)
		}

		out = append(out, ResolvedAggregateSpec{
			Name:  name,
			Fn:    fn,
			Field: field,
			Type:  strings.TrimSpace(cfg.Type),
			Expr:  expr,
			Alias: fmt.Sprintf("__agg_%d", i),
		})
	}

	return out, nil
}

func aggregateFunctionAllowed(cfg *Aggregatable, fn string) bool {
	if cfg == nil {
		return false
	}
	for _, allowed := range cfg.Functions {
		if normalizeAggregateFn(allowed) == fn {
			return true
		}
	}
	return false
}

func (m *Model) validateAggregateFieldPath(field string) error {
	if m == nil {
		return fmt.Errorf("nil model")
	}
	segs := strings.Split(field, ".")
	if len(segs) == 1 {
		return nil
	}

	curr := m
	for i := 0; i < len(segs)-1; i++ {
		rel := curr.Relations[segs[i]]
		if rel == nil || rel.GetModelRef() == nil {
			return fmt.Errorf("relation %q not found", strings.Join(segs[:i+1], "."))
		}
		if rel.Type == "has_many" {
			return fmt.Errorf("field %q traverses has_many relation %q", field, strings.Join(segs[:i+1], "."))
		}
		curr = rel.GetModelRef()
	}
	return nil
}

func collectAggregatePaths(m *Model, aggregates []AggregateSpec) []string {
	if m == nil || len(aggregates) == 0 {
		return nil
	}

	set := make(map[string]struct{})
	addPath := func(path string) {
		path = strings.TrimSpace(path)
		if path != "" {
			set[path] = struct{}{}
		}
	}

	for _, spec := range aggregates {
		field := strings.TrimSpace(ExpandAliasPath(m, spec.Field))
		if field == "" {
			continue
		}

		if idx := strings.LastIndex(field, "."); idx >= 0 {
			addPath(field[:idx])
		}

		curr := m
		basePath := ""
		fieldName := field
		if idx := strings.LastIndex(field, "."); idx >= 0 {
			basePath = field[:idx]
			fieldName = field[idx+1:]
			for _, seg := range strings.Split(basePath, ".") {
				rel := curr.Relations[seg]
				if rel == nil || rel.GetModelRef() == nil {
					curr = nil
					break
				}
				curr = rel.GetModelRef()
			}
		}
		if curr == nil {
			continue
		}
		if comp := curr.Computable[fieldName]; comp != nil {
			for _, p := range extractPathsFromExpr(comp.Source) {
				if basePath != "" {
					addPath(basePath + "." + p)
				} else {
					addPath(p)
				}
			}
			for _, p := range extractPathsFromExpr(comp.Where) {
				if basePath != "" {
					addPath(basePath + "." + p)
				} else {
					addPath(p)
				}
			}
		}
	}

	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	return out
}

func (m *Model) CreateAliasMapForAggregates(preset *DataPreset, filters map[string]interface{}, aggregates []AggregateSpec) (*AliasMap, error) {
	aliasMap, err := m.CreateAliasMap(m, preset, filters, nil)
	if err != nil {
		return nil, err
	}
	nextIdx := detectNextAliasIndex(aliasMap)
	for _, path := range mergeAndSortPaths(collectAggregatePaths(m, aggregates)) {
		if err := ensureAliasPath(m, aliasMap, path, &nextIdx); err != nil {
			return nil, err
		}
	}
	return aliasMap, nil
}

func (m *Model) BuildCountAggregateQuery(aliasMap *AliasMap, preset *DataPreset, filters map[string]interface{}, aggregates []ResolvedAggregateSpec) (squirrel.SelectBuilder, error) {
	filters = NormalizeFiltersWithAliases(m, filters)

	base := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)
	base = base.From(fmt.Sprintf("%s AS main", m.Table))

	filterKeys := PathsFromFilters(filters)
	aggregatePaths := make([]string, 0, len(aggregates))
	for _, agg := range aggregates {
		if idx := strings.LastIndex(agg.Field, "."); idx >= 0 {
			aggregatePaths = append(aggregatePaths, agg.Field[:idx])
		}
	}

	compPaths := collectComputablePathsForRequest(m, preset, filters, nil)
	compPaths = append(compPaths, collectAggregatePaths(m, toAggregateSpecs(aggregates))...)
	requiredJoins, err := m.DetectJoins(aliasMap, mergeAndSortPaths(filterKeys, aggregatePaths), nil, compPaths)
	if err != nil {
		return base, err
	}

	cteSpecs, computableOverride, skipAliases := buildHasManyCTEs(m, preset, filters, nil, aliasMap, requiredJoins)
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
	requiredJoins = filterJoinSpecs(requiredJoins, skipAliases)
	for _, join := range requiredJoins {
		onClause := join.On
		if join.Where != "" {
			onClause = fmt.Sprintf("(%s) AND (%s)", join.On, join.Where)
		}
		base = base.LeftJoin(fmt.Sprintf("%s AS %s ON %s", join.Table, join.Alias, onClause))
	}

	wherePart, havingPart, err := m.buildWhereClause(aliasMap, preset, filters, requiredJoins, computableOverride)
	if err != nil {
		return base, err
	}
	if wherePart != nil {
		base = base.Where(wherePart)
	}

	inner := base.Column("main.id")
	groupByCols := []string{"main.id"}
	for _, agg := range aggregates {
		inner = inner.Column(fmt.Sprintf("%s AS %s", agg.Expr, quoteLiteralAlias(agg.Alias)))
		if !isAggregateExpr(agg.Expr) {
			groupByCols = append(groupByCols, agg.Expr)
		}
	}

	if havingPart == nil {
		inner = inner.Distinct()
	} else {
		for _, expr := range collectComputableExprsForRequest(m, preset, filters, nil, aliasMap) {
			if !isAggregateExpr(expr) && !containsString(groupByCols, expr) {
				groupByCols = append(groupByCols, expr)
			}
		}
		inner = inner.GroupBy(groupByCols...).Having(havingPart)
	}

	outer := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)
	outer = outer.Column("COUNT(*) AS count").FromSelect(inner, "sub")
	for _, agg := range aggregates {
		outer = outer.Column(fmt.Sprintf("%s(sub.%s) AS %s", strings.ToUpper(agg.Fn), quoteLiteralAlias(agg.Alias), quoteLiteralAlias(agg.Alias)))
	}
	return outer, nil
}

func toAggregateSpecs(in []ResolvedAggregateSpec) []AggregateSpec {
	out := make([]AggregateSpec, 0, len(in))
	for _, agg := range in {
		out = append(out, AggregateSpec{
			Name:  agg.Name,
			Fn:    agg.Fn,
			Field: agg.Field,
		})
	}
	return out
}
