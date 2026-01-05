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
func (m *Model) ScanFlatRows(rows pgx.Rows, preset *DataPreset, aliasMap *AliasMap) ([]map[string]any, error) {
	if rows == nil {
		return nil, fmt.Errorf("rows is nil")
	}
	if aliasMap == nil {
		return nil, fmt.Errorf("alias map is nil (AttachAliasMap must be called)")
	}

	// 1) Восстанавливаем список выражений колонок так же, как их формировал BuildIndexQuery
	cols := m.ScanColumns(preset, aliasMap, "")
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns resolved for scan (preset=%v)", preset != nil)
	}

	// 2) Читаем строки
	out := make([]map[string]any, 0, 64)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}

		row := make(map[string]any, len(cols))
		if len(vals) < len(cols) {
			return nil, fmt.Errorf("expected %d columns, got %d", len(cols), len(vals))
		}
		for i, col := range cols {
			v := vals[i]
			switch col.Type {
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
			if strings.TrimSpace(col.Key) == "" {
				continue
			}
			row[col.Key] = v
		}
		row = FoldFlatRowByPreset(row) // сворачиваем вложенные объекты
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FoldFlatRowByPreset сворачивает плоский row вида
// {"address.id": 1, "address.value": "...", "address.area.id": 660000}
// в {"address": {"id":1, "value":"...", "area":{"id":660000}}}
// Рекурсирует, пока в ключах не останется '.'.
func FoldFlatRowByPreset(flat map[string]any) map[string]any {
	res := make(map[string]any, len(flat))

	// 1) сначала переносим все "листовые" ключи без точки
	for k, v := range flat {
		if strings.IndexByte(k, '.') < 0 {
			res[k] = v
		}
	}

	// 2) группируем ключи с точкой по первому сегменту
	buckets := make(map[string]map[string]any) // head -> subFlat (tail->value)
	for k, v := range flat {
		if i := strings.IndexByte(k, '.'); i >= 0 {
			head := k[:i]
			tail := k[i+1:]
			if tail == "" {
				// защита от "address." — считаем это листом под именем head
				res[head] = v
				continue
			}
			if _, ok := buckets[head]; !ok {
				buckets[head] = make(map[string]any)
			}
			buckets[head][tail] = v
		}
	}

	// 3) рекурсивно сворачиваем каждую группу
	for head, subFlat := range buckets {
		sub := FoldFlatRowByPreset(subFlat)

		// Конфликт: если в res уже есть leaf "head" (не map), отдаём приоритет объекту.
		// При желании можно сохранить leaf в sub["_"].
		if exist, ok := res[head]; ok {
			if _, isMap := exist.(map[string]any); !isMap {
				// приоритет вложенному объекту
			}
		}
		res[head] = sub
	}

	return res
}

// Опционально: свёртка для слайса строк
func FoldFlatRowsByPreset(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = FoldFlatRowByPreset(r)
	}
	return out
}
