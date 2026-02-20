package model

import (
	"fmt"
	"regexp"
	"strings"

	"YrestAPI/internal/logger"

	"github.com/Masterminds/squirrel"
)

var aggregateRe = regexp.MustCompile(`(?i)\b(sum|count|avg|min|max)\s*\(`)

func isAggregateExpr(expr string) bool {
	return aggregateRe.MatchString(expr)
}

func (m *Model) buildWhereClause(
	aliasMap *AliasMap,
	preset *DataPreset,
	filters map[string]any,
	joins []*JoinSpec,
	computableOverride map[string]string,
) (squirrel.Sqlizer, squirrel.Sqlizer, error) {
	var buildGroupExpr func(map[string]any, string) (squirrel.Sqlizer, bool)
	var buildGroupAnd func(map[string]any) (squirrel.Sqlizer, squirrel.Sqlizer)

	resolveField := func(fld string) string {
		if computableOverride != nil {
			if expr, ok := computableOverride[fld]; ok && strings.TrimSpace(expr) != "" {
				return expr
			}
		}
		if expr, ok := m.resolveFieldExpression(preset, aliasMap, fld); ok {
			return expr
		}
		if idx := strings.LastIndex(fld, "."); idx != -1 {
			path := fld[:idx]     // например "contragent.organization"
			column := fld[idx+1:] // например "name"
			alias, ok := aliasMap.PathToAlias[path]
			if !ok {
				logger.Warn("unknown_relation_path", map[string]any{"path": path})
				return ""
			}
			return fmt.Sprintf("%s.%s", alias, column)
		}
		return fmt.Sprintf("main.%s", fld)
	}

	resolveComputableAlias := func(field string) string {
		if preset == nil {
			return ""
		}
		for _, f := range preset.Fields {
			if f.Type != "computable" {
				continue
			}
			if f.Source != field {
				continue
			}
			alias := strings.TrimSpace(f.Alias)
			if alias == "" {
				alias = f.Source
			}
			if alias != "" {
				return quoteIdentifier(alias)
			}
		}
		return ""
	}

	buildCond := func(field, op string, val any) ([]squirrel.Sqlizer, bool) {
		fields, comb := ParseCompositeField(field)
		parts := make([]squirrel.Sqlizer, 0, len(fields))
		hasAgg := false
		baseOp := op
		caseSensitive := false
		if strings.HasSuffix(baseOp, "_cs") {
			baseOp = strings.TrimSuffix(baseOp, "_cs")
			caseSensitive = true
		}

		for _, f := range fields {
			expr := resolveField(f)
			if expr == "" {
				continue
			}
			fieldType := resolveFilterFieldType(m, f)
			sqlField := expr
			agg := isAggregateExpr(expr)
			if agg {
				if alias := resolveComputableAlias(f); alias != "" {
					sqlField = alias
				}
				hasAgg = true
			}
			var cond squirrel.Sqlizer
			switch baseOp {
			case "eq":
				if s, ok := val.(string); ok {
					comparisonField := sqlField
					if needsTextCast(fieldType, "string") {
						comparisonField = fmt.Sprintf("CAST(%s AS TEXT)", sqlField)
					}
					if caseSensitive {
						cond = squirrel.Expr(fmt.Sprintf("%s = ?", comparisonField), s)
					} else {
						cond = squirrel.Expr(fmt.Sprintf("LOWER(%s) = LOWER(?)", comparisonField), s)
					}
				} else if b, ok := val.(bool); ok && !b && agg {
					cond = squirrel.Expr(fmt.Sprintf("(%s = false AND %s IS NOT NULL)", sqlField, sqlField))
				} else {
					cond = squirrel.Eq{sqlField: val}
				}
			case "in":
				cond = squirrel.Eq{sqlField: val} // поддерживает slice
			case "lt":
				cond = squirrel.Lt{sqlField: val}
			case "lte":
				cond = squirrel.LtOrEq{sqlField: val}
			case "gt":
				cond = squirrel.Gt{sqlField: val}
			case "gte":
				cond = squirrel.GtOrEq{sqlField: val}
			case "start":
				if s, ok := val.(string); ok {
					comparisonField := sqlField
					if needsTextCast(fieldType, "string") {
						comparisonField = fmt.Sprintf("CAST(%s AS TEXT)", sqlField)
					}
					if caseSensitive {
						cond = squirrel.Expr(fmt.Sprintf("%s LIKE ?", comparisonField), s+"%")
					} else {
						cond = squirrel.Expr(fmt.Sprintf("%s ILIKE ?", comparisonField), s+"%")
					}
				}
			case "end":
				if s, ok := val.(string); ok {
					comparisonField := sqlField
					if needsTextCast(fieldType, "string") {
						comparisonField = fmt.Sprintf("CAST(%s AS TEXT)", sqlField)
					}
					if caseSensitive {
						cond = squirrel.Expr(fmt.Sprintf("%s LIKE ?", comparisonField), "%"+s)
					} else {
						cond = squirrel.Expr(fmt.Sprintf("%s ILIKE ?", comparisonField), "%"+s)
					}
				}
			case "cnt":
				if s, ok := val.(string); ok {
					comparisonField := sqlField
					if needsTextCast(fieldType, "string") {
						comparisonField = fmt.Sprintf("CAST(%s AS TEXT)", sqlField)
					}
					if caseSensitive {
						cond = squirrel.Expr(fmt.Sprintf("%s LIKE ?", comparisonField), "%"+s+"%")
					} else {
						cond = squirrel.Expr(fmt.Sprintf("%s ILIKE ?", comparisonField), "%"+s+"%")
					}
				}
			case "null":
				if b, ok := val.(bool); ok {
					if b {
						cond = squirrel.Expr(fmt.Sprintf("%s IS NULL", sqlField))
					} else {
						cond = squirrel.Expr(fmt.Sprintf("%s IS NOT NULL", sqlField))
					}
				}
			case "is_null":
				cond = squirrel.Expr(fmt.Sprintf("%s IS NULL", sqlField))
			case "not_null":
				cond = squirrel.Expr(fmt.Sprintf("%s IS NOT NULL", sqlField))
			}

			if cond != nil {
				parts = append(parts, cond)
			} else {
				logger.Warn("unknown_filter_operator", map[string]any{"op": op, "field": field})
			}
		}

		if len(parts) == 0 {
			return nil, false
		}
		if comb == "_or_" && len(parts) > 1 {
			return []squirrel.Sqlizer{squirrel.Or(parts)}, hasAgg
		}
		if comb == "_and_" && len(parts) > 1 {
			return []squirrel.Sqlizer{squirrel.And(parts)}, hasAgg
		}
		return []squirrel.Sqlizer{parts[0]}, hasAgg
	}

	buildGroupExpr = func(f map[string]any, mode string) (squirrel.Sqlizer, bool) {
		if len(f) == 0 {
			return nil, false
		}
		var exprs []squirrel.Sqlizer
		hasAgg := false
		for key, val := range f {
			// группирующие ключи or/and
			if key == "or" || key == "and" {
				if sub, ok := val.(map[string]any); ok {
					if nested, nestedAgg := buildGroupExpr(sub, key); nested != nil {
						exprs = append(exprs, nested)
						if nestedAgg {
							hasAgg = true
						}
					}
					continue
				}
				if arr, ok := val.([]any); ok {
					var parts []squirrel.Sqlizer
					subAgg := false
					for _, item := range arr {
						subMap, ok := item.(map[string]any)
						if !ok || len(subMap) == 0 {
							continue
						}
						// каждый элемент массива — отдельная группа (AND внутри)
						if nested, nestedAgg := buildGroupExpr(subMap, "and"); nested != nil {
							parts = append(parts, nested)
							if nestedAgg {
								subAgg = true
							}
						}
					}
					if len(parts) > 0 {
						if key == "or" {
							exprs = append(exprs, squirrel.Or(parts))
						} else {
							exprs = append(exprs, squirrel.And(parts))
						}
						if subAgg {
							hasAgg = true
						}
					}
					continue
				}
			}

			field := key
			op := "eq"
			if parts := strings.SplitN(key, "__", 2); len(parts) == 2 {
				field = parts[0]
				op = parts[1]
			}

			if conds, condAgg := buildCond(field, op, val); len(conds) > 0 {
				exprs = append(exprs, conds...)
				if condAgg {
					hasAgg = true
				}
			}
		}

		if len(exprs) == 0 {
			return nil, false
		}
		if mode == "or" {
			return squirrel.Or(exprs), hasAgg
		}
		return squirrel.And(exprs), hasAgg
	}

	buildGroupAnd = func(f map[string]any) (squirrel.Sqlizer, squirrel.Sqlizer) {
		if len(f) == 0 {
			return nil, nil
		}
		var whereParts []squirrel.Sqlizer
		var havingParts []squirrel.Sqlizer

		for key, val := range f {
			if key == "or" {
				if sub, ok := val.(map[string]any); ok {
					if expr, agg := buildGroupExpr(sub, "or"); expr != nil {
						if agg {
							havingParts = append(havingParts, expr)
						} else {
							whereParts = append(whereParts, expr)
						}
					}
				}
				if arr, ok := val.([]any); ok {
					var parts []squirrel.Sqlizer
					subAgg := false
					for _, item := range arr {
						subMap, ok := item.(map[string]any)
						if !ok || len(subMap) == 0 {
							continue
						}
						if expr, agg := buildGroupExpr(subMap, "and"); expr != nil {
							parts = append(parts, expr)
							if agg {
								subAgg = true
							}
						}
					}
					if len(parts) > 0 {
						expr := squirrel.Or(parts)
						if subAgg {
							havingParts = append(havingParts, expr)
						} else {
							whereParts = append(whereParts, expr)
						}
					}
				}
				continue
			}
			if key == "and" {
				if sub, ok := val.(map[string]any); ok {
					subWhere, subHaving := buildGroupAnd(sub)
					if subWhere != nil {
						whereParts = append(whereParts, subWhere)
					}
					if subHaving != nil {
						havingParts = append(havingParts, subHaving)
					}
				}
				if arr, ok := val.([]any); ok {
					for _, item := range arr {
						subMap, ok := item.(map[string]any)
						if !ok || len(subMap) == 0 {
							continue
						}
						subWhere, subHaving := buildGroupAnd(subMap)
						if subWhere != nil {
							whereParts = append(whereParts, subWhere)
						}
						if subHaving != nil {
							havingParts = append(havingParts, subHaving)
						}
					}
				}
				continue
			}

			field := key
			op := "eq"
			if parts := strings.SplitN(key, "__", 2); len(parts) == 2 {
				field = parts[0]
				op = parts[1]
			}
			if conds, condAgg := buildCond(field, op, val); len(conds) > 0 {
				if condAgg {
					havingParts = append(havingParts, conds...)
				} else {
					whereParts = append(whereParts, conds...)
				}
			}
		}

		var whereExpr squirrel.Sqlizer
		if len(whereParts) > 0 {
			whereExpr = squirrel.And(whereParts)
		}
		var havingExpr squirrel.Sqlizer
		if len(havingParts) > 0 {
			havingExpr = squirrel.And(havingParts)
		}
		return whereExpr, havingExpr
	}

	where, having := buildGroupAnd(filters)
	if where == nil && having == nil {
		return nil, nil, nil
	}
	return where, having, nil
}

func resolveFilterFieldType(m *Model, fieldPath string) string {
	if m == nil {
		return ""
	}
	if c, ok := m.Computable[fieldPath]; ok && c != nil {
		return normalizeFieldType(c.Type)
	}

	segs := strings.Split(fieldPath, ".")
	target := m
	field := fieldPath
	if len(segs) > 1 {
		field = segs[len(segs)-1]
		for i := 0; i < len(segs)-1; i++ {
			rel := target.Relations[segs[i]]
			if rel == nil || rel.GetModelRef() == nil {
				return ""
			}
			target = rel.GetModelRef()
		}
	}

	if c, ok := target.Computable[field]; ok && c != nil {
		return normalizeFieldType(c.Type)
	}
	for _, p := range target.Presets {
		for _, f := range p.Fields {
			if f.Source != field && strings.TrimSpace(f.Alias) != field {
				continue
			}
			if f.Type == "computable" {
				if c, ok := target.Computable[f.Source]; ok && c != nil {
					return normalizeFieldType(c.Type)
				}
			}
			return normalizeFieldType(f.Type)
		}
	}
	return ""
}

func normalizeFieldType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "uuid" {
		return "uuid"
	}
	return t
}

func needsTextCast(fieldType, valueType string) bool {
	if fieldType == "" || valueType == "" {
		return false
	}
	return fieldType != valueType
}
