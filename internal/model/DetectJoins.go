package model

import (
	"fmt"
	"math/rand"
	"strings"
)

// DetectJoins определяет, какие JOIN-ы нужны для модели на основе фильтров, сортировки и пресетов
// Возвращает список JoinSpec, которые можно использовать в SQL-запросах
// filterFields, sortFields и presetFields — это поля, по которым нужно искать связи
// Если пресеты не нужны, можно передать nil или пустой срез
func (m *Model) DetectJoins(
	aliasMap *AliasMap,
	filterFields []string,
	sortFields []string, // опционально
	presetFields []string, // опционально.
) ([]*JoinSpec, error) {
	joinMap := map[string]*JoinSpec{}
	joins := make([]*JoinSpec, 0)

	// Объединяем все поля в единый список для рекурсивного поиска
	allFields := make([]string, 0, len(filterFields)+len(sortFields)+len(presetFields))
	allFields = append(allFields, filterFields...)
	if sortFields != nil {
		allFields = append(allFields, sortFields...)
	}
	if presetFields != nil {
		allFields = append(allFields, presetFields...)
	}
	// detectJoinsRecursive рекурсивно определяет JOIN-ы для модели
	toAdd, err := detectJoinsRecursive(m, allFields, joinMap, "", "main", false, aliasMap)
	if err != nil {
		return nil, err
	}
	// Добавляем JOIN-ы filters
	joins = append(joins, toAdd...)
	return joins, nil
}

// detectJoinsRecursive рекурсивно определяет JOIN-ы для модели на основе полей
// fields — список полей, по которым нужно искать связи
// joinMap — карта уже найденных JOIN-ов, чтобы избежать дублирования
// prefix — префикс для алиасов, чтобы избежать конфликтов имен
// throughMode — если true, значит мы находимся в режиме обработки через промежуточ
func detectJoinsRecursive(
	m *Model,
	fields []string,
	joinMap map[string]*JoinSpec,
	pathPrefix string,
	parentAlias string,
	throughMode bool,
	aliasMap *AliasMap,
) ([]*JoinSpec, error) {
	joins := make([]*JoinSpec, 0)

	for _, field := range fields {
		if expanded := ExpandAliasPath(m, field); expanded != "" {
			field = expanded
		}
		// Разбиваем путь: relName[.tail?]
		var relName, tail string
		if i := strings.IndexByte(field, '.'); i >= 0 {
			relName = field[:i]
			tail = field[i+1:]
		} else {
			relName = field
			tail = "" // терминальный сегмент
		}
		relName = strings.TrimSpace(relName)
		if relName == "" {
			continue
		}

		rel, ok := m.Relations[relName]
		if !ok || (rel._ModelRef == nil && !rel.Polymorphic) {
			continue
		}
		if rel.Polymorphic {
			// joins для полиморфных связей не строим
			continue
		}

		fullPath := pathPrefix + relName
		alias, ok := aliasMap.PathToAlias[fullPath]
		if !ok {
			return joins, fmt.Errorf("alias not found for path %s в карте %+v for model %s", fullPath, aliasMap.PathToAlias, m.Name)
		}

		// Если JOIN уже добавлен — ок, но возможно нужна рекурсия глубже
		if _, exists := joinMap[alias]; !exists {
			// THROUGH
			if rel.Through != "" && !throughMode {
				throughAlias := generateUniqueAlias(joinMap)

				onClause := fmt.Sprintf("%s.%s = %s.%s", parentAlias, rel.PK, throughAlias, rel.FK)
				joinMap[throughAlias] = &JoinSpec{
					Table:    rel._ThroughRef.Table,
					Alias:    throughAlias,
					On:       onClause,
					JoinType: "LEFT JOIN",
					Where:    replaceTableWithAlias(rel.ThroughWhere, throughAlias),
				}
				joins = append(joins, joinMap[throughAlias])

				// связь через промежуточную → ищем финальную
				var finalRel *ModelRelation
				for _, subRel := range rel._ThroughRef.Relations {
					if subRel._ModelRef == rel._ModelRef {
						finalRel = subRel
						break
					}
				}
				if finalRel == nil {
					return joins, fmt.Errorf("no final relation found in through %s -> %s", rel._ThroughRef.Table, rel._ModelRef.Table)
				}

				joinMap[alias] = &JoinSpec{
					Table:    rel._ModelRef.Table,
					Alias:    alias,
					On:       fmt.Sprintf("%s.%s = %s.%s", throughAlias, finalRel.FK, alias, finalRel.PK),
					JoinType: "LEFT JOIN",
					Where:    replaceTableWithAlias(rel.Where, alias),
					Distinct: rel.Type == "has_many" || rel.Type == "has_one",
				}
				joins = append(joins, joinMap[alias])
			} else {
				// Обычный JOIN
				var onClause string
				switch rel.Type {
				case "belongs_to":
					// parent.FK = alias.PK
					onClause = fmt.Sprintf("%s.%s = %s.%s", parentAlias, rel.FK, alias, rel.PK)
				case "has_one", "has_many":
					// alias.FK = parent.PK
					onClause = fmt.Sprintf("%s.%s = %s.%s", alias, rel.FK, parentAlias, rel.PK)
				default:
					return joins, fmt.Errorf("unsupported relation type: %s", rel.Type)
				}

				joinMap[alias] = &JoinSpec{
					Table:    rel._ModelRef.Table,
					Alias:    alias,
					On:       onClause,
					JoinType: "LEFT JOIN",
					Where:    replaceTableWithAlias(rel.Where, alias),
					Distinct: rel.Type == "has_many" || rel.Type == "has_one",
				}
				joins = append(joins, joinMap[alias])
			}
		}

		// Рекурсия, только если есть хвост
		if tail != "" {
			nextPrefix := fullPath + "."
			toADD, err := detectJoinsRecursive(
				rel._ModelRef,
				[]string{tail},
				joinMap,
				nextPrefix,
				alias,
				false,
				aliasMap,
			)
			if err != nil {
				return joins, err
			}
			joins = append(joins, toADD...)
		}
	}

	return joins, nil
}

// generateUniqueAlias создает уникальный алиас для JOIN-а
// Используется для генерации алиасов, которые не конфликтуют с уже существующими
func generateUniqueAlias(existing map[string]*JoinSpec) string {
	for {
		alias := randomAlias(3) // например "ab", "xz"
		if _, exists := existing[alias]; !exists {
			return alias
		}
	}
}

// randomAlias генерирует случайный алиас из букв
// Используется для создания уникальных алиасов в JOIN-ах
func randomAlias(length int) string {
	letters := []rune("bcfghjklmnpqrstvwxz")
	b := make([]rune, length)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
func replaceTableWithAlias(where string, alias string) string {
	if where == "" {
		return ""
	}
	return strings.ReplaceAll(where, ".", alias+".")
}
