package model

import "fmt"

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