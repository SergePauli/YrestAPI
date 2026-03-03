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

func TestRelationNameFromTable(t *testing.T) {
	if got := relationNameFromTable(" Project_Members "); got != "project_members" {
		t.Fatalf("relationNameFromTable=%q, want %q", got, "project_members")
	}
}

func TestUniqueRelationName(t *testing.T) {
	relations := map[string]relationYAML{
		"project_members":   {Type: "has_many", Model: "ProjectMember"},
		"project_members_2": {Type: "has_many", Model: "ProjectMember"},
	}
	got := uniqueRelationName(relations, "project_members")
	if got != "project_members_3" {
		t.Fatalf("uniqueRelationName=%q, want %q", got, "project_members_3")
	}
}

func TestAddHasManyRelationPresets(t *testing.T) {
	presets := map[string]presetYAML{
		"item":      {Fields: []fieldYAML{{Source: "id", Type: "int"}}},
		"full_info": {Fields: []fieldYAML{{Source: "id", Type: "int"}}},
	}
	relations := map[string]relationYAML{
		"organization": {Type: "belongs_to", Model: "Organization"},
		"members":      {Type: "has_many", Model: "ProjectMember"},
	}

	addHasManyRelationPresets(presets, relations)

	got, ok := presets["with_members"]
	if !ok {
		t.Fatalf("expected with_members preset to be generated")
	}
	if len(got.Fields) != 1 {
		t.Fatalf("with_members fields len=%d, want 1", len(got.Fields))
	}
	if got.Fields[0].Source != "members" || got.Fields[0].Type != "preset" || got.Fields[0].Preset != "item" {
		t.Fatalf("unexpected with_members field: %#v", got.Fields[0])
	}
	if _, exists := presets["with_organization"]; exists {
		t.Fatalf("did not expect preset for non-has_many relation")
	}
}

func TestUniquePresetName(t *testing.T) {
	presets := map[string]presetYAML{
		"item":           {},
		"with_members":   {},
		"with_members_2": {},
	}
	got := uniquePresetName(presets, "with_members")
	if got != "with_members_3" {
		t.Fatalf("uniquePresetName=%q, want %q", got, "with_members_3")
	}
}
