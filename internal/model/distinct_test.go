package model

import (
	"strings"
	"testing"
)

func TestBuildDistinctValuesQuerySimpleFieldIgnoresPreset(t *testing.T) {
	m := &Model{Name: "Person", Table: "people", Relations: map[string]*ModelRelation{}, Computable: map[string]*Computable{}}
	am, err := BuildAliasMap(m, nil, nil, []string{"department_id ASC"})
	if err != nil {
		t.Fatal(err)
	}
	query, err := m.BuildDistinctValuesQuery(am, nil, "department_id", 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := query.ToSql()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SELECT DISTINCT main.department_id AS value", "ORDER BY value ASC", "LIMIT 10", "OFFSET 5"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected %q in SQL: %s", want, sql)
		}
	}
}

func TestBuildDistinctValuesQueryKeepsHasManyFilterDeduplication(t *testing.T) {
	child := &Model{Name: "Membership", Table: "memberships", Relations: map[string]*ModelRelation{}, Computable: map[string]*Computable{}}
	m := &Model{
		Name: "Person", Table: "people", Computable: map[string]*Computable{},
		Relations: map[string]*ModelRelation{
			"memberships": {Type: "has_many", Model: "Membership", FK: "person_id", PK: "id"},
		},
	}
	m.Relations["memberships"].SetModelRef(child)
	filters := map[string]any{"memberships.active__eq": true}
	am, err := BuildAliasMap(m, nil, filters, []string{"department_id ASC"})
	if err != nil {
		t.Fatal(err)
	}
	query, err := m.BuildDistinctValuesQuery(am, filters, "department_id", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := query.ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "LEFT JOIN memberships AS") || !strings.Contains(sql, "SELECT DISTINCT main.department_id AS value") {
		t.Fatalf("expected has_many join and scalar deduplication: %s", sql)
	}
}

func TestResolveDistinctFieldRejectsHasManyPath(t *testing.T) {
	child := &Model{Name: "Membership", Table: "memberships", Relations: map[string]*ModelRelation{}, Computable: map[string]*Computable{}}
	m := &Model{Name: "Person", Table: "people", Computable: map[string]*Computable{}, Relations: map[string]*ModelRelation{
		"memberships": {Type: "has_many", Model: "Membership", FK: "person_id", PK: "id"},
	}}
	m.Relations["memberships"].SetModelRef(child)
	am, err := BuildAliasMap(m, nil, nil, []string{"memberships.role ASC"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = m.ResolveDistinctField(am, "memberships.role")
	if err == nil || !strings.Contains(err.Error(), "traverses has_many") {
		t.Fatalf("expected has_many validation error, got %v", err)
	}
}

func TestBuildDistinctCountIncludesNullGroup(t *testing.T) {
	m := &Model{Name: "Person", Table: "people", Relations: map[string]*ModelRelation{}, Computable: map[string]*Computable{}}
	am, err := BuildAliasMap(m, nil, nil, []string{"department_id ASC"})
	if err != nil {
		t.Fatal(err)
	}
	query, err := m.BuildDistinctCountQuery(am, nil, "department_id")
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := query.ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "COUNT(*)") || !strings.Contains(sql, "SELECT DISTINCT main.department_id AS value") || strings.Contains(sql, "COUNT(DISTINCT") {
		t.Fatalf("count must wrap the distinct value set (including NULL): %s", sql)
	}
}
