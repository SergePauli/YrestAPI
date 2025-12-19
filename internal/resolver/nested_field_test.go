package resolver

import (
	"testing"

	"YrestAPI/internal/model"
)

func TestNestedFieldCopiesNestedValue(t *testing.T) {
	p := &model.DataPreset{
		Name: "card",
		Fields: []model.Field{
			{Type: "nested_field", Source: "{person.contacts}", Alias: "contacts"},
		},
	}
	items := []map[string]any{
		{
			"id": 1,
			"person": map[string]any{
				"contacts": []map[string]any{
					{"type": "phone", "value": "123"},
				},
			},
		},
	}

	if err := applyAllFormatters(nil, p, items, ""); err != nil {
		t.Fatalf("applyAllFormatters error: %v", err)
	}

	got, ok := items[0]["contacts"].([]map[string]any)
	if !ok {
		t.Fatalf("contacts not copied or wrong type: %#v", items[0]["contacts"])
	}
	if len(got) != 1 || got[0]["value"] != "123" {
		t.Fatalf("unexpected contacts: %#v", got)
	}
}
