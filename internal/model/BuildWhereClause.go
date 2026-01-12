package model

import (
	"fmt"
	"log"
	"strings"

	"github.com/Masterminds/squirrel"
)

func (m *Model) buildWhereClause(
	aliasMap *AliasMap,
	preset *DataPreset,
	filters map[string]any,
	joins []*JoinSpec,
) (squirrel.Sqlizer, error) {
	var buildGroup func(map[string]any, string) squirrel.Sqlizer

	resolveField := func(fld string) string {
		if expr, ok := m.resolveFieldExpression(preset, aliasMap, fld); ok {
			return expr
		}
		if idx := strings.LastIndex(fld, "."); idx != -1 {
			path := fld[:idx]     // например "contragent.organization"
			column := fld[idx+1:] // например "name"
			alias, ok := aliasMap.PathToAlias[path]
			if !ok {
				log.Printf("⚠️ Unknown relation path in filter: %s", path)
				return ""
			}
			return fmt.Sprintf("%s.%s", alias, column)
		}
		return fmt.Sprintf("main.%s", fld)
	}

	buildCond := func(field, op string, val any) []squirrel.Sqlizer {
		fields, comb := ParseCompositeField(field)
		parts := make([]squirrel.Sqlizer, 0, len(fields))

		for _, f := range fields {
			sqlField := resolveField(f)
			if sqlField == "" {
				continue
			}
			var cond squirrel.Sqlizer
			switch op {
			case "eq":
				cond = squirrel.Eq{sqlField: val}
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
					cond = squirrel.Like{sqlField: s + "%"}
				}
			case "end":
				if s, ok := val.(string); ok {
					cond = squirrel.Like{sqlField: "%" + s}
				}
			case "cnt":
				if s, ok := val.(string); ok {
					cond = squirrel.Like{sqlField: "%" + s + "%"}
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
				log.Printf("⚠️ Unknown filter operator: %s in key: %s", op, field)
			}
		}

		if len(parts) == 0 {
			return nil
		}
		if comb == "_or_" && len(parts) > 1 {
			return []squirrel.Sqlizer{squirrel.Or(parts)}
		}
		if comb == "_and_" && len(parts) > 1 {
			return []squirrel.Sqlizer{squirrel.And(parts)}
		}
		return []squirrel.Sqlizer{parts[0]}
	}

	buildGroup = func(f map[string]any, mode string) squirrel.Sqlizer {
		if len(f) == 0 {
			return nil
		}
		var exprs []squirrel.Sqlizer
		for key, val := range f {
			// группирующие ключи or/and
			if key == "or" || key == "and" {
				if sub, ok := val.(map[string]any); ok {
					if nested := buildGroup(sub, key); nested != nil {
						exprs = append(exprs, nested)
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

			if conds := buildCond(field, op, val); len(conds) > 0 {
				exprs = append(exprs, conds...)
			}
		}

		if len(exprs) == 0 {
			return nil
		}
		if mode == "or" {
			return squirrel.Or(exprs)
		}
		return squirrel.And(exprs)
	}

	where := buildGroup(filters, "and")
	if where == nil {
		return nil, nil
	}
	return where, nil
}
