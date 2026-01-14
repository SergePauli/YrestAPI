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

func TestComputableHasManyCTE(t *testing.T) {
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
			"is_funded": {
				Source: "CASE WHEN SUM(CAST({stages}.is_funded AS INT)) = 0 THEN false " +
					"WHEN SUM(CAST({stages}.is_funded AS INT)) = COUNT({stages}.id) THEN true ELSE null END",
				Type: "bool",
			},
			"cost": {
				Source: "coalesce(sum({stages}.cost), 0)",
				Type:   "float",
			},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
					{Source: "is_funded", Alias: "is_funded", Type: "computable"},
					{Source: "cost", Alias: "cost", Type: "computable"},
				},
			},
		},
	}
	Registry = map[string]*Model{"Project": m, "Stage": stageModel}
	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}

	preset := m.Presets["list"]
	filters := map[string]any{"is_funded__eq": false, "status_id__in": []int{0, 1}}
	sorts := []string{"id DESC"}

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

	if !strings.Contains(sql, "WITH t0_agg AS") {
		t.Fatalf("expected CTE for has_many aggregate, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "LEFT JOIN t0_agg ON t0_agg.id = main.id") {
		t.Fatalf("expected join to CTE, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "t0_agg.\"is_funded\"") {
		t.Fatalf("expected filter to use CTE column, got SQL: %s", sql)
	}
	if strings.Contains(sql, "HAVING") {
		t.Fatalf("unexpected HAVING with CTE-based filtering, got SQL: %s", sql)
	}
	if strings.Count(sql, "LEFT JOIN stages AS t0") != 1 {
		t.Fatalf("expected stages join only inside CTE, got SQL: %s", sql)
	}
}

func TestComputableBareColumnsQualifiedInFilters(t *testing.T) {
	naming := &Model{
		Name:  "Naming",
		Table: "namings",
		Computable: map[string]*Computable{
			"fio": {
				Source: "(select concat(surname, ' ', name, ' ', patrname))",
				Type:   "string",
			},
		},
		Presets: map[string]*DataPreset{
			"item": {
				Fields: []Field{
					{Source: "id", Type: "int"},
				},
			},
		},
	}
	person := &Model{
		Name:  "Person",
		Table: "people",
		Relations: map[string]*ModelRelation{
			"naming": {Type: "belongs_to", Model: "Naming"},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
				},
			},
		},
	}
	Registry = map[string]*Model{"Person": person, "Naming": naming}
	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}

	preset := person.Presets["list"]
	filters := map[string]any{"naming.fio__cnt": "ann"}

	aliasMap, err := person.CreateAliasMap(person, preset, filters, nil)
	if err != nil {
		t.Fatalf("CreateAliasMap error: %v", err)
	}

	sb, err := person.BuildIndexQuery(aliasMap, filters, nil, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery error: %v", err)
	}

	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql error: %v", err)
	}

	if strings.Contains(sql, "concat(surname") {
		t.Fatalf("computable columns not qualified in SQL: %s", sql)
	}
	if !strings.Contains(sql, "concat(t0.surname") {
		t.Fatalf("computable columns not qualified with alias: %s", sql)
	}
	if !strings.Contains(sql, "WHERE ((select concat(") {
		t.Fatalf("WHERE clause does not use computable expression: %s", sql)
	}
}
