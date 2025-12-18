package resolver

import (
	"testing"

	"YrestAPI/internal/model"
)

func TestApplyLocalization_IntFieldNormalizesNumericKeys(t *testing.T) {
	origDict := model.ActiveDict
	origRegistry := model.Registry
	t.Cleanup(func() {
		model.ActiveDict = origDict
		model.Registry = origRegistry
	})

	model.ActiveDict = map[any]*model.LocaleNode{
		"status": {
			Children: map[any]*model.LocaleNode{
				0: {Value: "inactive"},
				1: {Value: "active"},
			},
		},
	}

	preset := &model.DataPreset{
		Name: "list",
		Fields: []model.Field{
			{Type: "int", Source: "status", Localize: true},
		},
	}

	m := &model.Model{
		Presets: map[string]*model.DataPreset{
			"list": preset,
		},
	}
	model.Registry = map[string]*model.Model{
		"Dummy": m,
	}

	items := []map[string]any{
		{"status": int64(1)},  // int64 из БД должен мапиться на int-ключ словаря
		{"status": uint32(0)}, // и uint32 также нормализуется
	}

	applyLocalization(m, preset, items)

	if got := items[0]["status"]; got != "active" {
		t.Fatalf("item0 status: got %v, want active", got)
	}
	if got := items[1]["status"]; got != "inactive" {
		t.Fatalf("item1 status: got %v, want inactive", got)
	}
}
