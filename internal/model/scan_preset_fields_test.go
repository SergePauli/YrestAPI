package model

import (
	"reflect"
	"sort"
	"testing"
)

// вспомогалка — сортируем для детерминизма сравнения
func sorted(ss []string) []string {
	cp := append([]string(nil), ss...)
	sort.Strings(cp)
	return cp
}

func TestScanPresetFields_BelongsToNestedPaths(t *testing.T) {
	// Модели: Root -> belongs_to Address -> belongs_to Area
	area := &Model{Table: "areas", Relations: map[string]*ModelRelation{}}
	address := &Model{
		Table: "addresses",
		Relations: map[string]*ModelRelation{
			"area": {Type: "belongs_to", FK: "area_id", PK: "id", _ModelRef: area},
		},
		Presets: map[string]*DataPreset{
			"item": {
				Fields: []Field{
					{Type: "preset", Source: "area", NestedPreset: "item", _PresetRef:  &DataPreset{
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

	// пресет корня содержит вложенный preset "address" → "address.item"
	p := &DataPreset{
		Fields: []Field{
			{Type: "preset", Source: "address", NestedPreset: "item", _PresetRef: address.Presets["item"]},
		},
	}

	got := root.ScanPresetFields(p, "")
	// ожидание: пути "address" и "address.area"
	want := []string{"address.", "address.area."}

	if !reflect.DeepEqual(sorted(got), sorted(want)) {
		t.Fatalf("ScanPresetFields:\n got: %v\nwant: %v", got, want)
	}
}
