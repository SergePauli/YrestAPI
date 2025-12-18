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

	cols, types := m.ScanColumns(p, aliasMap, "")
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d: %v", len(cols), cols)
	}

	want := map[string]string{
		"main.id":         "int",
		"main.created_at": "datetime",
	}
	for _, c := range cols {
		ft, ok := types[c]
		if !ok {
			t.Fatalf("type for col %q missing", c)
		}
		if ft != want[c] {
			t.Fatalf("col %q type mismatch: got %q, want %q", c, ft, want[c])
		}
		delete(want, c)
	}
	if len(want) != 0 {
		t.Fatalf("missing columns: %v", want)
	}
}
