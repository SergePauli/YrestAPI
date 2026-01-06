package resolver

import (
	"testing"

	"YrestAPI/internal/model"
)

// belongs_to контейнер с одними nil должен схлопываться в nil.
func TestFinalizeItems_BelongsToAllNilToNull(t *testing.T) {
	statusModel := &model.Model{
		Name:  "Status",
		Table: "statuses",
		Presets: map[string]*model.DataPreset{
			"item": {
				Name: "item",
				Fields: []model.Field{
					{Type: "int", Source: "id"},
					{Type: "string", Source: "name"},
				},
			},
		},
	}

	stageModel := &model.Model{
		Name:  "Stage",
		Table: "stages",
		Relations: map[string]*model.ModelRelation{
			"status": {Type: "belongs_to"},
		},
		Presets: map[string]*model.DataPreset{
			"list": {
				Name: "list",
				Fields: []model.Field{
					{Type: "preset", Source: "status", NestedPreset: "item"},
				},
			},
		},
	}
	stageModel.Relations["status"].SetModelRef(statusModel)

	items := []map[string]any{
		{
			"id": 1,
			"status": map[string]any{
				"id":   nil,
				"name": nil,
			},
		},
	}

	if err := finalizeItems(stageModel, stageModel.Presets["list"], items); err != nil {
		t.Fatalf("finalizeItems error: %v", err)
	}

	if items[0]["status"] != nil {
		t.Fatalf("status should be nil, got: %#v", items[0]["status"])
	}
}
