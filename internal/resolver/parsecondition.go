package resolver

import "strings"
func parseCondition(cond string) (key string, val any, ok bool) {
    cond = strings.TrimSpace(cond)
    cond = strings.TrimPrefix(cond, ".") // убираем ведущую точку

    // ищем оператор по приоритету (более длинные сначала)
    ops := []string{"<=", ">=", "<", ">", " in ", " cnt ", " start ", " end ", "="}
    for _, op := range ops {
        if strings.Contains(cond, op) {
            parts := strings.SplitN(cond, op, 2)
            if len(parts) != 2 {
                return "", nil, false
            }
            field := strings.TrimSpace(parts[0])
            raw := strings.TrimSpace(parts[1])
            raw = strings.Trim(raw, "'\"") // убираем кавычки

            switch op {
            case "=":
                return field + "__eq", raw, true
            case "<":
                return field + "__lt", raw, true
            case "<=":
                return field + "__lte", raw, true
            case ">":
                return field + "__gt", raw, true
            case ">=":
                return field + "__gte", raw, true
            case " in ":
                // поддержка списков
                items := strings.Split(raw, ",")
                vals := make([]string, 0, len(items))
                for _, it := range items {
                    vals = append(vals, strings.Trim(strings.TrimSpace(it), "'\""))
                }
                return field + "__in", vals, true
            case " cnt ":
                return field + "__cnt", raw, true
            case " start ":
                return field + "__start", raw, true
            case " end ":
                return field + "__end", raw, true
            }
        }
    }
    return "", nil, false
}