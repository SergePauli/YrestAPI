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

// LocaleNode универсальный узел словаря
type LocaleNode struct {
    Value string
    Children  map[string]*LocaleNode
}

// Глобальный словарь для активной локали
var ActiveDict = map[string]*LocaleNode{}

// LoadLocales загружает словари из cfg/locales/*.yml
func LoadLocales(locale string) error {
    dir := "cfg/locales"
    path := filepath.Join(dir, locale+".yml")

    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("⚠️ cannot read locale file %s: %w", path, err)
    }

    var raw map[string]any
    if err := yaml.Unmarshal(data, &raw); err != nil {
        return fmt.Errorf("⚠️ unmarshal locale error in %s: %w", path, err)
    }

    ActiveDict = parseNodeMap(raw)
		log.Printf("✅ Loaded locale %s with %+v entries", locale, ActiveDict["used"])
    return nil
}

// parseNodeMap рекурсивно строит словарь
func parseNodeMap(raw map[string]any) map[string]*LocaleNode {
    result := make(map[string]*LocaleNode)

    for key, val := range raw {
			
        switch v := val.(type) {
        case string:
            result[key] = &LocaleNode{
                Value: v,
            }
						
        case map[string]any:
            result[key] = &LocaleNode{
                Children: parseNodeMap(v),
            }
						
        default:
            result[key] = &LocaleNode{
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
func  (n *LocaleNode) Lookup(keys ...string) (string, bool) {
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
