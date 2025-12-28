package model

import (
	"testing"
)

// Ensures polymorphic belongs_to does not require model linking and still adds FK/type columns.
func TestPolymorphicBelongsToScanColumns(t *testing.T) {
	origRegistry := Registry
	defer func() { Registry = origRegistry }()

	m := &Model{
		Name:  "Audit",
		Table: "audits",
		Relations: map[string]*ModelRelation{
			"auditable": {Type: "belongs_to", Model: "*", Polymorphic: true},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
					{Source: "auditable", Type: "preset", NestedPreset: "base"},
				},
			},
		},
	}
	Registry = map[string]*Model{"Audit": m}

	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}
	if err := BuildPresetAliasMaps(); err != nil {
		t.Fatalf("BuildPresetAliasMaps: %v", err)
	}

	am, err := m.CreateAliasMap(m, m.Presets["list"], nil, nil)
	if err != nil {
		t.Fatalf("CreateAliasMap: %v", err)
	}

	cols := m.ScanColumns(m.Presets["list"], am, "")
	want := map[string]string{
		"main.id":             "int",
		"main.auditable_id":   "int",
		"main.auditable_type": "string",
	}
	if len(cols) != len(want) {
		t.Fatalf("expected %d cols, got %d: %v", len(want), len(cols), cols)
	}
	for _, c := range cols {
		if c.Type != want[c.Expr] {
			t.Fatalf("col %s type %s, want %s", c.Expr, c.Type, want[c.Expr])
		}
		delete(want, c.Expr)
	}
	if len(want) != 0 {
		t.Fatalf("missing cols: %v", want)
	}
}
