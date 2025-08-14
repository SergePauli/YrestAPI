package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Разрешённые ключи для объектов
var allowedModelKeys = map[string]bool{
	"table":     true,
	"relations": true,
	"presets":   true,
}

var allowedRelationKeys = map[string]bool{
	"model":   true,
	"type":    true,
	"fk":      true,
	"pk":      true,
	"table":   true,
	"where":   true,
	"order":   true,
	"through": true,
	"through_where": true,
}

var allowedPresetKeys = map[string]bool{
	"fields": true,
}

var allowedFieldKeys = map[string]bool{
	"source":        true,
	"type":          true,
	"alias":         true,	
	"preset": 			 true,
	"internal":      true,
	"formatter":    true,
}

// Разрешённые значения для type в полях
var allowedFieldTypeValues = map[string]bool{
	"int":       true,
	"string":    true,
	"formatter": true,
	"preset":    true,
	"bool":      true,
	"float":     true,
	"date":      true,
	"datetime":  true,
}

func validateYAMLNode(node *yaml.Node, context string) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := validateYAMLNode(child, "model"); err != nil {
				return err
			}
		}

	case yaml.MappingNode:
		var allowedKeys map[string]bool
		switch context {
		case "model":
			allowedKeys = allowedModelKeys
		case "relation":
			allowedKeys = allowedRelationKeys
		case "preset":
			allowedKeys = allowedPresetKeys
		case "field":
			allowedKeys = allowedFieldKeys
		default:
			allowedKeys = nil // свободная форма
		}

		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value

			if allowedKeys != nil && !allowedKeys[key] {
				return fmt.Errorf("unknown key '%s' in %s", key, context)
			}

			// Проверка допустимых значений для type в поле
			if context == "field" && key == "type" {
				if !allowedFieldTypeValues[valNode.Value] {
					return fmt.Errorf("unknown type value '%s' in field", valNode.Value)
				}
			}

			// Определяем новый контекст
			nextContext := ""
			if context == "model" && key == "relations" {
				nextContext = "relations-map"
			} else if context == "relations-map" {
				nextContext = "relation"
			} else if context == "model" && key == "presets" {
				nextContext = "presets-map"
			} else if context == "presets-map" {
				nextContext = "preset"
			} else if context == "preset" && key == "fields" {
				nextContext = "fields-seq"
			} else if context == "field" {
				nextContext = "field-value"
			} else {
				nextContext = context
			}

			if err := validateYAMLNode(valNode, nextContext); err != nil {
				return err
			}
		}

	case yaml.SequenceNode:
		if context == "fields-seq" {
			for _, item := range node.Content {
				if err := validateYAMLNode(item, "field"); err != nil {
					return err
				}
			}
		} else {
			for _, item := range node.Content {
				if err := validateYAMLNode(item, context); err != nil {
					return err
				}
			}
		}

	case yaml.ScalarNode:
		// скаляры не валидируем на ключи — они уже проверяются при разборе MappingNode
	}

	return nil
}
