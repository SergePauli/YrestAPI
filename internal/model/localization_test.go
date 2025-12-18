package model

import (
	"testing"
)

// Ensures parseNodeMap keeps numeric YAML keys (e.g., 0:) available via string keys.
func TestParseNodeMapSupportsNumericKeys(t *testing.T) {
	raw := map[any]any{
		"action_name": map[int]any{
			0: "добавлено",
			1: "изменено",
			2: "удалено",
		},
		"used": map[string]any{
			"true":  "да",
			"false": "нет",
		},
	}

	dict := parseNodeMap(raw)
	node := dict["action_name"]
	if node == nil || node.Children == nil {
		t.Fatalf("action_name node missing or has no children: %+v", node)
	}

	want := map[int]string{
		0: "добавлено",
		1: "изменено",
		2: "удалено",
	}
	for key, expected := range want {
		got, ok := node.Lookup(key) 		
		if !ok {
			t.Fatalf("missing key %v in action_name children", key)
		}
		if got != expected {
			t.Fatalf("key %v mismatch: got %q want %q", key, got, expected)
		}
	}

	// sanity: string-keyed map still works
	usedNode := dict["used"]
	if usedNode == nil {
		t.Fatalf("used node missing")
	}
	if got, ok := usedNode.Lookup("true"); !ok || got != "да" {
		t.Fatalf("used.true lookup failed: got %q ok=%v", got, ok)
	}
}
