package model

import (
	"os"
	"path/filepath"
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

func TestLoadLocalesReadsLayoutSettings(t *testing.T) {
	tmpDir := t.TempDir()
	data := []byte(`
layoutSettings:
  date: "DD"
  ttime: "TT"
  datetime: "DT"
sample: "value"
`)
	locale := "xx"

	origDict := ActiveDict
	origLocale := ActiveLocale
	origLayouts := ActiveLayouts
	origLocaleDir := LocaleDir
	t.Cleanup(func() {
		ActiveDict = origDict
		ActiveLocale = origLocale
		ActiveLayouts = origLayouts
		LocaleDir = origLocaleDir
	})

	LocaleDir = filepath.Join(tmpDir, "cfg", "locales")
	if err := os.MkdirAll(LocaleDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	destPath := filepath.Join(LocaleDir, locale+".yml")
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		t.Fatalf("write locale file: %v", err)
	}

	if err := LoadLocales(locale); err != nil {
		t.Fatalf("LoadLocales: %v", err)
	}

	if ActiveLayouts.Date != "DD" || ActiveLayouts.Time != "TT" || ActiveLayouts.DateTime != "DT" {
		t.Fatalf("layout settings not loaded: %+v", ActiveLayouts)
	}
	if ActiveLocale != locale {
		t.Fatalf("ActiveLocale = %s, want %s", ActiveLocale, locale)
	}
	if _, ok := ActiveDict["layoutSettings"]; ok {
		t.Fatalf("layoutSettings should not remain in ActiveDict")
	}
}
