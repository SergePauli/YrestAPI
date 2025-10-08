// file: scan_flat_rows_preset_skip_test.go
package model

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// --- фейковая реализация pgx.Rows ---

type stubRows struct {
	data [][]any
	pos  int
	err  error
}

var _ pgx.Rows = (*stubRows)(nil)

func (r *stubRows) Close()                                  {}
func (r *stubRows) Err() error                              { return r.err }
func (r *stubRows) CommandTag() pgconn.CommandTag           { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
// ВАЖНО для pgx/v5: добавить Conn() *pgx.Conn
func (r *stubRows) Conn() *pgx.Conn { return nil }
func (r *stubRows) Next() bool {
	if r.pos >= len(r.data) {
		return false
	}
	r.pos++
	return true
}
func (r *stubRows) Scan(dest ...any) error { // не используется в тесте
	row := r.data[r.pos-1]
	for i := range dest {
		if i >= len(row) {
			break
		}
		if p, ok := dest[i].(*any); ok {
			*p = row[i]
		}
	}
	return nil
}
func (r *stubRows) Values() ([]any, error) {
	if r.pos == 0 || r.pos-1 >= len(r.data) {
		return nil, nil
	}
	return r.data[r.pos-1], nil
}
func (r *stubRows) RawValues() [][]byte { return nil }

// --- сам тест ---

func TestScanFlatRows_SkipsPresetPlaceholders(t *testing.T) {
	// Модели: root -> belongs_to address -> belongs_to area
	area := &Model{Table: "areas", Relations: map[string]*ModelRelation{}}

	address := &Model{
		Table: "addresses",
		Relations: map[string]*ModelRelation{
			"area": {Type: "belongs_to", FK: "area_id", PK: "id", _ModelRef: area},
		},
		Presets: map[string]*DataPreset{
			"item": {
				Name: "item",
				Fields: []Field{
					// Листовые поля адреса
					{Type: "int", Source: "id"},
					{Type: "string", Source: "name"},
					// Вложенный preset: area (это создаст виртуальную "preset"-колонку)
					{Type: "preset", Source: "area", NestedPreset: "item", _PresetRef: &DataPreset{
						Name: "item",
						Fields: []Field{
							{Type: "int", Source: "id"},
							{Type: "string", Source: "name"},
						},
					}},
				},
			},
		},
	}

	root := &Model{
		Table: "contragent_addresses",
		Relations: map[string]*ModelRelation{
			"address": {Type: "belongs_to", FK: "address_id", PK: "id", _ModelRef: address},
		},
		
	}
	aliasMap:= &AliasMap{
			// важны оба направления: путь → алиас и обратно
			PathToAlias: map[string]string{
				"address":      "t0",
				"address.area": "t1",
			},
			AliasToPath: map[string]string{
				"t0": "address",
				"t1": "address.area",
			},
		};
	// Пресет корня: один preset "address" (виртуальная колонка) + его вложенные листовые поля
	p := &DataPreset{
		Name: "card",
		Fields: []Field{
			{Type: "preset", Source: "address", NestedPreset: "item", _PresetRef: address.Presets["item"]},
		},
	}

	// Фейковые данные одной строки. ВАЖНО:
	// здесь только реальные значения (для листовых полей),
	// без "preset"-плейсхолдеров — ScanFlatRows обязан правильно сопоставить индексы.
	vals := []any{
		int64(105),                 // address.id
		"ул. Кирова, 81",           // address.name
		int64(24),                  // address.area.id
		"Красноярский край",        // address.area.name
	}

	rows := &stubRows{data: [][]any{vals}}

	gotFlat, err := root.ScanFlatRows(rows, p, aliasMap)
	
	if err != nil {
		t.Fatalf("ScanFlatRows error: %v", err)
	}
	if len(gotFlat) != 1 {
		t.Fatalf("expected 1 row, got %d", len(gotFlat))
	}
	row := gotFlat[0]	
	// Ожидаемые ключи (их строит ScanFlatRows по AliasMap)
	want := map[string]any{
		"address": map[string]any{  
			"area": map[string]any{"id" : int64(24), "name" : "Красноярский край",},
			"id":   int64(105),
			"name": "ул. Кирова, 81",
		},	
	}

	// Проверим, что:
	// 1) все листовые ключи присутствуют с верными значениями,
	// 2) не появилось лишних значений (например, записей по виртуальным "preset"-колонкам)
	for k, v := range want {
		if !reflect.DeepEqual(row[k], v) {
			t.Fatalf("key %q mismatch: got %#v, want %#v", k, row[k], v)
		}
	}
	// Убедимся, что общее число пар ключ-значение равно количеству листовых полей
	if len(row) != len(want) {
		t.Fatalf("unexpected keys in row: got %d keys, want %d. row=%#v", len(row), len(want), row)
	}
}
