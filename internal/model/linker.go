package model

import (
	"fmt"
	"regexp"
	"strings"

	"YrestAPI/internal/logger"
	"unicode"
)

var formatterSrcRe = regexp.MustCompile(`\{[^}]+\}`)

func LinkModelRelations() error {
	for modelName, model := range Registry {
		// 1. Link & validate relations
		for relName, rel := range model.Relations {
			if rel.Polymorphic {
				if rel.Type != "belongs_to" {
					return fmt.Errorf("invalid relation: polymorphic allowed only for belongs_to (%s.%s)", modelName, relName)
				}
				if rel.TypeColumn == "" {
					rel.TypeColumn = relName + "_type"
				}
				// FK/PK fallbacks below
			} else {
				// Модель должна существовать
				targetModel, ok := Registry[rel.Model]
				if !ok {
					return fmt.Errorf("invalid relation: model '%s' not found in '%s.%s'",
						rel.Model, modelName, relName)
				}
				rel._ModelRef = targetModel
			}

			// FK по умолчанию
			if rel.FK == "" {
				switch rel.Type {
				case "belongs_to":
					// FK в текущей модели, указывает на связанную
					rel.FK = relName + "_id"
				case "has_one", "has_many":
					// FK в связанной модели, указывает на текущую
					rel.FK = toSnakeCase(modelName) + "_id"
				}
			}

			// PK по умолчанию
			if rel.PK == "" {
				rel.PK = "id"
			}

			// Проверка through
			if rel.Through != "" {
				if rel.Polymorphic {
					return fmt.Errorf("polymorphic relation '%s.%s' cannot use through", modelName, relName)
				}
				throughModel, ok := Registry[rel.Through]
				if !ok {
					return fmt.Errorf("invalid through: model '%s' not found in '%s.%s'",
						rel.Through, modelName, relName)
				}
				rel._ThroughRef = throughModel

				// Проверяем, что из throughModel есть связь к конечной модели
				var found bool
				for _, throughRel := range throughModel.Relations {
					if throughRel.Model == rel.Model {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("invalid through: no relation from '%s' to '%s' found in '%s.%s'",
						rel.Through, rel.Model, modelName, relName)
				}
			}

			// Проверка типа связи
			if rel.Type != "has_many" && rel.Type != "has_one" && rel.Type != "belongs_to" {
				return fmt.Errorf("relation '%s.%s' has invalid type '%s' (must be has_many, has_one, belongs_to)",
					modelName, relName, rel.Type)
			}
			if rel.Type == "belongs_to" && rel.Polymorphic && rel.PK == "" {
				rel.PK = "id"
			}

			model.Relations[relName] = rel
		}

		// 2. Link & validate presets
		for presetName, preset := range model.Presets {
			// Если нет fields — создаём пустой слайс
			if preset.Fields == nil {
				preset.Fields = []Field{}
				logger.Warn("preset_no_fields", map[string]any{
					"preset": presetName,
					"model":  modelName,
				})
			}
			preset.Name = presetName
			// Проверяем каждое поле
			for fi := range preset.Fields {
				f := &preset.Fields[fi]

				// 2.0) Сначала: если Source похож на форматтер, а тип не "formatter"/"nested_field" — ошибка.
				isFormatterSrc := formatterSrcRe.MatchString(f.Source)
				if isFormatterSrc && f.Type != "formatter" && f.Type != "nested_field" {
					return fmt.Errorf(
						"field '%s' in preset '%s' of model '%s' uses template-like source '%s' but its type is '%s'; expected type 'formatter'",
						f.Alias, presetName, modelName, f.Source, f.Type,
					)
				} else if isFormatterSrc && f.Type == "formatter" {
					// 2.1) Если поле — formatter → алиас обязателен
					if strings.TrimSpace(f.Alias) == "" {
						return fmt.Errorf(
							"formatter field with source '%s' in preset '%s' of model '%s' must have explicit alias",
							f.Source, presetName, modelName,
						)
					}
				}
				if f.Type == "computable" {
					if model.Computable == nil {
						return fmt.Errorf("computable '%s' referenced in preset '%s' of model '%s' but model.computable is empty", f.Source, presetName, modelName)
					}
					if _, ok := model.Computable[f.Source]; !ok {
						return fmt.Errorf("computable '%s' referenced in preset '%s' of model '%s' not found in model.computable", f.Source, presetName, modelName)
					}
				}
				if f.Alias == "" {
					f.Alias = f.Source
				}
				fieldName := f.Alias
				// Проверка preset-полей
				if f.Type == "preset" {
					// 2.1 Должно быть указано nested_preset
					if f.NestedPreset == "" {
						return fmt.Errorf("field '%s' in preset '%s' of model '%s' has type 'preset' but no nested_preset is defined",
							fieldName, presetName, modelName)
					}

					// 2.2 Должна быть валидная relation
					rel, ok := model.Relations[f.Source]
					if !ok {
						return fmt.Errorf("field '%s' in preset '%s' refers to missing relation '%s' in model '%s'",
							fieldName, presetName, f.Source, modelName)
					}

					// 2.3 У relation должен быть целевой modelRef
					nestedModel := rel._ModelRef
					if rel.Polymorphic {
						// пропускаем проверку наличия preset в конкретной модели — определяется на этапе резолва
						f._PresetRef = nil
					} else {
						if nestedModel == nil {
							return fmt.Errorf("field '%s' in preset '%s' refers to relation '%s' with nil model in '%s'",
								fieldName, presetName, f.Source, modelName)
						}
						// 2.4 nested_preset должен существовать в целевой модели
						nestedPreset := nestedModel.Presets[f.NestedPreset]
						if nestedPreset == nil {
							return fmt.Errorf("nested preset '%s' not found in model '%s' (referenced from '%s.%s')",
								f.NestedPreset, nestedModel.Table, modelName, presetName)
						}

						// Линкуем
						f._PresetRef = nestedPreset
					}
				}
			}
		}
	}
	return nil
}

func FindPresetByName(model *Model, name string) *DataPreset {

	preset, ok := model.Presets[name]
	if !ok {
		logger.Warn("preset_not_found", map[string]any{
			"preset": name,
			"model":  model.Table,
		})
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
