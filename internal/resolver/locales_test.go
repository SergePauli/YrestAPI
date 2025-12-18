package resolver

import (
	"testing"
	"time"

	"YrestAPI/internal/model"
)

func TestApplyLocalization_IntFieldNormalizesNumericKeys(t *testing.T) {
	origDict := model.ActiveDict
	origRegistry := model.Registry
	origLocale := model.ActiveLocale
	origLayouts := model.ActiveLayouts
	t.Cleanup(func() {
		model.ActiveDict = origDict
		model.Registry = origRegistry
		model.ActiveLocale = origLocale
		model.ActiveLayouts = origLayouts
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

func TestApplyLocalization_FormatsTemporalFieldsByLocale(t *testing.T) {
	origDict := model.ActiveDict
	origLocale := model.ActiveLocale
	origLayouts := model.ActiveLayouts
	t.Cleanup(func() {
		model.ActiveDict = origDict
		model.ActiveLocale = origLocale
		model.ActiveLayouts = origLayouts
	})

	model.ActiveDict = map[any]*model.LocaleNode{} // no dictionary lookups needed
	model.ActiveLocale = "ru"
	model.ActiveLayouts = model.LayoutSettings{
		Date:     "02.01.2006",
		Time:     "15:04:05",
		DateTime: "02.01.2006 15:04:05",
	}

	preset := &model.DataPreset{
		Name: "list",
		Fields: []model.Field{
			{Type: "datetime", Source: "created_at", Localize: true},
			{Type: "date", Source: "birth_date", Localize: true},
			{Type: "time", Source: "start_time", Localize: true},
		},
	}
	m := &model.Model{
		Presets: map[string]*model.DataPreset{"list": preset},
	}

	items := []map[string]any{
		{
			"created_at": time.Date(2002, 12, 10, 12, 12, 58, 0, time.UTC),
			"birth_date": time.Date(1990, 3, 5, 0, 0, 0, 0, time.UTC),
			"start_time": time.Date(0000, 1, 1, 9, 30, 15, 0, time.UTC),
		},
	}

	applyLocalization(m, preset, items)

	if got := items[0]["created_at"]; got != "10.12.2002 12:12:58" {
		t.Fatalf("created_at got %v", got)
	}
	if got := items[0]["birth_date"]; got != "05.03.1990" {
		t.Fatalf("birth_date got %v", got)
	}
	if got := items[0]["start_time"]; got != "09:30:15" {
		t.Fatalf("start_time got %v", got)
	}
}

func TestApplyLocalization_UsesAliasAndSourceKeys(t *testing.T) {
	origDict := model.ActiveDict
	origRegistry := model.Registry
	t.Cleanup(func() {
		model.ActiveDict = origDict
		model.Registry = origRegistry
	})

	model.ActiveDict = map[any]*model.LocaleNode{
		"Audit": {
			Children: map[any]*model.LocaleNode{
				"card": {
					Children: map[any]*model.LocaleNode{
						"auditable_type": {
							Children: map[any]*model.LocaleNode{
								"Person": {Value: "ФЛ"},
							},
						},
					},
				},
			},
		},
	}

	preset := &model.DataPreset{
		Name: "card",
		Fields: []model.Field{
			{Type: "string", Source: "auditable_type", Alias: "where", Localize: true},
		},
	}
	m := &model.Model{
		Presets: map[string]*model.DataPreset{"card": preset},
	}
	model.Registry = map[string]*model.Model{"Audit": m}

	items := []map[string]any{
		{"where": "Person"},
	}

	applyLocalization(m, preset, items)

	if got := items[0]["where"]; got != "ФЛ" {
		t.Fatalf("alias lookup failed, got %v", got)
	}
}
