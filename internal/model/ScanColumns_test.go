package model

import (
	"testing"
)

func TestScanColumnsIncludesDatetimeFields(t *testing.T) {
	m := &Model{}
	p := &DataPreset{
		Fields: []Field{
			{Source: "id", Type: "int"},
			{Source: "created_at", Type: "datetime"},
		},
	}

	aliasMap := &AliasMap{
		PathToAlias: map[string]string{},
		AliasToPath: map[string]string{},
	}

	cols := m.ScanColumns(p, aliasMap, "")
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d: %v", len(cols), cols)
	}

	want := map[string]string{
		"main.id":         "int",
		"main.created_at": "datetime",
	}
	for _, c := range cols {
		if c.Type != want[c.Expr] {
			t.Fatalf("col %q type mismatch: got %q, want %q", c.Expr, c.Type, want[c.Expr])
		}
		delete(want, c.Expr)
	}
	if len(want) != 0 {
		t.Fatalf("missing columns: %v", want)
	}
}
