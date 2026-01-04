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
