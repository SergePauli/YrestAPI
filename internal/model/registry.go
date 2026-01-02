package model

import (
	"fmt"
	"log"
	"runtime"
)

var Registry = map[string]*Model{}

func InitRegistry(dir string) error {
	runtime.GC() // baseline before loading
	before := readAllocBytes()

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

	after := readAllocBytes() // heap right after load (without GC)
	regUsage := int64(after) - int64(before)
	totalPresets, totalComputable := countPresetsAndComputable()
	limitBytes, limitSrc := detectMemoryLimit()
	log.Printf("📦 Registry initialized: models=%d, presets=%d, computable=%d, heap now≈%s, delta≈%s, limit≈%s (source: %s)",
		len(Registry), totalPresets, totalComputable, formatBytes(after), formatBytes(uint64(max64(regUsage, 0))), formatBytes(limitBytes), limitSrc)

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
	if p == nil {
		log.Println("p==nil in GetPresetName")
	}
	for name, preset := range m.Presets {
		if preset == p {
			return name
		}
	}
	return ""
}
