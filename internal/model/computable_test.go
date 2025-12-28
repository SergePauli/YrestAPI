package model

import (
	"strings"
	"testing"
)

func TestComputableUsedInSelectFilterAndSort(t *testing.T) {
	m := &Model{
		Name:  "Person",
		Table: "people",
		Computable: map[string]*Computable{
			"fio": {
				Source: "(SELECT concat(n.surname, ' ', n.name) FROM namings n WHERE n.person_id = main.id)",
				Type:   "string",
			},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "fio", Alias: "fio", Type: "computable"},
				},
			},
		},
	}
	Registry = map[string]*Model{"Person": m}
	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}

	preset := m.Presets["list"]
	filters := map[string]any{"fio__cnt": "ann"}
	sorts := []string{"fio DESC"}

	aliasMap, err := m.CreateAliasMap(m, preset, filters, sorts)
	if err != nil {
		t.Fatalf("CreateAliasMap error: %v", err)
	}

	sb, err := m.BuildIndexQuery(aliasMap, filters, sorts, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery error: %v", err)
	}

	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql error: %v", err)
	}

	if !strings.Contains(sql, "AS \"fio\"") {
		t.Fatalf("select does not contain aliased computable field: %s", sql)
	}
	if !strings.Contains(sql, "SELECT concat(n.surname") {
		t.Fatalf("computable subquery not present in SQL: %s", sql)
	}
	if !strings.Contains(sql, "WHERE ((SELECT concat(n.surname") {
		t.Fatalf("where clause does not use computable expression: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY (SELECT concat(n.surname") {
		t.Fatalf("order by does not use computable expression: %s", sql)
	}
}

func TestComputablePlaceholderUsesAlias(t *testing.T) {
	stageModel := &Model{
		Name:  "Stage",
		Table: "stages",
	}
	m := &Model{
		Name:  "Project",
		Table: "projects",
		Relations: map[string]*ModelRelation{
			"stages": {Type: "has_many", Model: "Stage"},
		},
		Computable: map[string]*Computable{
			"summ": {
				Source: "(select sum({stages}.amount))",
				Type:   "float",
			},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
					{Source: "summ", Alias: "summ", Type: "computable"},
				},
			},
		},
	}
	Registry = map[string]*Model{"Project": m, "Stage": stageModel}
	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}

	preset := m.Presets["list"]
	filters := map[string]any{"summ__gt": 10}
	sorts := []string{"summ DESC"}

	aliasMap, err := m.CreateAliasMap(m, preset, filters, sorts)
	if err != nil {
		t.Fatalf("CreateAliasMap: %v", err)
	}

	sb, err := m.BuildIndexQuery(aliasMap, filters, sorts, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}

	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if !strings.Contains(sql, "LEFT JOIN stages AS t0") {
		t.Fatalf("expected join for stages, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "sum(t0.amount)") {
		t.Fatalf("placeholder {stages} not replaced with alias in SQL: %s", sql)
	}
}
