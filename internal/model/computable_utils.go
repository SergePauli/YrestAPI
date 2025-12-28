package model

import (
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\{([^}]+)\}`)

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
	matches := placeholderRe.FindAllStringSubmatch(expr, -1)
	set := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		p := strings.TrimSpace(m[1])
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
	matches := placeholderRe.FindAllStringSubmatchIndex(expr, -1)
	if len(matches) == 0 {
		return expr
	}

	var b strings.Builder
	last := 0

	for _, idx := range matches {
		start, end := idx[0], idx[1]
		pathStart, pathEnd := idx[2], idx[3]
		b.WriteString(expr[last:start])

		path := strings.TrimSpace(expr[pathStart:pathEnd])
		repl := expr[start:end] // fallback

		if path != "" && aliasMap != nil {
			hasDotNext := end < len(expr) && expr[end] == '.'
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
				if hasDotNext {
					repl = alias
				} else {
					repl = alias + "." + col
				}
			}
		}

		b.WriteString(repl)
		last = end
	}

	b.WriteString(expr[last:])
	return b.String()
}
