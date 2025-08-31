package model

import (
	"reflect"
	"testing"
)

func TestFoldFlatRowByPreset_Simple(t *testing.T) {
	in := map[string]any{
		"id":                 1024,
		"name":               "Русский Продукт",
		"address.id":         377,
		"address.value":      "660013, г. Красноярск...",
		"address.area.id":    660000,
		"address.area.name":  "КРАСНОЯРСК",
		"requisites.id":      1128,
		"requisites.name":    "Русский Продукт",
		"requisites.org.id":  1128,
		"requisites.org.name":"Русский Продукт",
	}

	got := FoldFlatRowByPreset(in)

	want := map[string]any{
		"id":   1024,
		"name": "Русский Продукт",
		"address": map[string]any{
			"id":    377,
			"value": "660013, г. Красноярск...",
			"area": map[string]any{
				"id":   660000,
				"name": "КРАСНОЯРСК",
			},
		},
		"requisites": map[string]any{
			"id":   1128,
			"name": "Русский Продукт",
			"org": map[string]any{
				"id":   1128,
				"name": "Русский Продукт",
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FoldFlatRowByPreset mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFoldFlatRowByPreset_EdgeCases(t *testing.T) {
	// пустые и "обрезанные" ключи не ломают свёртку
	in := map[string]any{
		"a.":        1,              // хвостовая точка → трактуем как leaf "a"
		"b":         2,              // уже leaf
		"c.d":       3,              // двойная точка схлопнется в два уровня
		"e.f.g.h":   4,
		"e.f.g.i":   5,
	}

	got := FoldFlatRowByPreset(in)
	// проверим только ключевые свойства
	// a → 1
	if v, ok := got["a"]; !ok || v.(int) != 1 {
		t.Fatalf("expected a=1, got: %#v", got["a"])
	}
	// b → 2
	if v, ok := got["b"]; !ok || v.(int) != 2 {
		t.Fatalf("expected b=2, got: %#v", got["b"])
	}
	// c.d → 3
	cd := got["c"].(map[string]any)["d"].(int)
	if cd != 3 {
		t.Fatalf("expected c.d=3, got: %v", cd)
	}
	// e.f.g.{h,i}
	efg := got["e"].(map[string]any)["f"].(map[string]any)["g"].(map[string]any)
	if efg["h"].(int) != 4 || efg["i"].(int) != 5 {
		t.Fatalf("expected e.f.g.h=4 and i=5, got: %#v", efg)
	}
}