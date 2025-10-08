package model

import (
	"fmt"
	"log"
)

var Registry = map[string]*Model{}


func InitRegistry(dir string) error {
	if err := LoadModelsFromDir(dir); err != nil {
		return fmt.Errorf("load error: %w", err)
	}
	if err := LinkModelRelations(); err != nil {
		return fmt.Errorf("link error: %w", err)
	}
	if err := ValidateAllPresets(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	if err := BuildPresetAliasMaps(); err != nil {
		log.Fatalf("InitRegistry failed: %v", err)
	}
	return nil
}

func (m *Model) GetPreset(name string) *DataPreset {
	if p, ok := m.Presets[name]; ok {
		return p
	}
	return nil
}

func (m *Model) GetRelation(alias string) *ModelRelation {
	if m == nil || m.Relations == nil {
		return nil
	}
	return m.Relations[alias]
}

func GetModelName(m *Model) string {
	for name, model := range Registry {
		if model == m {
			return name
		}
	}
	return ""
}

func GetPresetName(m *Model, p *DataPreset) string {
	if (p==nil) {log.Println("p==nil in GetPresetName")}
	for name, preset := range m.Presets {
		if preset == p {
			return name
		}
	}
	return ""
}