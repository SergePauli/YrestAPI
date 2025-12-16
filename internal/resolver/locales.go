package resolver

import (
	"YrestAPI/internal/model"
	"fmt"
)

// applyLocalization проходит по полям пресета и заменяет значения через словарь
func applyLocalization(m *model.Model, p *model.DataPreset, items []map[string]any) {
	if model.ActiveDict == nil || len(items) == 0 || p == nil {
		return
	}
	
	modelName := model.GetModelName(m)
	presetName := p.Name

	
	for _, f := range p.Fields {
		if !f.Localize {
			continue
		}
		key := f.Alias
		if key == "" {
			key = f.Source
		}
		srcKey := f.Source

		for i := range items {
			v, ok := items[i][key]
			if !ok && key != srcKey {
				v, ok = items[i][srcKey]
			}
			if !ok {
				continue
			}

			strVal := fmt.Sprintf("%v", v)
			// ищем в словаре начиная с глубины: model → preset → field
			if translated, ok := model.ActiveDict[modelName].Lookup(presetName, key, strVal); ok {
				items[i][key] = translated
				continue
			}
			// пробуем глобальный пресет
			if translated, ok := model.ActiveDict[presetName].Lookup(key, strVal); ok {
				items[i][key] = translated
				continue
			}
			// пробуем глобальное поле
			if translated, ok := model.ActiveDict[key].Lookup(strVal); ok {
				items[i][key] = translated
				continue
			}

			// если перевода нет — хотя бы продублируем исходное значение в алиас
			items[i][key] = strVal
		}
	}
}
