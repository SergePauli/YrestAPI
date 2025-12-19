package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyTemplateIncludesMergesRelationsAndPresets(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}

	template := `
relations:
  contact:
    model: Contact
    type: belongs_to
  audits:
    model: Audit
    type: has_many
    fk: auditable_id
presets:
  item:
    fields:
      - source: id
        type: int
      - source: template_field
        type: string
`
	if err := os.WriteFile(filepath.Join(dir, "templates", "shared.yml"), []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	modelY := `
table: employees
include: shared
relations:
  boss:
    model: Person
    type: belongs_to
presets:
  item:
    fields:
      - source: template_field
        type: string
        alias: overridden
      - source: own_field
        type: string
`
	if err := os.WriteFile(filepath.Join(dir, "Employee.yml"), []byte(modelY), 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}

	Registry = map[string]*Model{}
	if err := LoadModelsFromDir(dir); err != nil {
		t.Fatalf("LoadModelsFromDir: %v", err)
	}

	m := Registry["Employee"]
	if m == nil {
		t.Fatalf("Employee model not loaded")
	}
	// relations merged
	if _, ok := m.Relations["contact"]; !ok {
		t.Fatalf("template relation not merged")
	}
	if _, ok := m.Relations["boss"]; !ok {
		t.Fatalf("model relation missing")
	}

	p := m.Presets["item"]
	if p == nil {
		t.Fatalf("preset item missing")
	}
	if len(p.Fields) != 4 {
		t.Fatalf("expected 4 fields after merge, got %d", len(p.Fields))
	}
	foundOverride := false
	for _, f := range p.Fields {
		if f.Alias == "overridden" {
			foundOverride = true
			break
		}
	}
	if !foundOverride {
		t.Fatalf("override field from model not present")
	}
	// override relation pulls type from template
	rel := m.Relations["audits"]
	if rel == nil {
		t.Fatalf("audits relation missing")
	}
	if rel.Type != "has_many" || rel.FK != "auditable_id" {
		t.Fatalf("relation not merged from template: %#v", rel)
	}
}
