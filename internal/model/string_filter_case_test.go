package model

import (
	"strings"
	"testing"
)

func stringFilterFixture() (*Model, *DataPreset, *AliasMap) {
	m := &Model{
		Name:  "Person",
		Table: "people",
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
					{Source: "name", Type: "string"},
				},
			},
		},
	}
	preset := m.Presets["list"]
	aliasMap, _ := m.CreateAliasMap(m, preset, nil, nil)
	return m, preset, aliasMap
}

func TestStringEqDefaultCaseInsensitive(t *testing.T) {
	m, preset, aliasMap := stringFilterFixture()
	filters := map[string]any{"name__eq": "John"}

	sb, err := m.BuildIndexQuery(aliasMap, filters, nil, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}
	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if !strings.Contains(sql, "LOWER(main.name) = LOWER(") {
		t.Fatalf("expected case-insensitive eq filter, got SQL: %s", sql)
	}
}

func TestStringCntDefaultCaseInsensitive(t *testing.T) {
	m, preset, aliasMap := stringFilterFixture()
	filters := map[string]any{"name__cnt": "oh"}

	sb, err := m.BuildIndexQuery(aliasMap, filters, nil, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}
	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if !strings.Contains(sql, "main.name ILIKE ") {
		t.Fatalf("expected ILIKE for default substring filter, got SQL: %s", sql)
	}
}

func TestStringOperatorsCaseSensitiveOverride(t *testing.T) {
	m, preset, aliasMap := stringFilterFixture()
	filters := map[string]any{
		"name__eq_cs":    "John",
		"name__cnt_cs":   "oh",
		"name__start_cs": "Jo",
		"name__end_cs":   "hn",
	}

	sb, err := m.BuildIndexQuery(aliasMap, filters, nil, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}
	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if strings.Contains(sql, "ILIKE") {
		t.Fatalf("did not expect ILIKE for case-sensitive operators, got SQL: %s", sql)
	}
	if strings.Contains(sql, "LOWER(main.name)") {
		t.Fatalf("did not expect LOWER() for case-sensitive operators, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "main.name = ") {
		t.Fatalf("expected case-sensitive eq operator, got SQL: %s", sql)
	}
	if strings.Count(sql, "main.name LIKE ") < 3 {
		t.Fatalf("expected LIKE for case-sensitive string operators, got SQL: %s", sql)
	}
}

func TestStringEqCastsOnlyOnTypeMismatch(t *testing.T) {
	m := &Model{
		Name:  "Person",
		Table: "people",
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
					{Source: "is_active", Type: "bool"},
				},
			},
		},
	}
	preset := m.Presets["list"]
	aliasMap, _ := m.CreateAliasMap(m, preset, nil, nil)
	filters := map[string]any{"is_active__eq": "true"}

	sb, err := m.BuildIndexQuery(aliasMap, filters, nil, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}
	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if !strings.Contains(sql, "LOWER(CAST(main.is_active AS TEXT)) = LOWER(") {
		t.Fatalf("expected cast only for type mismatch, got SQL: %s", sql)
	}
}
