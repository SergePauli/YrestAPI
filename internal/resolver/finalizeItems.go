package resolver

import (
	"YrestAPI/internal/model"
	"fmt"
	"log"
	"maps"

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
	 // маленький локальный хелпер: достать/создать под-объект по prefix
    	getCtx := func(root map[string]any, pfx string) map[string]any {
        if pfx == "" {
            return root
        }				
        cur := root				
        for _, seg := range strings.Split(pfx, ".") {						
            seg = strings.TrimSpace(seg)
            if seg == "" {
                continue
            } 
            if v, ok := cur[seg]; ok {
                if m, ok := v.(map[string]any); ok {
                    cur = m
                    continue
                }
            }
            // создаём, если нет или тип не map
            m := map[string]any{}
            cur[seg] = m
            cur = m
        }
				
        return cur
			}
	var head *formatterNode		
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
			nested = f.GetPresetRef()
			
			if nested == nil && f.NestedPreset != "" {
				nested = nestedModel.Presets[f.NestedPreset]
			} 
			if nested == nil {
				log.Printf("applyAllFormatters: warning: preset %q not found in model %q for field %q", f.NestedPreset, relKey, f.Alias)
				continue
			}
			
			// Рекурсивно обрабатываем вложенные форматтеры
			nextPrefix := prefixFor(prefix, relKey)
			if err := applyAllFormatters(nestedModel, nested, items, nextPrefix); err != nil {
				return err
			}
			// Применяем formatter к belongs_to полю
			if strings.TrimSpace(f.Formatter) != "" {	
				pf := &f
				node := &formatterNode{
					Alias: f.Alias, // alias гарантирован валидатором
					F: pf,
				}
				head = insertByDeps(head, node)
			}
		case "formatter":
			pf := &f
			node := &formatterNode{
					Alias: f.Alias, // alias гарантирован валидатором
					F: pf,
			}
			head = insertByDeps(head, node)
		default:
			// прочие типы полей — пропускаем
		}
	}
	
	// теперь обойти узлы в порядке зависимостей
	for node := head; node != nil; node = node.Next {
		f := node.F
		switch f.Type {

		case "formatter":
			// шаблон в f.Source, писать в f.Alias на уровне prefix			
			template := f.Source
    	target   := f.Alias // alias обязателен
    	// alias -> unwrapKey для preset-полей с through (через геттеры)
    	unwrapByAlias := map[string]string{}
    	for _, pf := range p.Fields {
        rel, ok := m.Relations[pf.Source]
				if !ok || rel == nil || rel.Through == "" {						
						continue
				} else {
        	through := rel.GetThroughRef()
        	final   := rel.GetModelRef()
        	// находим связь из through к final (обычно belongs_to "contact")				
        	for k, r2 := range through.Relations {
            if  r2.GetModelRef() == final {
                unwrapByAlias[pf.Alias] = k // напр. "email" -> "contact"
                break
            }
        	}
				}	
    	}
    	for i := range items {
        // копия ВСЕГО item — чтобы {name} и прочие обычные поля были доступны
        fctx := make(map[string]any, len(items[i]))
				maps.Copy(fctx, items[i])
        if len(unwrapByAlias) > 0 {					
					 // разворачиваем through-алиасы: fctx[email] = fctx[email][contact]
					 for alias, inner := range unwrapByAlias {
            if outer, ok := fctx[alias].(map[string]any); ok {
                if innerMap, ok := outer[inner].(map[string]any); ok {
                    fctx[alias] = innerMap
                }
            }
        	}
				} else {
					 // без through — просто берем контекст текущего пресета				 
					 fctx = getCtx(fctx, prefix)					 
				}	 				
        // считаем шаблон по fctx и записываем прямо в alias этого formatter-поля
        items[i][target] = applyFormatter(template, fctx)
			}	
			case "preset":
				// форматтер на контейнер (belongs_to / through)
				relKey := f.Source
				rel, ok:= m.Relations[relKey]			
				alias := f.Alias
				// определить unwrapKey для through (например "contact")
    		unwrapKey := ""
    		if ok && rel.Through != "" {
        	for k, r2 := range rel.GetThroughRef().Relations {
            if r2 != nil && r2.Type == "belongs_to" && r2.GetModelRef() == rel.GetModelRef() {
                unwrapKey = k
                break
            }
        	}
    		}			  		
				for i := range items {
					if unwrapKey != "" {
						ctx := getCtx(items[i], prefix)
            // through: форматируем по конечной модели (alias -> unwrapKey)
            if sub, ok := ctx[alias].(map[string]any); ok {
                if inner, ok := sub[unwrapKey].(map[string]any); ok {
                    ctx[alias] = applyFormatter(f.Formatter, inner)
                } 
            } else {
                ctx[alias] = ""
            }
        	} else {
            // обычный случай: форматируем по текущему контексту ветки
						nextPrefix := prefixFor(prefix, f.Source)
        		ctx := getCtx(items[i], nextPrefix) // контекст текущей ветки пресета
            items[i][alias] = applyFormatter(f.Formatter, ctx)						
        	}
				}	
		}		
	}	
	return nil
	
}

func prefixFor(base string, relKey string) string {
	if base == "" {
		return relKey
	}
	return base + "." + relKey
}
type formatterNode struct {
	F      *model.Field
	Alias  string
	Next   *formatterNode
}
func insertByDeps(head *formatterNode, node *formatterNode) *formatterNode {
	if head == nil {
		return node
	}
	var lastMatch *formatterNode
	for cur := head; cur != nil; cur = cur.Next {
		if (node.F.Source != "" && strings.Contains(node.F.Source, cur.Alias)) ||
		   (node.F.Formatter != "" && strings.Contains(node.F.Formatter, cur.Alias)) {
			lastMatch = cur
		}
	}
	if lastMatch == nil {
		// вставляем в начало
		node.Next = head
		return node
	}
	// вставляем после последнего совпадения
	node.Next = lastMatch.Next
	lastMatch.Next = node
	return head
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


// applyFormatter применяет тернарники, затем обычные токены
func applyFormatter(fmtStr string, row map[string]any) string {
	// 1) Тернарники парсим state machine-ом (учитываем кавычки и вложенные { } )
	out := replaceTernaries(fmtStr, row)

	// 2) Затем — обычные токены {path}[i] / [i..j]
	return reToken.ReplaceAllStringFunc(out, func(tok string) string {
		m := reToken.FindStringSubmatch(tok)
		if len(m) == 0 {
			return ""
		}
		path := m[1]
		iStr := m[2]
		jStr := m[3]

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

// Разбирает и заменяет все блоки вида `{? cond ? then : else}`.
// Учитывает кавычки и вложенные { } внутри веток и условия.
func replaceTernaries(s string, row map[string]any) string {
	var out strings.Builder
	n := len(s)
	for i := 0; i < n; {
		// ищем старт `{?`
		if i+1 < n && s[i] == '{' && s[i+1] == '?' {
			// пропускаем `{?`
			i += 2
			depth := 1 // уже открыли одну '{'
			var inQuote byte
			start := i
			for i < n && depth > 0 {
				c := s[i]
				if inQuote != 0 {
					// выходим из кавычек при той же кавычке, если не экранирована
					if c == inQuote && (i == 0 || s[i-1] != '\\') {
						inQuote = 0
					}
				} else {
					switch c {
					case '"', '\'':
						inQuote = c
					case '{':
						depth++
					case '}':
						depth--
						if depth == 0 {
							// захватили весь блок тернарника
							block := s[start : i] // без финальной '}'
							repl := evalTernaryBlock(block, row)
							out.WriteString(repl)
							i++ // съедаем '}'
							goto cont // продолжить внешний цикл
						}
					}
				}
				i++
			}
			// если не закрыли — считаем это текстом как есть
			out.WriteString("{?")
			out.WriteString(s[start:])
			break
		}
		// обычный символ
		out.WriteByte(s[i])
		i++
	cont:
	}
	return out.String()
}

// Разобрать внутренности `{? ... }` на cond ? then : else, учитывая кавычки/{}.
func evalTernaryBlock(block string, row map[string]any) string {
	// Найти разделители: первый '?' и первый ':' на верхнем уровне.
	var inQuote byte
	depth := 0
	qPos, cPos := -1, -1
	for i := 0; i < len(block); i++ {
		c := block[i]
		if inQuote != 0 {
			if c == inQuote && (i == 0 || block[i-1] != '\\') {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case '?':
			if depth == 0 && qPos == -1 {
				qPos = i
			}
		case ':':
			if depth == 0 && qPos != -1 {
				cPos = i
				i = len(block) // нашли оба — выходим
			}
		}
	}
	if qPos == -1 || cPos == -1 || qPos > cPos {
		// нераспознанный тернарник — вернём как есть
		return "{?" + block + "}"
	}

	cond := strings.TrimSpace(block[:qPos])
	thenStr := strings.TrimSpace(block[qPos+1 : cPos])
	elseStr := strings.TrimSpace(block[cPos+1:])

	ok, err := evalCondition(cond, row)

	chosen := elseStr
	if err == nil && ok {
		chosen = thenStr
	}

	// null → пусто
	if isNullLiteral(chosen) {
		return ""
	}

	// снимаем кавычки
	chosen = unquoteIfQuoted(chosen)

	// ВАЖНО: прогоняем результат снова через applyFormatter,
	// чтобы обработать вложенные токены/тернарники
	return applyFormatter(chosen, row)
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
func isNullLiteral(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "null")
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
     
    // Строка в кавычках: "..." или '...'
		if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'')) {
			return s[1 : len(s)-1], nil
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