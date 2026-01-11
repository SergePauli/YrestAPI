package model

import (
	"regexp"
	"strings"
)

// допускаем пустой плейсхолдер {} для ссылки на базовый алиас
var placeholderRe = regexp.MustCompile(`\{([^}]*)\}`)
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isSQLKeyword(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "select", "from", "where", "and", "or", "not", "null", "true", "false",
		"like", "ilike", "similar", "between", "as", "case", "when", "then", "else", "end",
		"on", "inner", "left", "right", "full", "cross", "join", "union", "all", "distinct",
		"order", "by", "group", "limit", "offset", "having", "exists", "in", "is", "over",
		"partition", "filter", "returning", "with", "update", "into", "delete",
		// типы
		"int", "integer", "bigint", "smallint", "serial", "bigserial", "real", "float", "float8", "double", "double precision",
		"numeric", "decimal", "boolean", "bool", "text", "varchar", "character", "char", "timestamp", "date", "time", "interval":
		return true
	default:
		return false
	}
}

func quoteIfNeeded(name string) string {
	if name == "" || name == "*" {
		return name
	}
	if identRe.MatchString(name) && !isSQLKeyword(name) {
		return name
	}
	return quoteIdentifier(name)
}

// isSubquerySource проверяет, начинается ли source с '(' — маркер подзапроса.
func isSubquerySource(src string) bool {
	return strings.HasPrefix(strings.TrimSpace(src), "(")
}

// wrapSubquery оборачивает выражение в скобки, если их нет.
func wrapSubquery(src string) string {
	s := strings.TrimSpace(src)
	if !strings.HasPrefix(s, "(") {
		return "(" + s + ")"
	}
	return s
}

// quoteIdentifier экранирует имя алиаса двойными кавычками.
func quoteIdentifier(name string) string {
	escaped := strings.ReplaceAll(name, `"`, `""`)
	return `"` + escaped + `"`
}

// extractPathsFromExpr достаёт relation-пути из плейсхолдеров {path}.
func extractPathsFromExpr(expr string) []string {
	if expr == "" {
		return nil
	}
	matches := placeholderRe.FindAllStringSubmatchIndex(expr, -1)
	set := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, idx := range matches {
		if len(idx) < 4 {
			continue
		}
		start, end := idx[2], idx[3]
		p := strings.TrimSpace(expr[start:end])
		if p == "" {
			continue
		}
		if _, ok := set[p]; ok {
			continue
		}
		set[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// applyAliasPlaceholders заменяет {path} на алиасы с учётом basePath.
// Если после плейсхолдера в исходной строке идёт '.', возвращаем только алиас (для {rel}.col).
// Иначе возвращаем "alias.column".
func applyAliasPlaceholders(expr string, aliasMap *AliasMap, basePath string) string {
	if aliasMap == nil || expr == "" {
		return expr
	}

	baseAlias := func() string {
		if basePath == "" {
			return "main"
		}
		if a, ok := aliasMap.PathToAlias[basePath]; ok {
			return a
		}
		return ""
	}
	matches := placeholderRe.FindAllStringSubmatchIndex(expr, -1)
	if len(matches) == 0 {
		alias := baseAlias()
		if alias == "" {
			return expr
		}
		return qualifyBareIdentifiers(expr, alias)
	}

	var b strings.Builder
	last := 0

	for _, idx := range matches {
		start, end := idx[0], idx[1]
		pathStart, pathEnd := idx[2], idx[3]
		b.WriteString(expr[last:start])

		path := strings.TrimSpace(expr[pathStart:pathEnd])
		repl := expr[start:end] // fallback

		if aliasMap != nil {
			hasDotNext := end < len(expr) && expr[end] == '.'
			if path == "" {
				if alias := baseAlias(); alias != "" {
					repl = alias
				}
			} else {
				segments := strings.Split(path, ".")
				col := segments[len(segments)-1]
				relPrefix := strings.Join(segments[:len(segments)-1], ".")
				aliasLookup := relPrefix
				if basePath != "" {
					if aliasLookup != "" {
						aliasLookup = basePath + "." + aliasLookup
					} else {
						aliasLookup = basePath
					}
				}
				if aliasLookup == "" {
					if relPrefix != "" {
						aliasLookup = relPrefix
					} else {
						aliasLookup = path
					}
				}
				if alias, ok := aliasMap.PathToAlias[aliasLookup]; ok && strings.TrimSpace(alias) != "" {
					quotedCol := quoteIfNeeded(col)
					if hasDotNext {
						repl = alias
					} else if relPrefix == "" && len(segments) == 1 {
						// одиночный плейсхолдер связи — трактуем как алиас таблицы
						repl = alias
					} else {
						repl = alias + "." + quotedCol
					}
				}
			}
		}

		b.WriteString(repl)
		last = end
	}

	b.WriteString(expr[last:])
	out := b.String()

	alias := baseAlias()
	if alias == "" {
		return out
	}
	return qualifyBareIdentifiers(out, alias)
}

func qualifyBareIdentifiers(expr, alias string) string {
	if alias == "" || strings.TrimSpace(expr) == "" {
		return expr
	}

	skipAfter := map[string]int{
		"from":   2,
		"join":   2,
		"update": 2,
		"into":   2,
		"delete": 2,
	}

	var b strings.Builder
	b.Grow(len(expr) + len(alias))
	inSingle, inDouble := false, false
	skipNext := 0

	for i := 0; i < len(expr); {
		ch := expr[i]

		if inSingle {
			b.WriteByte(ch)
			if ch == '\'' {
				if i+1 < len(expr) && expr[i+1] == '\'' {
					b.WriteByte(expr[i+1])
					i += 2
					continue
				}
				inSingle = false
			}
			i++
			continue
		}
		if inDouble {
			b.WriteByte(ch)
			if ch == '"' {
				if i+1 < len(expr) && expr[i+1] == '"' {
					b.WriteByte('"')
					i += 2
					continue
				}
				inDouble = false
			}
			i++
			continue
		}

		if ch == '\'' {
			inSingle = true
			b.WriteByte(ch)
			i++
			continue
		}
		if ch == '"' {
			inDouble = true
			b.WriteByte(ch)
			i++
			continue
		}

		if isIdentStart(ch) {
			start := i
			i++
			for i < len(expr) && isIdentPart(expr[i]) {
				i++
			}

			ident := expr[start:i]
			lower := strings.ToLower(ident)
			if c, ok := skipAfter[lower]; ok {
				skipNext = c
			}
			if isSQLKeyword(lower) {
				b.WriteString(ident)
				continue
			}

			if skipNext > 0 {
				skipNext--
				b.WriteString(ident)
				continue
			}

			prev := prevNonSpace(expr, start-1)
			next := nextNonSpace(expr, i)
			if prev == '.' || prev == ':' || prev == '"' {
				b.WriteString(ident)
				continue
			}
			if next == '.' || next == '(' {
				b.WriteString(ident)
				continue
			}

			b.WriteString(alias)
			b.WriteByte('.')
			b.WriteString(ident)
			continue
		}

		b.WriteByte(ch)
		i++
	}

	return b.String()
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func prevNonSpace(s string, idx int) byte {
	for i := idx; i >= 0; i-- {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return s[i]
		}
	}
	return 0
}

func nextNonSpace(s string, idx int) byte {
	for i := idx; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return s[i]
		}
	}
	return 0
}
