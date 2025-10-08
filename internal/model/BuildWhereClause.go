package model

import (
	"fmt"
	"log"
	"strings"

	"github.com/Masterminds/squirrel"
)
func (m *Model) buildWhereClause(
	aliasMap *AliasMap,
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

		
		// Используем aliasMap.PathToAlias для определения алиаса таблицы
		var sqlField string
		if idx := strings.LastIndex(field, "."); idx != -1 {
			path := field[:idx]      // например "contragent.organization"
			column := field[idx+1:] // например "name"
			alias, ok := aliasMap.PathToAlias[path]
			if !ok {
				log.Printf("⚠️ Unknown relation path in filter: %s", path)
				continue
			}
			sqlField = fmt.Sprintf("%s.%s", alias, column)
		} else {
			// поле без ".", значит поле из основной модели
			sqlField = fmt.Sprintf("main.%s", field)
		}

		// Построим условие
		switch op {
		case "eq":
			exprs = append(exprs, squirrel.Eq{sqlField: val})
		case "in":
			exprs = append(exprs, squirrel.Eq{sqlField: val}) // поддерживает slice
		case "lt":
			exprs = append(exprs, squirrel.Lt{sqlField: val})
		case "lte":
			exprs = append(exprs, squirrel.LtOrEq{sqlField: val})
		case "gt":
			exprs = append(exprs, squirrel.Gt{sqlField: val})
		case "gte":
			exprs = append(exprs, squirrel.GtOrEq{sqlField: val})
		case "start":
			if s, ok := val.(string); ok {
				exprs = append(exprs, squirrel.Like{sqlField: s + "%"})
			}
		case "end":
			if s, ok := val.(string); ok {
				exprs = append(exprs, squirrel.Like{sqlField: "%" + s})
			}
		case "cnt":
			if s, ok := val.(string); ok {
				exprs = append(exprs, squirrel.Like{sqlField: "%" + s + "%"})
			}
		default:
			log.Printf("⚠️ Unknown filter operator: %s in key: %s", op, key)
		}
	}

	

	if len(exprs) == 0 {
		return nil, nil
	}

	return squirrel.And(exprs), nil
}