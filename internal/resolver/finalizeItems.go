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

	// 1.1 удаляем префиксы alias'ов у preset-полей belongs_to
	stripPresetPrefixes(m, p, items, "")
	// 1.2 посчитать все formatter'ы до удаления internal
	if err := applyAllFormatters(m, p, items, ""); err != nil {
		return err
	}
	if model.HasLocales {
		applyLocalization(m, p, items) // или locale из запроса
	}
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

			// Рекурсивно обрабатываем вложенные форматтеры
			if err := applyAllFormatters(nestedModel, nested, items, prefixFor(prefix, relKey)); err != nil {
				return err
			}

			// Применяем formatter к belongs_to полю
			if strings.TrimSpace(f.Formatter) != "" {
				for i := range items {
					if sub, ok := items[i][f.Alias].(map[string]any); ok {
						items[i][f.Alias] = applyFormatter(f.Formatter, sub)
					} else {
						items[i][f.Alias] = ""
					}
				}
			}

		case "formatter":
			// template находится в f.Source
			template := f.Source
			target := f.Alias
			if target == "" {
				target = "value"
			}
			for i := range items {
				items[i][target] = applyFormatter(template, items[i])
			}

		default:
			// Старый стиль formatter как отдельное поле
			if strings.TrimSpace(f.Formatter) != "" {
				target := f.Alias
				if target == "" {
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

func prefixFor(base, relKey string) string {
	if base == "" {
		return relKey
	}
	return base + "." + relKey
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


var reToken = regexp.MustCompile(`\{([a-zA-Z0-9_\.]+)\}(?:\[(\d+)(?:\.\.(\d+))?\])?`)
// {? left op right ? then : else}
var reTernary = regexp.MustCompile(`\{\?\s*([^?]+?)\s*\?\s*([^:]*?)\s*:\s*([^}]*)\}`)

// applyFormatter применяет тернарники, затем обычные токены
func applyFormatter(fmtStr string, row map[string]any) string {
    // 1) Сначала — тернарные выражения. Делаем несколько проходов, пока что-то меняется (на случай каскадов).
    prev := ""
    out := fmtStr
    for out != prev {
        prev = out
        out = reTernary.ReplaceAllStringFunc(out, func(tok string) string {
            m := reTernary.FindStringSubmatch(tok)
            if len(m) != 4 {
                return "" // не должно случиться, но на всякий
            }
            cond := strings.TrimSpace(m[1])
            thenStr := m[2]
            elseStr := m[3]

            ok, err := evalCondition(cond, row)
            if err != nil {
                // В случае ошибки парсинга условия — считаем условие ложным (или можно вернуть пусто)
                // Чтобы не скрывать проблемы, можно логнуть err
                return unquoteIfQuoted(elseStr)
            }
            if ok {
                return unquoteIfQuoted(thenStr)
            }
            return unquoteIfQuoted(elseStr)
        })
    }
		
		// 2) Затем — обычные токены с индексами/срезами
    return reToken.ReplaceAllStringFunc(out, func(tok string) string {
        m := reToken.FindStringSubmatch(tok)
        if len(m) == 0 {
            return ""
        }
        path := m[1]
        iStr := m[2]
        jStr := m[3]

        // Прямой ключ → иначе по точкам
        val, ok := row[path]
        if !ok {
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
// снимет только внешние одинаковые кавычки '...' или "..."
		func unquoteIfQuoted(s string) string {
    	s = strings.TrimSpace(s)
    	if len(s) >= 2 {
        if (s[0] == '"'  && s[len(s)-1] == '"') ||
           (s[0] == '\'' && s[len(s)-1] == '\'') {
            return s[1 : len(s)-1]
        }
    }
    return s
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

func evalCondition(cond string, row map[string]any) (bool, error) {
    cond = strings.TrimSpace(cond)
    if cond == "" {
        return false, fmt.Errorf("empty condition")
    }

    // 0) если cond не содержит операторов, значит это просто поле/путь
    opRe := regexp.MustCompile(`\s*(==|=|!=|>=|<=|>|<)\s*`)
    if !opRe.MatchString(cond) {
        val, ok := row[cond]
        if !ok {
            val = getNested(row, cond)
        }
        return isTruthy(val), nil
    }

    // 1) обычный парсинг "<left> <op> <right>"
    parts := opRe.Split(cond, 2)
    ops := opRe.FindStringSubmatch(cond)
    if len(parts) != 2 || len(ops) == 0 {
        return false, fmt.Errorf("invalid condition: %q", cond)
    }
    left := strings.TrimSpace(parts[0])
    right := strings.TrimSpace(parts[1])
    op := ops[1]
    if op == "=" {
        op = "=="
    }

    lv, ok := row[left]
    if !ok {
        lv = getNested(row, left)
    }
    rv, err := parseLiteral(right)
    if err != nil {
        return false, err
    }

    return compareValues(lv, op, rv), nil
}

func isTruthy(v any) bool {
    switch x := v.(type) {
    case nil:
        return false
    case bool:
        return x
    case string:
        return x != ""
    case int, int32, int64, float32, float64:
        n, ok := toNumber(x)
        return ok && n != 0
    default:
        return true // любое другое значение считаем truthy
    }
}



func parseLiteral(s string) (any, error) {
    s = strings.TrimSpace(s)
    if s == "null" {
        return nil, nil
    }
    if s == "true" {
        return true, nil
    }
    if s == "false" {
        return false, nil
    }
    // строка в кавычках
    if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) || (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
        unq := s[1 : len(s)-1]
        return unq, nil
    }
    // попробовать как число
    if i, err := strconv.ParseInt(s, 10, 64); err == nil {
        return float64(i), nil
    }
    if f, err := strconv.ParseFloat(s, 64); err == nil {
        return f, nil
    }
    // голое слово — трактуем как строку
    return s, nil
}

func toNumber(v any) (float64, bool) {
    switch x := v.(type) {
    case int: return float64(x), true
    case int32: return float64(x), true
    case int64: return float64(x), true
    case float32: return float64(x), true
    case float64: return x, true
    case string:
        if f, err := strconv.ParseFloat(x, 64); err == nil {
            return f, true
        }
        return 0, false
    default:
        return 0, false
    }
}

func toString(v any) string {
    switch x := v.(type) {
    case nil:
        return ""
    case string:
        return x
    default:
        return fmt.Sprintf("%v", x)
    }
}

func compareValues(lv any, op string, rv any) bool {
    // если оба приводимы к числу — числовое сравнение
    if ln, lok := toNumber(lv); lok {
        if rn, rok := toNumber(rv); rok {
            switch op {
            case "==": return ln == rn
            case "!=": return ln != rn
            case ">":  return ln > rn
            case ">=": return ln >= rn
            case "<":  return ln < rn
            case "<=": return ln <= rn
            }
            return false
        }
    }

    // булево
    if lb, lok := lv.(bool); lok {
        if rb, rok := rv.(bool); rok {
            switch op {
            case "==": return lb == rb
            case "!=": return lb != rb
            default:   return false
            }
        }
    }

    // строковое сравнение (лексикографическое для >,<)
    ls := toString(lv)
    rs := toString(rv)
    switch op {
    case "==": return ls == rs
    case "!=": return ls != rs
    case ">":  return ls > rs
    case ">=": return ls >= rs
    case "<":  return ls < rs
    case "<=": return ls <= rs
    }
    return false
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

func ApplyFormatterTestShim(fmtStr string, row map[string]any) string {
	return applyFormatter(fmtStr, row)
}