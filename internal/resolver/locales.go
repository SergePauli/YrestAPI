package resolver

import (
	"YrestAPI/internal/model"
	"log"
	"math"
)
func toIntKey(v any) (int, bool) {	
	switch x := v.(type) {
	case int:
		return x, true

	case int16:
		return int(x), true

	case int32:
		return int(x), true

	case int64:
		if x < math.MinInt || x > math.MaxInt {
			return 0, false
		}
		return int(x), true

	case uint:
		if x > uint(math.MaxInt) {
			return 0, false
		}
		return int(x), true

	case uint16:
		return int(x), true

	case uint32:
		if uint(x) > uint(math.MaxInt) {
			return 0, false
		}
		return int(x), true

	case uint64:
		if x > uint64(math.MaxInt) {
			return 0, false
		}
		return int(x), true
	}

	return 0, false
}
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

			// ищем в словаре начиная с глубины: model → preset → field
			if translated, ok := model.ActiveDict[modelName].Lookup(presetName, key, v); ok {
				items[i][key] = translated
				continue
			}
			// пробуем глобальный пресет
			if translated, ok := model.ActiveDict[presetName].Lookup(key, v); ok {
				items[i][key] = translated
				continue
			}
			// пробуем глобальное поле	
			if f.Type == "int" {
				if k, ok := toIntKey(v); ok {
					log.Printf("applyLocalization: looking up int key %d in field %s\n", k, key)
					if translated, ok := model.ActiveDict[key].Lookup(k); ok {
						items[i][key] = translated
						continue
					}
				}
			}	else if translated, ok := model.ActiveDict[key].Lookup(v); ok {
				items[i][key] = translated
				continue
			}

			// если перевода нет — оставляем как есть
			items[i][key] = v
		}
	}
}
