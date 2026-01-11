package model

import "strings"

// ScanPresetFields возвращает список полей пресета для belongs_to связей
func (m *Model) ScanPresetFields(preset *DataPreset, prefix string) []string {

	out := make([]string, 0)
	seen := make(map[string]bool)

	var appendOnce = func(path string) {
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}

	for _, f := range preset.Fields {
		// только поля типа "preset" рассматриваем как ссылку на связь
		if f.Type != "preset" {
			// но вытащим пути из плейсхолдеров nested_field/formatter, чтобы aliasMap знал про связи
			if f.Type == "nested_field" || f.Type == "formatter" {
				for _, p := range extractPathsFromExpr(f.Source) {
					full := p
					if prefix != "" {
						trimmed := strings.TrimSuffix(prefix, ".")
						if trimmed != "" {
							full = trimmed + "." + p
						}
					}
					if idx := strings.LastIndex(full, "."); idx > 0 {
						appendOnce(full[:idx])
					}
				}
			}
			continue
		}

		relKey := f.Source // в пресете source для nested preset — это ключ связи в текущей модели
		if relKey == "" {
			continue
		}

		rel, ok := m.Relations[relKey]
		if !ok {
			// связь не описана в модели — пропускаем
			continue
		}

		// нас интересуют только belongs_to (в соответствии с твоим требованием)
		if rel.Type != "belongs_to" {
			// Для других типов — отмечаем флаг elsewhere (DetectJoins сам поймёт has_* по полям фильтров)
			continue
		}

		// сформируем путь
		var path string
		if prefix == "" {
			path = relKey + "."
		} else {
			// если prefix уже оканчивается на точку – не добавляем ещё одну
			if strings.HasSuffix(prefix, ".") {
				path = prefix + relKey + "."
			} else {
				path = prefix + "." + relKey
			}
		}
		appendOnce(path)

		// Если указан nested_preset (Preset внутри связанной модели), рекурсивно обходим его
		if f.NestedPreset != "" {
			// nestedPreset может быть "ModelName.preset" или просто "preset"
			nested := f.NestedPreset
			parts := strings.SplitN(nested, ".", 2)
			var presetName string
			if len(parts) == 2 {
				// parts[0] — имя модели (можно игнорировать, т.к. у нас есть rel._ModelRef),
				// parts[1] — имя пресета внутри связанной модели
				presetName = parts[1]
			} else {
				presetName = nested
			}

			relatedModel := rel._ModelRef
			if relatedModel == nil {
				// если _ModelRef не установлен — пропускаем рекурсию
				continue
			}
			nestedPreset, ok := relatedModel.Presets[presetName]
			if !ok || nestedPreset == nil {
				continue
			}

			// рекурсивный вызов: префикс расширяем текущим relKey
			subPaths := relatedModel.ScanPresetFields(nestedPreset, path)
			for _, p := range subPaths {
				appendOnce(p)
			}
		}
	}

	return out
}
