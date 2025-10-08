package model

import (
	"fmt"

	"strings"
)

// ValidateAllPresets выполняет полную проверку всех пресетов:
// 1) корректность ссылок и типов,
// 2) допустимые (или недопустимые) циклы согласно политикам Reentrant/MaxDepth.
func ValidateAllPresets() error {
	for modelName, model := range Registry {
		for presetName := range model.Presets {
			if err := validatePresetGraph(modelName, presetName); err != nil {
				return err
			}
		}
	}
	return nil
}

// validatePresetGraph запускает DFS-обход графа NestedPreset'ов с политиками циклов.
// path хранит цепочку узлов "Model" для сообщений об ошибках и подсчёта повторов.
func validatePresetGraph(modelName, presetName string) error {
	node := modelName + "." + presetName
	// pathNodes — ради читаемых сообщений
	pathNodes := []string{node}
	// pathModels — именно по ним считаем ре-энтри в модель
	pathModels := []string{modelName}
	// сколько раз модель встречалась на текущем пути
	modelCounts := map[string]int{modelName: 1}
	return dfsPresetWithPolicy(modelName, presetName, pathNodes, pathModels, modelCounts)

}

// dfsPresetWithPolicy — основной обход.
// Разрешает возвращаться к уже встреченным узлам ТОЛЬКО если связь reentrant
// и не превышен эффективный maxDepth (берётся из поля или связи).
func dfsPresetWithPolicy(modelName, presetName string, pathNodes, pathModels []string, modelCounts map[string]int) error {
	model, ok := Registry[modelName]
	if !ok {
		return fmt.Errorf("model not found: %s", modelName)
	}
	preset, ok := model.Presets[presetName]
	if !ok {
		return fmt.Errorf("preset not found: %s.%s", modelName, presetName)
	}

	for _, f := range preset.Fields {
		if f.NestedPreset == "" {
			continue
		}

		// 1) поле с NestedPreset должно быть type=preset
		if strings.ToLower(strings.TrimSpace(f.Type)) != "preset" {
			return fmt.Errorf(
				"field %q in %s.%s uses nested preset %q but Type=%q (expected Type=\"preset\")",
				fieldNameForMsg(f), modelName, presetName, f.NestedPreset, f.Type,
			)
		}

		// 2) source должен быть валидной связью has_one/has_many
		rel, ok := model.Relations[f.Source]
		if !ok {
			return fmt.Errorf(
				"field %q in %s.%s refers to unknown relation %q",
				fieldNameForMsg(f), modelName, presetName, f.Source,
			)
		}
		switch rel.Type {
			case "has_one", "has_many", "belongs_to":
    		// ok
			default:
    		return fmt.Errorf(
        "field %q in %s.%s refers to relation %q of unsupported type %q (allowed: has_one, has_many, belongs_to)",
        fieldNameForMsg(f), modelName, presetName, f.Source, rel.Type,
				)
	}

		// 3) определить целевую модель/пресет
		nestedModelName := rel.Model
		nestedModel, ok := Registry[nestedModelName]
		if !ok {
			return fmt.Errorf(
				"relation %q in %s points to unknown model %q",
				f.Source, modelName, nestedModelName,
			)
		}
		nestedPresetName := f.NestedPreset
		if _, ok := nestedModel.Presets[nestedPresetName]; !ok {
			return fmt.Errorf(
				"invalid nested preset %q for model %s (referenced from %s.%s field %q)",
				nestedPresetName, nestedModelName, modelName, presetName, fieldNameForMsg(f),
			)
		}

		// 4) проверка ре-энтри по МОДЕЛИ (а не по узлу Model.Preset)
		seen := modelCounts[nestedModelName] // 0 если не встречалась
		if seen > 0 {
			// повторный вход в уже встретившуюся модель
			if !rel.Reentrant {
				return fmt.Errorf(
					"cycle detected: re-entry into model %s via relation %q is not reentrant (set reentrant: true or break the cycle). Path: %s → %s.%s",
					nestedModelName, f.Source, strings.Join(pathNodes, " → "), nestedModelName, nestedPresetName,
				)
			}
			// бюджет глубины: поле имеет приоритет, затем связь; по умолчанию разрешаем один повтор
			effMax := effectiveMaxDepth(f.MaxDepth, rel.MaxDepth)
			if effMax <= 0 {
				effMax = 1
			}
			if seen >= effMax {
				return fmt.Errorf(
					"cycle would exceed max_depth for model %s (eff.max_depth=%d). Path: %s → %s.%s",
					nestedModelName, effMax, strings.Join(pathNodes, " → "), nestedModelName, nestedPresetName,
				)
			}
		}

		// 5) спускаемся глубже
		nextNode := nestedModelName + "." + nestedPresetName

		newPathNodes := append(pathNodes[:], nextNode)
		newPathModels := append(pathModels[:], nestedModelName)
		newCounts := cloneCounts(modelCounts)
		newCounts[nestedModelName] = seen + 1

		if err := dfsPresetWithPolicy(nestedModelName, nestedPresetName, newPathNodes, newPathModels, newCounts); err != nil {
			return err
		}
	}

	return nil
}

// --- helpers ---

func fieldNameForMsg(f Field) string {
	if f.Alias != "" {
		return f.Alias
	}
	if f.Source != "" {
		return f.Source
	}
	return "<unnamed>"
}


func cloneCounts(m map[string]int) map[string]int {
	cp := make(map[string]int, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func effectiveMaxDepth(fieldMax, relMax int) int {
	switch {
	case fieldMax > 0:
		return fieldMax
	case relMax > 0:
		return relMax
	default:
		return 0
	}
}