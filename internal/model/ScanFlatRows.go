package model

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ScanFlatRows преобразует плоский результат SQL (alias.column без AS) в []map[string]any.
// Ключи строятся как "<path>.<column>" или просто "column" для корня.
// Путь берём из m._AliasMap.AliasToPath (для "main" путь пустой).
func (m *Model) ScanFlatRows(rows pgx.Rows, preset *DataPreset) ([]map[string]any, error) {
	if rows == nil {
		return nil, fmt.Errorf("rows is nil")
	}
	aliasMap := m.GetAliasMap()
	if aliasMap == nil {
		return nil, fmt.Errorf("alias map is nil (AttachAliasMap must be called)")
	}

	// 1) Восстанавливаем список выражений колонок так же, как их формировал BuildIndexQuery
	cols := m.ScanColumns(preset, aliasMap, "")
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns resolved for scan (preset=%v)", preset != nil)
	}

	// 2) Предрассчитать ключи для результата по каждой колонке
	//    пример: "t3.name" -> alias "t3" -> path "person_name.naming" -> key "person_name.naming.name"
	keys := make([]string, len(cols))
	for i, expr := range cols {
		alias, col := splitAliasCol(expr) // expr вида "alias.column"
		var path string
		if alias == "main" || alias == "" {
			path = ""
		} else {
			path = aliasMap.AliasToPath[alias]
		}
		if path == "" {
			keys[i] = col
		} else {
			keys[i] = path + "." + col
		}
	}

	// 3) Читаем строки
	out := make([]map[string]any, 0, 64)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		// На всякий случай — берём минимум от фактических и ожидаемых колонок
		n := len(vals)
		if len(keys) < n {
			n = len(keys)
		}
		row := make(map[string]any, n)
		for i := 0; i < n; i++ {
			row[keys[i]] = vals[i]
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// splitAliasCol разбивает "alias.column" -> ("alias","column").
// Если точки нет — считаем это колонкой корня ("main").
func splitAliasCol(expr string) (alias, col string) {
	idx := strings.IndexByte(expr, '.')
	if idx <= 0 {
		return "main", expr
	}
	return expr[:idx], expr[idx+1:]
}
