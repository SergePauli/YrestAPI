package model

import (
	"fmt"
)

type AliasMap struct {
	PathToAlias map[string]string
	AliasToPath map[string]string
}
// BuildAliasMap создает карту алиасов для модели
// Возвращает AliasMap, где ключи — алиасы, а значения — пути к полям
// Если обнаружена циклическая зависимость, возвращает ошибку
func BuildAliasMap(model *Model) (*AliasMap, error) {
    aliasToPath := map[string]string{}
    pathToAlias := map[string]string{}
    visited := map[string]bool{}
		aliasCounter := 0

    var walk func(m *Model, prefix, fullPath string) error
    walk = func(m *Model, prefix, fullPath string) error {
        if visited[fullPath] {
            return fmt.Errorf("❌ cycle detected in path: %s", fullPath)
        }
        visited[fullPath] = true

        for key, rel := range m.Relations {
            // Пропускаем связи на родительскую модель (inverse)
            if rel.InverseOf != "" {
                continue
            }

            nextPath := key
            if fullPath != "" {
                nextPath = fullPath + "." + key
            }

            alias := fmt.Sprintf("t%d", aliasCounter)
            aliasCounter++
            aliasToPath[alias] = nextPath
            pathToAlias[nextPath] = alias

            if rel._ModelRef != nil && len(rel._ModelRef.Relations) > 0 {
                if err := walk(rel._ModelRef, alias, nextPath); err != nil {
                    return err
                }
            }
        }
        return nil
    }

    err := walk(model, "", "")
    if err != nil {
        return nil, err
    }
		

    return &AliasMap{
        AliasToPath: aliasToPath,
        PathToAlias: pathToAlias,
    }, nil
}
