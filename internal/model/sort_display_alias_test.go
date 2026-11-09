package model

import (
	"strings"
	"testing"
)

func TestStageSortIgnoresDisplayAliasAndKeepsModelAliases(t *testing.T) {
	previous := Registry
	Registry = map[string]*Model{}
	t.Cleanup(func() { Registry = previous })
	if err := InitRegistry("../../db"); err != nil {
		t.Fatal(err)
	}
	m := Registry["Stage"]
	preset := m.GetPreset("list")
	for _, tc := range []struct {
		name    string
		aliases map[string]string
		column  string
	}{
		{"physical_field", nil, "name"},
		{"model_field_alias", map[string]string{"name": "stage_title"}, "stage_title"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.Aliases = tc.aliases
			// Match the resolver's normalization, and exercise direct builder calls too.
			for _, sorts := range [][]string{{"name asc"}, NormalizeSortsWithAliases(m, []string{"name asc"})} {
				am, err := m.CreateAliasMap(m, preset, nil, sorts)
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
				if strings.ContainsAny(sql, "{}") {
					t.Fatalf("display template leaked into SQL: %s", sql)
				}
				column := "main." + tc.column
				if !strings.Contains(sql, "ORDER BY "+column+" asc") {
					t.Fatalf("wrong sort: %s", sql)
				}
				group := strings.SplitN(sql, "GROUP BY ", 2)
				if len(group) != 2 {
					t.Fatalf("expected grouped Stage query: %s", sql)
				}
				groupColumns := strings.SplitN(group[1], " ORDER BY ", 2)[0]
				if !containsString(strings.Split(groupColumns, ", "), column) {
					t.Fatalf("sort column missing from GROUP BY: %s", sql)
				}
				selectColumns := strings.SplitN(strings.TrimPrefix(sql, "SELECT "), " FROM ", 2)[0]
				if !containsString(strings.Split(selectColumns, ", "), column) {
					t.Fatalf("sort column missing from SELECT: %s", sql)
				}
			}
		})
	}
}
