package model

import (
	"fmt"
	"log"
	"strings"
)

// ValidateAllPresets выполняет полную проверку всех пресетов: циклы, корректность ссылок и вложенных пресетов.
func ValidateAllPresets() error {
	for modelName, model := range Registry {
		for presetName := range model.Presets {
			if hasPresetCycle(modelName, presetName, map[string]bool{}, nil) {
				return fmt.Errorf("cycle detected in preset: %s.%s", modelName, presetName)
			}
		}
	}
	return nil
}

// hasPresetCycle проверяет наличие циклических вложенных пресетов, следуя за NestedPreset.
func hasPresetCycle(modelName, presetName string, visited map[string]bool, path []string) bool {
	fullName := modelName + "." + presetName
	if visited[fullName] {
		log.Printf("⛔ Cycle detected: %s", strings.Join(append(path, fullName), " → "))
		return true
	}
	visited[fullName] = true

	model, ok := Registry[modelName]
	if !ok {
		log.Printf("⚠️ Model not found: %s", modelName)
		return false
	}

	preset, ok := model.Presets[presetName]
	if !ok {
		log.Printf("⚠️ Preset not found: %s.%s", modelName, presetName)
		return false
	}

	for _, f := range preset.Fields {
		if f.NestedPreset != "" {
			nestedPreset := model.Relations[f.Source]._ModelRef.Presets[f.NestedPreset]
			nestedModel:= model.Relations[f.Source].Model
			if nestedPreset == nil {
				log.Printf("⚠️ Invalid nested preset name: %s, for %s", f.NestedPreset, nestedModel)
				continue
			}
			
			if hasPresetCycle(nestedModel, f.NestedPreset, copyMap(visited), append(path, fullName)) {
				return true
			}
		}
	}
	return false
}

// copyMap делает поверхностную копию карты visited для независимого обхода.
func copyMap(m map[string]bool) map[string]bool {
	cp := make(map[string]bool, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
