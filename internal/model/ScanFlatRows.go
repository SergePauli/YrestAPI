package model

import (
	"fmt"

	"strconv"
	"strings"

	"github.com/google/uuid"
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
	cols, types := m.ScanColumns(preset, aliasMap, "")
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
		
    row := make(map[string]any, len(cols))    
    for i, expr := range keys {
        v := vals[i]				
        switch types[cols[i]] {
        case "UUID":
            switch val := v.(type) {
    					case []byte:
        				if id, err := uuid.FromBytes(val); err == nil {
            		v = id.String()
        			}
    					case [16]byte: // вот этот кейс и срабатывает у pgx
        				if id, err := uuid.FromBytes(val[:]); err == nil {
            		v = id.String()
        			}
    					case uuid.UUID: // если pgx уже вернул готовый тип
        				v = val.String()
    				}
        case "int":
            // pgx чаще отдаёт int64 сразу, но если []byte — конвертируем
            if b, ok := v.([]byte); ok {
                v, _ = strconv.ParseInt(string(b), 10, 64)
            }
        case "float":
            if b, ok := v.([]byte); ok {
                v, _ = strconv.ParseFloat(string(b), 64)
            }
        case "bool":
            if b, ok := v.([]byte); ok {
                v = (string(b) == "t")
            }
        case "string":
            if b, ok := v.([]byte); ok {
                v = string(b)
            }
        }
        row[expr] = v
        i++
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
