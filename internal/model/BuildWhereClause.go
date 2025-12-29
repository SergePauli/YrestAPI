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
	var exprs []squirrel.Sqlizer

	// 1. Соберем WHERE из фильтров
	for key, val := range filters {
		field := key
		op := "eq"

		// Разделим ключ фильтра на имя поля и оператор
		if parts := strings.SplitN(key, "__", 2); len(parts) == 2 {
			field = parts[0]
			op = parts[1]
		}

		fields, comb := ParseCompositeField(field)
		parts := make([]squirrel.Sqlizer, 0, len(fields))

		buildCond := func(sqlField string) squirrel.Sqlizer {
			switch op {
			case "eq":
				return squirrel.Eq{sqlField: val}
			case "in":
				return squirrel.Eq{sqlField: val} // поддерживает slice
			case "lt":
				return squirrel.Lt{sqlField: val}
			case "lte":
				return squirrel.LtOrEq{sqlField: val}
			case "gt":
				return squirrel.Gt{sqlField: val}
			case "gte":
				return squirrel.GtOrEq{sqlField: val}
			case "start":
				if s, ok := val.(string); ok {
					return squirrel.Like{sqlField: s + "%"}
				}
			case "end":
				if s, ok := val.(string); ok {
					return squirrel.Like{sqlField: "%" + s}
				}
			case "cnt":
				if s, ok := val.(string); ok {
					return squirrel.Like{sqlField: "%" + s + "%"}
				}
			}
			log.Printf("⚠️ Unknown filter operator: %s in key: %s", op, key)
			return nil
		}

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

		for _, f := range fields {
			sqlField := resolveField(f)
			if sqlField == "" {
				continue
			}
			if cond := buildCond(sqlField); cond != nil {
				parts = append(parts, cond)
			}
		}

		if len(parts) == 0 {
			continue
		}
		if comb == "_or_" && len(parts) > 1 {
			exprs = append(exprs, squirrel.Or(parts))
		} else if comb == "_and_" && len(parts) > 1 {
			exprs = append(exprs, squirrel.And(parts))
		} else {
			exprs = append(exprs, parts[0])
		}
	}

	if len(exprs) == 0 {
		return nil, nil
	}

	return squirrel.And(exprs), nil
}
