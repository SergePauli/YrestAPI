package model

import (
	"strings"
	"testing"
)

func TestSortIgnoresDisplayAliasAndKeepsModelAliases(t *testing.T) {
	preset := &DataPreset{Fields: []Field{
		{Source: "id", Type: "int"},
		{Source: "{title}", Alias: "name", Type: "formatter"},
	}}
	for _, tc := range []struct {
		name    string
		aliases map[string]string
		column  string
	}{
		{"physical_field", nil, "name"},
		{"model_field_alias", map[string]string{"name": "stage_title"}, "stage_title"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &Model{Name: "SortFixture", Table: "sort_fixture", Aliases: tc.aliases}
			// Match the resolver's normalization, and exercise direct builder calls too.
			for _, sorts := range [][]string{{"name asc"}, NormalizeSortsWithAliases(m, []string{"name asc"})} {
				am, err := BuildAliasMap(m, preset, nil, sorts)
				if err != nil {
					t.Fatal(err)
				}
				query, err := m.BuildIndexQuery(am, nil, sorts, preset, 0, 50)
				if err != nil {
					t.Fatal(err)
				}
				sql, _, err := query.ToSql()
				if err != nil {
					t.Fatal(err)
				}
				parts := strings.SplitN(sql, " ORDER BY ", 2)
				if len(parts) != 2 {
					t.Fatalf("missing ORDER BY: %s", sql)
				}
				order := strings.SplitN(parts[1], " LIMIT ", 2)[0]
				if want := "main." + tc.column + " asc"; order != want {
					t.Fatalf("ORDER BY = %q, want %q", order, want)
				}
			}
		})
	}
}
