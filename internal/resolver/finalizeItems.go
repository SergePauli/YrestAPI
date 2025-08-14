package resolver

import (
	"YrestAPI/internal/model"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// finalizeItems:
// 1) применяет formatter-поля к items
// 2) удаляет все поля/поддеревья, помеченные internal: true
func finalizeItems(m *model.Model, p *model.DataPreset, items []map[string]any) error {
	if p == nil || len(items) == 0 {
		return nil
	}

	// 1) посчитать все formatter'ы до удаления internal
	if err := applyAllFormatters(m, p, items, ""); err != nil {
		return err
	}
	// 1.5 удаляем префиксы alias'ов у preset-полей belongs_to
	stripPresetPrefixes(m, p, items, "")
	// 2) собрать маркеры internal: префиксы-деревья и точные ключи
	var (
		prefixes []string // удалить всё, что key == prefix или начинается с "prefix."
		exacts   []string // удалить ровно этот ключ
	)
	collectInternalMarkers(m, p, "", &prefixes, &exacts)

	// 3) удалить
	if len(prefixes) == 0 && len(exacts) == 0 {
		return nil
	}
	for i := range items {
		// точные ключи
		for _, k := range exacts {
			delete(items[i], k)
		}
		// префиксные удаления
		if len(prefixes) > 0 {
			for k := range items[i] {
				for _, pref := range prefixes {
					if k == pref || strings.HasPrefix(k, pref+".") {
						delete(items[i], k)
						break
					}
				}
			}
		}
	}
	return nil
}

// рекурсивно обходит пресет и применяет formatter-поля
// prefix — путь, формируемый как в ScanColumns: через relKey, а не alias поля
func applyAllFormatters(m *model.Model, p *model.DataPreset, items []map[string]any, prefix string) error {
	for _, f := range p.Fields {
		switch f.Type {
		case "preset":
			relKey := f.Source
			rel, ok := m.Relations[relKey]
			nestedModel := rel.GetModelRef()
			if !ok || rel == nil || rel.Type != "belongs_to" || nestedModel == nil {
				// внутрь уходим только по belongs_to
				continue
			}
			// nested preset
			var nested *model.DataPreset
			if f.NestedPreset != "" {
				nested = nestedModel.Presets[f.NestedPreset]
			}
			if nested == nil {
				continue
			}
			nextPrefix := relKey
			if prefix != "" {
				nextPrefix = prefix + "." + relKey
			}
			if err := applyAllFormatters( nestedModel, nested, items, nextPrefix); err != nil {
				return err
			}

		case "formatter":
			// template находится в f.Source (как в твоём примере)
			template := f.Source
			target := f.Alias
			if target == "" {
				// разумный дефолт, если alias не задан
				target = "value"
			}
			for i := range items {
				items[i][target] = applyFormatter(template, items[i])
			}

		default:
			// поддержим старый стиль, когда formatter был отдельным полем
			if strings.TrimSpace(f.Formatter) != "" {
				target := f.Alias
				if target == "" {
					// если alias не задан, пишем в имя поля
					if prefix == "" {
						target = f.Source
					} else {
						target = prefix + "." + f.Source
					}
				}
				for i := range items {
					items[i][target] = applyFormatter(f.Formatter, items[i])
				}
			}
		}
	}
	return nil
}

// собирает списки internal-маркеров:
// - prefixes: для preset/internal — удалить всё поддерево по пути "<prefix>.<relKey>"
// - exacts:   для простых полей/internal — удалить ровно "<prefix>.<field>"
func collectInternalMarkers(m *model.Model, p *model.DataPreset, prefix string, prefixes *[]string, exacts *[]string) {
	for _, f := range p.Fields {
		if f.Type == "preset" {
			relKey := f.Source
			rel, ok := m.Relations[relKey]
			nestedModel := rel.GetModelRef()
			if !ok || rel == nil || nestedModel == nil {
				continue
			}
			curPath := relKey
			if prefix != "" {
				curPath = prefix + "." + relKey
			}

			// если preset помечен internal — удаляем всё поддерево
			if f.Internal {
				*prefixes = append(*prefixes, curPath)
				// тем не менее, продолжим обход, если внутри есть ещё internal (на всякий случай)
			}

			// рекурсивно в nested только если belongs_to (равно как и в ScanColumns)
			if rel.Type == "belongs_to" {
				var nested *model.DataPreset
				if f.NestedPreset != "" {
					nested = nestedModel.Presets[f.NestedPreset]
				}
				if nested != nil {
					collectInternalMarkers(nestedModel, nested, curPath, prefixes, exacts)
				}
			}
			continue
		}

		// простой internal-поле — точечное удаление
		if f.Internal {
			// ключ строим по тем же правилам dotted-путей, что в ScanFlatRows
			var key string
			if prefix == "" {
				key = f.Source
			} else {
				key = prefix + "." + f.Source
			}
			*exacts = append(*exacts, key)
		}
	}
}

// applyFormatter уже был ранее, оставляю как есть
// шаблон: "{naming.surname} {naming.name}[0] {naming.patrname}[0..1]"
var reToken = regexp.MustCompile(`\{([a-zA-Z0-9_\.]+)\}(?:\[(\d+)(?:\.\.(\d+))?\])?`)

func applyFormatter(fmtStr string, row map[string]any) string {
    return reToken.ReplaceAllStringFunc(fmtStr, func(tok string) string {
        m := reToken.FindStringSubmatch(tok)
        if len(m) == 0 {
            return ""
        }
        path := m[1]
        iStr := m[2]
        jStr := m[3]

        // 1) Пытаемся достать как есть (прямой ключ)
        val, ok := row[path]
        if !ok {
            // 2) Если нет — идём по точкам
            val = getNested(row, path)
        }
        if val == nil {
            return ""
        }

        s := fmt.Sprintf("%v", val)

        if iStr == "" {
            return s
        }
        i, _ := strconv.Atoi(iStr)
        if jStr == "" {
            runes := []rune(s)
            if i >= 0 && i < len(runes) {
                return string(runes[i])
            }
            return ""
        }
        j, _ := strconv.Atoi(jStr)
        runes := []rune(s)
        if i < 0 {
            i = 0
        }
        if j > len(runes) {
            j = len(runes)
        }
        if i >= j {
            return ""
        }
        return string(runes[i:j])
    })
}

func getNested(m map[string]any, path string) any {
    parts := strings.Split(path, ".")
    var cur any = m
    for _, p := range parts {
        if mm, ok := cur.(map[string]any); ok {
            cur = mm[p]
        } else {
            return nil
        }
    }
    return cur
}
func stripPresetPrefixes(m *model.Model, p *model.DataPreset, items []map[string]any, prefix string) {
    for _, f := range p.Fields {
        if f.Type != "preset" {
            continue
        }

        relKey := f.Source
        rel, ok := m.Relations[relKey]
        if !ok || rel == nil {
            continue
        }

        // Обрабатываем только belongs_to
        if rel.Type != "belongs_to" {
            continue
        }

        // Префикс в flat-ключах
        curPrefix := relKey
        if prefix != "" {
            curPrefix = prefix + "." + relKey
        }

        for _, row := range items {
            sub := make(map[string]any)
            for k, v := range row {
                if strings.HasPrefix(k, curPrefix+".") {
                    subKey := strings.TrimPrefix(k, curPrefix+".")
                    sub[subKey] = v
                    delete(row, k)
                }
            }
            if len(sub) > 0 {
                row[f.Alias] = sub
            }
        }

        // Рекурсивно спускаемся внутрь
        nestedModel := rel.GetModelRef()
        if nestedModel != nil && f.NestedPreset != "" {
            if nested := nestedModel.Presets[f.NestedPreset]; nested != nil {
                stripPresetPrefixes(nestedModel, nested, items, curPrefix)
            }
        }
    }
}