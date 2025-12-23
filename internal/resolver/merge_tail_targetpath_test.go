package resolver

import (
	"testing"

	"YrestAPI/internal/model"
)

// Regression: tails with TargetPath must read PK from nested context, not root.
func TestMergeTailsUsesTargetPathContext(t *testing.T) {
	// Build minimal models/presets resembling Audit -> Person -> PersonName(has_one)
	origRegistry := model.Registry
	defer func() { model.Registry = origRegistry }()

	audit := &model.Model{
		Name:      "Audit",
		Table:     "audits",
		Relations: map[string]*model.ModelRelation{},
		Presets:   map[string]*model.DataPreset{},
	}
	person := &model.Model{
		Name:      "Person",
		Table:     "people",
		Relations: map[string]*model.ModelRelation{},
		Presets:   map[string]*model.DataPreset{},
	}
	personName := &model.Model{
		Name:      "PersonName",
		Table:     "person_names",
		Relations: map[string]*model.ModelRelation{},
		Presets:   map[string]*model.DataPreset{},
	}

	audit.Relations["person"] = &model.ModelRelation{Type: "belongs_to", Model: "Person"}
	person.Relations["person_name"] = &model.ModelRelation{Type: "has_one", Model: "PersonName", Where: ".used = true"}

	audit.Presets["card"] = &model.DataPreset{
		Name: "card",
		Fields: []model.Field{
			{Source: "id", Type: "int"},
			{Source: "person", Type: "preset", NestedPreset: "head"},
		},
	}
	person.Presets["head"] = &model.DataPreset{
		Name: "head",
		Fields: []model.Field{
			{Source: "person_name", Type: "preset", NestedPreset: "item", Alias: "name"},
		},
	}
	personName.Presets["item"] = &model.DataPreset{
		Name: "item",
		Fields: []model.Field{
			{Source: "id", Type: "int"},
			{Source: "name", Type: "string"},
		},
	}

	model.Registry = map[string]*model.Model{
		"Audit":      audit,
		"Person":     person,
		"PersonName": personName,
	}
	if err := model.LinkModelRelations(); err != nil {
		t.Fatalf("link relations: %v", err)
	}

	tails := collectTails(audit, audit.Presets["card"], "")
	if len(tails) != 1 {
		t.Fatalf("expected 1 tail, got %d", len(tails))
	}
	tail := tails[0]
	if tail.TargetPath != "person" {
		t.Fatalf("expected TargetPath 'person', got %q", tail.TargetPath)
	}

	// Main items after ScanFlatRows/FoldFlatRows (PK of tail lives under targetPath)
	items := []map[string]any{
		{"id": 1, "person_id": 1, "person": map[string]any{"id": 1}},
		{"id": 2, "person_id": 1, "person": map[string]any{"id": 1}},
	}

	// Child rows grouped by FK (person_id) for has_one tail alias "name"
	groupedByAlias := map[string]map[any][]map[string]any{
		tail.FieldAlias: {
			1: {
				{"person_id": 1, "id": 11, "name": "Сергей"},
			},
		},
	}

	// Merge tails (mirrors Resolver's merging loop)
	for i := range items {
		ctx := getTargetContext(items[i], tail.TargetPath)
		if ctx == nil {
			t.Fatalf("ctx nil for item %d", i)
		}
		pid := ctx[tail.Rel.PK]
		target := ensureTargetContext(items[i], tail.TargetPath)

		var groups []map[string]any
		if m, ok := groupedByAlias[tail.FieldAlias]; ok {
			if g, ok := m[pid]; ok {
				groups = g
			}
		}
		if len(groups) == 0 {
			t.Fatalf("no groups for item %d pid %v", i, pid)
		}

		if tail.LimitOne {
			for _, row := range groups {
				delete(row, tail.Rel.FK)
			}
			target[tail.FieldAlias] = groups[0]
		} else {
			for _, row := range groups {
				delete(row, tail.Rel.FK)
			}
			target[tail.FieldAlias] = groups
		}
	}

	for i, it := range items {
		personCtx, ok := it["person"].(map[string]any)
		if !ok {
			t.Fatalf("item %d person not map", i)
		}
		name, ok := personCtx["name"].(map[string]any)
		if !ok {
			t.Fatalf("item %d name not map", i)
		}
		if _, exists := name["person_id"]; exists {
			t.Fatalf("item %d: person_id should be removed from merged tail", i)
		}
		if got := name["name"]; got != "Сергей" {
			t.Fatalf("item %d: expected name Сергей, got %v", i, got)
		}
	}
}
