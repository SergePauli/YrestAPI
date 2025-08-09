package model

import (
	"fmt"
	"regexp"
	"strconv"
)
var templateRegexp = regexp.MustCompile(`\{([\w\.]+)(\[(\d+)(\.\.(\d+))?\])?\}`)

func FormatTemplate(template string, row map[string]any) string {
    return templateRegexp.ReplaceAllStringFunc(template, func(match string) string {
        parts := templateRegexp.FindStringSubmatch(match)
        key := parts[1]              // поле: surname или name
        from := parts[3]            // начальный индекс (например, 0)
        to := parts[5]              // конечный индекс (например, 1)

        valRaw, ok := row[key]
        if !ok || valRaw == nil {
            return ""
        }
        val := fmt.Sprintf("%v", valRaw)
        if from == "" {
            return val
        }

        startIdx, _ := strconv.Atoi(from)
        endIdx := startIdx + 1
        if to != "" {
            endIdx, _ = strconv.Atoi(to)
        }

        if startIdx >= len(val) {
            return ""
        }
        if endIdx > len(val) {
            endIdx = len(val)
        }
        return val[startIdx:endIdx]
    })
}
