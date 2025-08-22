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
		for i := range items {
			if v, ok := items[i][f.Alias]; ok {
				strVal := fmt.Sprintf("%v", v)
				// ищем в словаре начиная с глубины: model → preset → field
				if translated, ok := model.ActiveDict[modelName].Lookup(presetName, f.Alias, strVal); ok {
					items[i][f.Alias] = translated
					continue
				}
				// пробуем глобальный пресет
				if translated, ok := model.ActiveDict[presetName].Lookup( f.Alias, strVal); ok {
					items[i][f.Alias] = translated
					continue
				}
				// пробуем глобальное поле
				if translated, ok := model.ActiveDict[f.Alias].Lookup( strVal); ok {
					items[i][f.Alias] = translated
					continue
				}
				
			}
		}
	}
}