package resolver

import (
	"testing"

	"YrestAPI/internal/model"
)

// Ensures internal preset fields are removed after nested_field copies data out.
func TestFinalizeItems_RemovesInternalPresetAfterNestedField(t *testing.T) {
	personModel := &model.Model{Table: "people", Presets: map[string]*model.DataPreset{
		"head": {
			Name: "head",
			Fields: []model.Field{
				{Type: "string", Source: "name"},
			},
		},
	}}
	m := &model.Model{
		Table: "audits",
		Relations: map[string]*model.ModelRelation{
			"person": {Type: "belongs_to"},
		},
		Presets: map[string]*model.DataPreset{
			"card": {
				Name: "card",
				Fields: []model.Field{
					{Type: "preset", Source: "person", NestedPreset: "head", Internal: true},
					{Type: "nested_field", Source: "{person.name}", Alias: "who"},
				},
			},
		},
	}
	// link relation
	rel := m.Relations["person"]
	rel.SetModelRef(personModel)

	items := []map[string]any{
		{
			"id": 1,
			"person": map[string]any{
				"name": "John",
			},
		},
	}

	if err := finalizeItems(m, m.Presets["card"], items); err != nil {
		t.Fatalf("finalizeItems error: %v", err)
	}

	if _, ok := items[0]["person"]; ok {
		t.Fatalf("internal preset not removed: %+v", items[0])
	}
	if got := items[0]["who"]; got != "John" {
		t.Fatalf("who not copied: %v", got)
	}
}
