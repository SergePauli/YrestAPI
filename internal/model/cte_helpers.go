package model

import (
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
)

type cteSpec struct {
	Name   string
	Join   *JoinSpec
	Fields map[string]string // computable source -> expr
}

func buildHasManyCTEs(m *Model, preset *DataPreset, filters map[string]any, sorts []string, aliasMap *AliasMap, joinSpecs []*JoinSpec) ([]cteSpec, map[string]string, map[string]struct{}) {
	if m == nil || aliasMap == nil {
		return nil, nil, nil
	}

	joinByAlias := make(map[string]*JoinSpec, len(joinSpecs))
	for _, j := range joinSpecs {
		if j != nil && j.Alias != "" {
			joinByAlias[j.Alias] = j
		}
	}

	used := make(map[string]struct{})
	if preset != nil {
		for _, f := range preset.Fields {
			if f.Type == "computable" {
				used[f.Source] = struct{}{}
			}
		}
	}
	for key := range filters {
		base := key
		if i := strings.Index(key, "__"); i >= 0 {
			base = key[:i]
		}
		if _, ok := m.Computable[base]; ok {
			used[base] = struct{}{}
		}
	}
	for _, s := range sorts {
		parts := strings.Fields(s)
		if len(parts) == 0 {
			continue
		}
		field := parts[0]
		if _, ok := m.Computable[field]; ok {
			used[field] = struct{}{}
		}
	}

	byAlias := map[string]*cteSpec{}
	override := map[string]string{}
	skipAliases := map[string]struct{}{}

	for name := range used {
		comp := m.Computable[name]
		if comp == nil || strings.TrimSpace(comp.Source) == "" {
			continue
		}
		paths := extractPathsFromExpr(comp.Source)
		var hasManyPath string
		for _, p := range paths {
			if isHasManyPath(m, p) {
				if hasManyPath != "" && hasManyPath != p {
					hasManyPath = ""
					break
				}
				hasManyPath = p
			}
		}
		if hasManyPath == "" {
			continue
		}
		alias := aliasMap.PathToAlias[hasManyPath]
		if alias == "" {
			continue
		}
		joinSpec := joinByAlias[alias]
		if joinSpec == nil {
			continue
		}

		spec := byAlias[alias]
		if spec == nil {
			spec = &cteSpec{
				Name:   alias + "_agg",
				Join:   joinSpec,
				Fields: map[string]string{},
			}
			byAlias[alias] = spec
		}
		expr := applyAliasPlaceholders(comp.Source, aliasMap, "")
		spec.Fields[name] = expr
		override[name] = fmt.Sprintf("%s.%s", spec.Name, quoteIdentifier(name))
		skipAliases[alias] = struct{}{}
	}

	if len(byAlias) == 0 {
		return nil, nil, nil
	}

	out := make([]cteSpec, 0, len(byAlias))
	for _, spec := range byAlias {
		out = append(out, *spec)
	}
	return out, override, skipAliases
}

func buildCTEQueries(m *Model, specs []cteSpec) (string, []any, error) {
	if m == nil || len(specs) == 0 {
		return "", nil, nil
	}
	parts := make([]string, 0, len(specs))
	var args []any

	for _, spec := range specs {
		sb := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Question)
		sb = sb.From(fmt.Sprintf("%s AS main", m.Table))

		if spec.Join != nil {
			onClause := spec.Join.On
			if spec.Join.Where != "" {
				onClause = fmt.Sprintf("(%s) AND (%s)", spec.Join.On, spec.Join.Where)
			}
			sb = sb.LeftJoin(fmt.Sprintf("%s AS %s ON %s", spec.Join.Table, spec.Join.Alias, onClause))
		}

		sb = sb.Column("main.id")
		for name, expr := range spec.Fields {
			sb = sb.Column(fmt.Sprintf("%s AS %s", expr, quoteIdentifier(name)))
		}
		sb = sb.GroupBy("main.id")

		sql, a, err := sb.ToSql()
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, fmt.Sprintf("%s AS (%s)", spec.Name, sql))
		args = append(args, a...)
	}

	return "WITH " + strings.Join(parts, ", "), args, nil
}

func filterJoinSpecs(joinSpecs []*JoinSpec, skipAliases map[string]struct{}) []*JoinSpec {
	if len(skipAliases) == 0 {
		return joinSpecs
	}
	out := make([]*JoinSpec, 0, len(joinSpecs))
	for _, j := range joinSpecs {
		if j == nil {
			continue
		}
		if _, skip := skipAliases[j.Alias]; skip {
			continue
		}
		out = append(out, j)
	}
	return out
}
