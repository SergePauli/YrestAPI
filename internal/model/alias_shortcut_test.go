package model

import (
	"strings"
	"testing"
)

func aliasShortcutFixture(t *testing.T) (*Model, *DataPreset, map[string]any) {
	t.Helper()

	person := &Model{
		Name:  "Person",
		Table: "people",
		Relations: map[string]*ModelRelation{
			"contragent": {Type: "belongs_to", Model: "Contragent"},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
				},
			},
		},
		Aliases: map[string]string{
			"org": "contragent.organization",
		},
	}

	contragent := &Model{
		Name:  "Contragent",
		Table: "contragents",
		Relations: map[string]*ModelRelation{
			"organization": {Type: "belongs_to", Model: "Organization"},
		},
	}

	organization := &Model{
		Name:  "Organization",
		Table: "organizations",
	}

	prevRegistry := Registry
	Registry = map[string]*Model{
		"Person":       person,
		"Contragent":   contragent,
		"Organization": organization,
	}
	t.Cleanup(func() { Registry = prevRegistry })

	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}
	if err := BuildPresetAliasMaps(); err != nil {
		t.Fatalf("BuildPresetAliasMaps: %v", err)
	}

	preset := person.Presets["list"]
	filters := map[string]any{"org.name__cnt": "IBM"}

	return person, preset, filters
}

// Проверяем, что короткие алиасы разворачиваются в пути и используются в JOIN/WHERE.
func TestAliasShortcutInFilters(t *testing.T) {
	person, preset, filters := aliasShortcutFixture(t)

	aliasMap, err := person.CreateAliasMap(person, preset, filters, nil)
	if err != nil {
		t.Fatalf("CreateAliasMap: %v", err)
	}

	sb, err := person.BuildIndexQuery(aliasMap, NormalizeFiltersWithAliases(person, filters), nil, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}
	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if !strings.Contains(sql, "organizations AS") {
		t.Fatalf("expected join to organizations, got: %s", sql)
	}
	if !strings.Contains(sql, "t1.name") {
		t.Fatalf("alias in filter not applied, sql: %s", sql)
	}
}

// Проверяем, что BuildCountQuery тоже использует развёрнутые алиасы из фильтров.
func TestAliasShortcutInCount(t *testing.T) {
	person, preset, filters := aliasShortcutFixture(t)

	aliasMap, err := person.CreateAliasMap(person, preset, filters, nil)
	if err != nil {
		t.Fatalf("CreateAliasMap: %v", err)
	}

	sb, err := person.BuildCountQuery(aliasMap, preset, filters)
	if err != nil {
		t.Fatalf("BuildCountQuery: %v", err)
	}

	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if !strings.Contains(sql, "organizations AS") {
		t.Fatalf("expected join to organizations, got: %s", sql)
	}
	if strings.Contains(sql, "org.name") {
		t.Fatalf("raw alias leaked into WHERE, sql: %s", sql)
	}
	if !strings.Contains(sql, "t1.name") {
		t.Fatalf("alias in filter not applied in count, sql: %s", sql)
	}
}

func TestAliasShortcutInSortsNested(t *testing.T) {
	org := &Model{Name: "Organization", Table: "organizations"}
	contrOrg := &Model{
		Name:  "ContragentOrganization",
		Table: "contragent_organizations",
		Relations: map[string]*ModelRelation{
			"organization": {Type: "belongs_to", Model: "Organization"},
		},
	}
	contr := &Model{
		Name:  "Contragent",
		Table: "contragents",
		Aliases: map[string]string{
			"org": "contragent_organization.organization",
		},
		Relations: map[string]*ModelRelation{
			"contragent_organization": {Type: "belongs_to", Model: "ContragentOrganization"},
		},
	}
	contract := &Model{
		Name:  "Contract",
		Table: "contracts",
		Relations: map[string]*ModelRelation{
			"contragent": {Type: "belongs_to", Model: "Contragent"},
		},
	}
	revision := &Model{
		Name:  "Revision",
		Table: "revisions",
		Relations: map[string]*ModelRelation{
			"contract": {Type: "belongs_to", Model: "Contract"},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
				},
			},
		},
	}

	prev := Registry
	Registry = map[string]*Model{
		"Revision":               revision,
		"Contract":               contract,
		"Contragent":             contr,
		"ContragentOrganization": contrOrg,
		"Organization":           org,
	}
	t.Cleanup(func() { Registry = prev })

	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}

	sorts := []string{"contract.contragent.org.name DESC"}
	preset := revision.Presets["list"]

	aliasMap, err := revision.CreateAliasMap(revision, preset, nil, sorts)
	if err != nil {
		t.Fatalf("CreateAliasMap: %v", err)
	}

	sb, err := revision.BuildIndexQuery(aliasMap, nil, sorts, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}
	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}

	if strings.Contains(sql, "contract.contragent.org.name") {
		t.Fatalf("raw alias remained in ORDER BY: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY t3.name DESC") {
		t.Fatalf("expected ORDER BY on expanded alias, got: %s", sql)
	}
}

func TestDistinctSelectAddsOrderColumns(t *testing.T) {
	b := &Model{
		Name:  "B",
		Table: "b",
	}
	a := &Model{
		Name:  "A",
		Table: "a",
		Relations: map[string]*ModelRelation{
			"b": {Type: "has_one", Model: "B", FK: "a_id", PK: "id"},
		},
		Presets: map[string]*DataPreset{
			"list": {
				Fields: []Field{
					{Source: "id", Type: "int"},
				},
			},
		},
	}
	prev := Registry
	Registry = map[string]*Model{"A": a, "B": b}
	t.Cleanup(func() { Registry = prev })
	if err := LinkModelRelations(); err != nil {
		t.Fatalf("LinkModelRelations: %v", err)
	}

	sorts := []string{"b.name ASC"}
	preset := a.Presets["list"]
	aliasMap, err := a.CreateAliasMap(a, preset, nil, sorts)
	if err != nil {
		t.Fatalf("CreateAliasMap: %v", err)
	}
	sb, err := a.BuildIndexQuery(aliasMap, nil, sorts, preset, 0, 0)
	if err != nil {
		t.Fatalf("BuildIndexQuery: %v", err)
	}
	sql, _, err := sb.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}
	if !strings.Contains(sql, "t0.name") {
		t.Fatalf("expected order column added to SELECT, got: %s", sql)
	}
}
