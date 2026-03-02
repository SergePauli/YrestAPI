package sqlimport

import (
	"strings"
	"testing"
)

func TestTableToModelName(t *testing.T) {
	cases := map[string]string{
		"areas":          "Area",
		"countries":      "Country",
		"project_stages": "ProjectStage",
		"people":         "Person",
		"statuses":       "Status",
		"order_statuses": "OrderStatus",
	}
	for in, want := range cases {
		if got := tableToModelName(in); got != want {
			t.Fatalf("tableToModelName(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestChooseNameLikeColumn(t *testing.T) {
	cols := []Column{
		{Name: "id", DataType: "integer"},
		{Name: "code", DataType: "character varying"},
		{Name: "display_name", DataType: "text"},
		{Name: "created_at", DataType: "timestamp without time zone"},
	}
	got := chooseNameLikeColumn(cols, "id")
	if got != "display_name" {
		t.Fatalf("chooseNameLikeColumn=%q, want %q", got, "display_name")
	}
}

func TestChooseIDColumnFallbackToSinglePK(t *testing.T) {
	cols := []Column{
		{Name: "area_uuid", DataType: "uuid", UDTName: "uuid"},
		{Name: "name", DataType: "text"},
	}
	src, typ, ok := chooseIDColumn(cols, []string{"area_uuid"})
	if !ok {
		t.Fatal("expected id column to be selected from single PK")
	}
	if src != "area_uuid" || typ != "UUID" {
		t.Fatalf("got source=%q type=%q, want area_uuid/UUID", src, typ)
	}
}

func TestRelationNameFromFK(t *testing.T) {
	cases := []struct {
		column  string
		ref     string
		wantRel string
	}{
		{column: "person_id", ref: "people", wantRel: "person"},
		{column: "created_by_user_id", ref: "users", wantRel: "created_by_user"},
		{column: "", ref: "order_statuses", wantRel: "order_status"},
		{column: "   ", ref: "task_kinds", wantRel: "task_kind"},
		{column: "contragent", ref: "contragents", wantRel: "contragent"},
		{column: "organization_uuid", ref: "organizations", wantRel: "organization_uuid"},
	}
	for _, tc := range cases {
		got := relationNameFromFK(tc.column, tc.ref)
		want := tc.wantRel
		if got != want {
			t.Fatalf("relation name mismatch: got %q, want %q", got, want)
		}
	}
}

func TestBuildListTablesQuery(t *testing.T) {
	qSimple := buildListTablesQuery(true)
	if !strings.Contains(qSimple, "constraint_type = 'FOREIGN KEY'") {
		t.Fatalf("expected only-simple query to filter foreign keys, got: %s", qSimple)
	}

	qFull := buildListTablesQuery(false)
	if strings.Contains(qFull, "constraint_type = 'FOREIGN KEY'") {
		t.Fatalf("did not expect full query to filter foreign keys, got: %s", qFull)
	}
}
