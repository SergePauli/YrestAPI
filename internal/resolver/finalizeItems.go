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
	// 0 удаляем префиксы alias'ов у preset-полей belongs_to
	//stripPresetPrefixes(m, p, items, "")

	// 0.5 применяем алиасы для простых полей (заменяем source ключ на alias)
	applyFieldAliases(p, items)

	// локализация
	if model.HasLocales {
		applyLocalization(m, p, items) // или locale из запроса
	}

	// 1 посчитать все formatter'ы до удаления internal
	if err := applyAllFormatters(m, p, items, ""); err != nil {
		return err
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
			deleteExactPath(items[i], k)
		}
		// удаления поддеревьев (person, members, members.contact)
		for _, pref := range prefixes {
			deletePrefix(items[i], pref)
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

// applyFieldAliases переносит значения из исходных ключей в alias и удаляет source.
// Работает только для плоских полей текущего пресета (не preset/formatter).
func applyFieldAliases(p *model.DataPreset, items []map[string]any) {
	if p == nil || len(items) == 0 {
		return
	}
	for _, f := range p.Fields {
		if f.Type == "preset" || f.Type == "formatter" {
			continue
		}
		if f.Alias == "" || f.Alias == f.Source {
			continue
		}
		src := f.Source
		dst := f.Alias
		for i := range items {
			if _, ok := items[i][dst]; ok {
				// уже есть конечный ключ — не трогаем
				continue
			}
			if v, ok := items[i][src]; ok {
				items[i][dst] = v
				delete(items[i], src)
			}
		}
	}
}

// --- helpers: recursive delete for dotted paths & prefixes ---

// deleteExactPath удаляет ключ по точному dotted-пути из вложенных map/слайсов.
// Пример: "person.first_name" или "members.first_name" (для каждого элемента).
func deleteExactPath(root map[string]any, path string) {
	segs := strings.Split(path, ".")
	var walk func(cur any, idx int)
	walk = func(cur any, idx int) {
		if idx >= len(segs) {
			return
		}
		switch node := cur.(type) {
		case map[string]any:
			key := segs[idx]
			if idx == len(segs)-1 {
				delete(node, key)
				return
			}
			if next, ok := node[key]; ok {
				// углубляемся
				walk(next, idx+1)
				// если дальше был срез, он сам обработан; map оставляем как есть
			}
		case []any:
			for _, it := range node {
				walk(it, idx)
			}
		case []map[string]any:
			for i := range node {
				walk(node[i], idx)
			}
		default:
			// тупик: путь длиннее, чем структура
			return
		}
	}
	walk(root, 0)
}

// deletePrefix удаляет ЦЕЛОЕ поддерево по dotted-префиксу.
// Пример: "person" — вырежет person; "members" — вырежет массив целиком;
// "members.contact" — из каждого элемента members вырежет contact.
func deletePrefix(root map[string]any, prefix string) {
	segs := strings.Split(prefix, ".")
	// special-case: удалить прямо ключ у родителя
	var walk func(parent any, cur any, idx int, lastKey string)
	walk = func(parent any, cur any, idx int, lastKey string) {
		// если дошли до конца префикса — удалить узел у parent
		if idx >= len(segs) {
			switch p := parent.(type) {
			case map[string]any:
				delete(p, lastKey)
			case []any:
				// удалять элемент по ключу из слайса не требуется (не применимо)
			case []map[string]any:
				// аналогично
			}
			return
		}
		switch node := cur.(type) {
		case map[string]any:
			key := segs[idx]
			next, ok := node[key]
			if !ok {
				return
			}
			// идём глубже
			walk(node, next, idx+1, key)
		case []any:
			for _, it := range node {
				walk(parent, it, idx, lastKey)
			}
		case []map[string]any:
			for i := range node {
				walk(parent, node[i], idx, lastKey)
			}
		default:
			return
		}
	}
	// стартуем от фиктивного родителя, чтобы корректно удалить корневой ключ
	walk(root, root, 0, "")
}

// синтетический пресет для through-модели (одно поле ведёт к конечной модели)
func makeThroughSyntheticPreset(unwrapKey, finalPresetName string) *model.DataPreset {
	return &model.DataPreset{
		Fields: []model.Field{
			{
				Type:         "preset",
				Source:       unwrapKey,
				Alias:        unwrapKey,
				NestedPreset: finalPresetName,
			},
		},
	}
}

func applyAllFormatters(m *model.Model, p *model.DataPreset, items []map[string]any, prefix string) error {
	// ---------- helpers ----------
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
				if mm, ok := v.(map[string]any); ok {
					cur = mm
					continue
				}
			}
			mm := map[string]any{}
			cur[seg] = mm
			cur = mm
		}
		return cur
	}

	getMapAt := func(root map[string]any, pfx string) (map[string]any, bool) {
		if pfx == "" {
			return root, true
		}
		cur := any(root)
		for _, seg := range strings.Split(pfx, ".") {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			mm, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			cur, ok = mm[seg]
			if !ok {
				return nil, false
			}
		}
		mm, ok := cur.(map[string]any)
		return mm, ok
	}

	// найти ключ unwrapKey в through-модели, ведущий к final-модели

	findUnwrapKey := func(through, final *model.Model) string {
		for k, r2 := range through.Relations {
			if r2 != nil && r2.Type == "belongs_to" && r2.GetModelRef() == final {
				return k
			}
		}
		return ""
	}

	fieldKey := func(f *model.Field) string {
		if strings.TrimSpace(f.Alias) != "" {
			return f.Alias
		}
		return f.Source
	}

	var head *formatterNode
	getValueAtPath := func(root map[string]any, path string) (any, bool) {
		cur := any(root)
		for _, seg := range strings.Split(path, ".") {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			mm, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			cur, ok = mm[seg]
			if !ok {
				return nil, false
			}
		}
		return cur, true
	}

	// ---------- 1) Подготовительный проход ----------
	for _, f := range p.Fields {
		if f.Type != "preset" {
			if f.Type == "formatter" {
				pf := &f
				head = insertByDeps(head, &formatterNode{Alias: f.Alias, F: pf})
			} else if f.Type == "nested_field" {
				pf := &f
				head = insertByDeps(head, &formatterNode{Alias: f.Alias, F: pf})
			}
			continue
		}

		rel := m.Relations[f.Source]
		if rel == nil {
			continue
		}

		// ВНИМАНИЕ: для рекурсий вниз используем ИМЕННО f.Source,
		// потому что реальные данные приходят под source, а не под alias.
		nextPrefixSource := prefixFor(prefix, f.Source)

		switch rel.Type {
		case "has_one", "has_many":
			// has_* приходят из дочерних резолверов — внутрь НЕ уходим.
			// Если есть through — распрямим контейнер до конечной модели.
			if rel.Through != "" && rel.GetThroughRef() != nil && rel.GetModelRef() != nil {
				through := rel.GetThroughRef()
				final := rel.GetModelRef()
				unwrapKey := findUnwrapKey(through, final)
				if unwrapKey != "" {
					key := fieldKey(&f) // куда кладём ветку в JSON (alias > source)
					for i := range items {
						parent := getCtx(items[i], prefix)
						switch rel.Type {
						case "has_one":
							if sub, ok := parent[key].(map[string]any); ok {
								if inner, ok := sub[unwrapKey].(map[string]any); ok {
									parent[key] = inner
								}
							}
						case "has_many":
							switch vv := parent[key].(type) {
							case []map[string]any:
								out := make([]map[string]any, 0, len(vv))
								for _, elem := range vv {
									if inner, ok := elem[unwrapKey].(map[string]any); ok {
										out = append(out, inner)
									}
								}
								parent[key] = out
							case []any:
								out := make([]map[string]any, 0, len(vv))
								for _, it := range vv {
									if elem, ok := it.(map[string]any); ok {
										if inner, ok := elem[unwrapKey].(map[string]any); ok {
											out = append(out, inner)
										}
									}
								}
								parent[key] = out
							}
						}
					}
				}
			}
			// больше ничего для has_* не делаем

		case "belongs_to":
			// для belongs_to — единственный тип, куда рекурсируем
			if nestedM := rel.GetModelRef(); nestedM != nil {
				var nested *model.DataPreset
				if f.GetPresetRef() != nil {
					nested = f.GetPresetRef()
				} else if f.NestedPreset != "" {
					nested = nestedM.Presets[f.NestedPreset]
				}
				if nested != nil {
					// РЕКУРСИЯ ПО f.Source, НЕ по alias
					if err := applyAllFormatters(nestedM, nested, items, nextPrefixSource); err != nil {
						return err
					}
				}
			}
		}

		// контейнерный форматтер выполним на текущем уровне
		if strings.TrimSpace(f.Formatter) != "" {
			pf := &f
			head = insertByDeps(head, &formatterNode{Alias: f.Alias, F: pf})
		}
	}

	// ---------- 2) Выполнение форматтеров текущего уровня ----------
	for node := head; node != nil; node = node.Next {
		f := node.F
		switch f.Type {

		case "formatter":
			tpl := f.Source
			target := f.Alias
			for i := range items {
				ctx := getCtx(items[i], prefix) // строго своя ветка
				ctx[target] = applyFormatter(tpl, ctx)
			}

		case "nested_field":
			path := strings.TrimSpace(f.Source)
			if strings.HasPrefix(path, "{") && strings.HasSuffix(path, "}") && len(path) >= 2 {
				path = strings.TrimSpace(path[1 : len(path)-1])
			}
			if path == "" {
				continue
			}
			targetKey := f.Alias
			if strings.TrimSpace(targetKey) == "" {
				targetKey = path
			}
			for i := range items {
				if val, ok := getValueAtPath(items[i], path); ok {
					ctx := getCtx(items[i], prefix)
					ctx[targetKey] = val
				}
			}

		case "preset":
			if strings.TrimSpace(f.Formatter) == "" {
				continue
			}
			// ЧИТАЕМ дочерний контекст по f.Source (реальные данные),
			// если его нет — fallback на alias-ветку (на случай когда данные действительно под alias).
			childPrefixSrc := prefixFor(prefix, f.Source)
			childPrefixAli := prefixFor(prefix, fieldKey(f))
			for i := range items {
				parent := getCtx(items[i], prefix)

				var child map[string]any
				if m, ok := getMapAt(items[i], childPrefixSrc); ok {
					child = m
				} else if m, ok := getMapAt(items[i], childPrefixAli); ok {
					child = m
				}
				if child != nil {
					parent[f.Alias] = applyFormatter(f.Formatter, child)
				}
			}
		}
	}

	return nil
}

// удалить префиксы alias'ов у preset-полей belongs_to

func prefixFor(base string, relKey string) string {
	if base == "" {
		return relKey
	}
	return base + "." + relKey
}

type formatterNode struct {
	F     *model.Field
	Alias string
	Next  *formatterNode
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
			curPath := relKey
			if prefix != "" {
				curPath = prefix + "." + relKey
			}

			if f.Internal {
				*prefixes = append(*prefixes, curPath)
			}

			if !ok || rel == nil {
				continue
			}

			// belongs_to — как раньше
			if rel.Type == "belongs_to" && rel.GetModelRef() != nil {
				nestedModel := rel.GetModelRef()
				var nested *model.DataPreset
				if f.NestedPreset != "" {
					nested = nestedModel.Presets[f.NestedPreset]
				}
				if nested != nil {
					collectInternalMarkers(nestedModel, nested, curPath, prefixes, exacts)
				}
			}

			// through — рекурсивно зайдём в синтетический пресет промежуточной модели
			if rel.Through != "" && rel.GetThroughRef() != nil && rel.GetModelRef() != nil {
				through := rel.GetThroughRef()
				final := rel.GetModelRef()
				unwrapKey := ""
				for k, r2 := range through.Relations {
					if r2 != nil && r2.Type == "belongs_to" && r2.GetModelRef() == final {
						unwrapKey = k
						break
					}
				}
				if unwrapKey != "" {
					finalPresetName := f.NestedPreset
					if finalPresetName == "" {
						if _, ok := final.Presets["item"]; ok {
							finalPresetName = "item"
						}
					}
					syn := makeThroughSyntheticPreset(unwrapKey, finalPresetName)
					collectInternalMarkers(through, syn, curPath, prefixes, exacts)
				}
			}
			continue
		}

		if f.Internal {
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
							block := s[start:i] // без финальной '}'
							repl := evalTernaryBlock(block, row)
							out.WriteString(repl)
							i++       // съедаем '}'
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
		if (s[0] == '"' && s[len(s)-1] == '"') ||
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
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
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
			case "==":
				return ln == rn
			case "!=":
				return ln != rn
			case ">":
				return ln > rn
			case ">=":
				return ln >= rn
			case "<":
				return ln < rn
			case "<=":
				return ln <= rn
			}
			return false
		}
	}
	// булево
	if lb, lok := lv.(bool); lok {
		if rb, rok := rv.(bool); rok {
			switch op {
			case "==":
				return lb == rb
			case "!=":
				return lb != rb
			default:
				return false
			}
		}
	}

	// строковое сравнение (лексикографическое для >,<)
	ls := toString(lv)
	rs := toString(rv)
	switch op {
	case "==":
		return ls == rs
	case "!=":
		return ls != rs
	case ">":
		return ls > rs
	case ">=":
		return ls >= rs
	case "<":
		return ls < rs
	case "<=":
		return ls <= rs
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
