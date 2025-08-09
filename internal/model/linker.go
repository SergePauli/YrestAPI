package model

import (
	"fmt"
	"log"
	"strings"
	"unicode"
)

func LinkModelRelations() error {
	for modelName, model := range Registry {
		// Validate and link relations
		for relName, rel := range model.Relations {
			targetModel, ok := Registry[rel.Model]
			if !ok {
				return fmt.Errorf("invalid relation: model '%s' not found in '%s.%s'", rel.Model, modelName, relName)
			}
			rel._ModelRef = targetModel
			// Присваиваем FK по умолчанию, если не задан
			switch rel.Type {
				case "belongs_to":
				// FK должен быть в текущей модели, указывать на связанную
				rel.FK = relName + "_id" // Ок
				case "has_one", "has_many":
				// FK находится в связанной модели и указывает на текущую
				rel.FK = toSnakeCase(modelName) + "_id" // ← Здесь важно использовать текущую модель
			}

			// Присваиваем PK по умолчанию, если не задан
			if rel.PK == "" {
				rel.PK = "id"
			}

			// Валидация: if through указано — оно должно быть валидной моделью
			if rel.Through != "" {
				throughModel, ok := Registry[rel.Through]
    		if !ok {
        	return fmt.Errorf(
            "invalid through: model '%s' not found in '%s.%s'",
            rel.Through, modelName, relName,
        	)
    		}
    		// Ищем связь из промежуточной модели к конечной
    		var found bool
    		for _, throughRel := range throughModel.Relations {
        		if throughRel.Model == rel.Model {            
            	found = true
            	break
        		}
    		}
    		if !found {
        	return fmt.Errorf(
            "invalid through: no relation from '%s' to '%s' found in '%s.%s'",
            rel.Through, rel.Model, modelName, relName,
        	)
    		} else {
					rel._ThroughRef = throughModel
				}
			}

			// Валидация: проверяем, что есть хотя бы одна связь
			// с типом has_one, has_many или belongs_to
			if rel.Type != "has_many" && rel.Type != "has_one" && rel.Type != "belongs_to" {
					return fmt.Errorf("relation '%s.%s' must have valid Type (has_many, has_one, belongs_to), got '%s'", modelName, relName, rel.Type)
			}
			model.Relations[relName] = rel
		}

		// Link and validate nested_presets
		for i := range model.Presets {
			for j := range model.Presets[i].Fields {
				f := &model.Presets[i].Fields[j]
				if f.NestedPreset != "" {
					preset := FindPresetByName(f.NestedPreset)
					if preset == nil {
						return fmt.Errorf("nested preset '%s' not found in %s", f.NestedPreset, modelName)
					}
					f._PresetRef = preset
				}
			}
		}
	}
	return nil
}

func FindPresetByName(fullName string) *DataPreset {
	parts := strings.Split(fullName, ".")
	if len(parts) != 2 {
		log.Printf("Invalid preset name format: %s", fullName)
		return nil
	}
	modelName := parts[0]
	presetName := parts[1]

	model, ok := Registry[modelName]
	if !ok {
		log.Printf("Model %s not found for preset %s", modelName, fullName)
		return nil
	}

	preset, ok := model.Presets[presetName]
	if !ok {
		log.Printf("Preset %s not found in model %s", presetName, modelName)
		return nil
	}

	return preset
}
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, unicode.ToLower(r))
	}
	return string(result)
}