package model

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var HasLocales = false

var ActiveLocale string
var LocaleDir = "cfg/locales"

type LayoutSettings struct {
	Date     string
	Time     string
	DateTime string
}

var ActiveLayouts = LayoutSettings{
	Date:     "2006-01-02",
	Time:     "15:04:05",
	DateTime: "2006-01-02 15:04:05",
}

// LocaleNode универсальный узел словаря
type LocaleNode struct {
	Value    string
	Children map[any]*LocaleNode
}

// Глобальный словарь для активной локали
var ActiveDict = map[any]*LocaleNode{}

// LoadLocales загружает словари из cfg/locales/*.yml
func LoadLocales(locale string) error {
	path := filepath.Join(LocaleDir, locale+".yml")

	ActiveLocale = locale
	ActiveLayouts = LayoutSettings{
		Date:     "2006-01-02",
		Time:     "15:04:05",
		DateTime: "2006-01-02 15:04:05",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("⚠️ cannot read locale file %s: %w", path, err)
	}

	var raw map[any]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("⚠️ unmarshal locale error in %s: %w", path, err)
	}

	if ls, ok := raw["layoutSettings"]; ok {
		switch v := ls.(type) {
		case map[string]any:
			if s, ok := v["date"].(string); ok && s != "" {
				ActiveLayouts.Date = s
			}
			if s, ok := v["ttime"].(string); ok && s != "" {
				ActiveLayouts.Time = s
			}
			if s, ok := v["datetime"].(string); ok && s != "" {
				ActiveLayouts.DateTime = s
			}
		case map[interface{}]interface{}:
			if s, ok := v["date"].(string); ok && s != "" {
				ActiveLayouts.Date = s
			}
			if s, ok := v["ttime"].(string); ok && s != "" {
				ActiveLayouts.Time = s
			}
			if s, ok := v["datetime"].(string); ok && s != "" {
				ActiveLayouts.DateTime = s
			}
		}
		delete(raw, "layoutSettings")
	}

	ActiveDict = parseNodeMap(raw)
	log.Printf("✅ Loaded locale %s with %+v entries", locale, ActiveDict["used"])
	return nil
}

// parseNodeMap рекурсивно строит словарь
func parseNodeMap(raw map[any]any) map[any]*LocaleNode {
	result := make(map[any]*LocaleNode)

	for key, val := range raw {
		k := key

		switch v := val.(type) {
		case string:
			result[k] = &LocaleNode{
				Value: v,
			}

		case map[string]any:
			mv := make(map[any]any, len(v))
			for kk, vv := range v {
				mv[kk] = vv
			}
			result[k] = &LocaleNode{Children: parseNodeMap(mv)}
		case map[int]any:
			mv := make(map[any]any, len(v))
			for kk, vv := range v {
				mv[kk] = vv
			}
			result[k] = &LocaleNode{Children: parseNodeMap(mv)}

		case map[interface{}]interface{}:
			mv := make(map[any]any, len(v))
			for kk, vv := range v {
				mv[kk] = vv
			}
			result[k] = &LocaleNode{Children: parseNodeMap(mv)}

		default:
			result[k] = &LocaleNode{
				Value: fmt.Sprintf("%v", v),
			}

		}
	}

	return result
}

// Translate ищет перевод по пути: model → preset → field → subkey ...
func Translate(path ...string) string {
	node := ActiveDict
	var current *LocaleNode

	for _, p := range path {
		next, ok := node[p]
		if !ok {
			return path[len(path)-1] // нет перевода → вернуть оригинал
		}
		current = next
		if current.Children != nil {
			node = current.Children
		}
	}

	if current != nil && current.Value != "" {
		return current.Value
	}
	return path[len(path)-1]
}
func (n *LocaleNode) Lookup(keys ...any) (string, bool) {
	if n == nil {
		return "", false
	}
	cur := n
	//log.Printf("Lookup in node: %+v with keys: %v\n", cur, keys)
	for _, k := range keys {
		if cur == nil {
			return "", false
		}
		next, ok := cur.Children[k]
		if !ok {
			return "", false
		}
		cur = next
	}

	if cur.Value != "" {
		return cur.Value, true
	}
	return "", false
}
