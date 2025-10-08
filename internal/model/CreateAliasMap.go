package model

import (
	"fmt"
	"log"
	"sort"
	"strings"
)
func (m *Model) CreateAliasMap(model *Model, preset *DataPreset, filters map[string]interface{}, sorts []string) (*AliasMap, error) {
	
		// 1. Генерация карты на лету
	aliasMap, err := BuildAliasMap(m, preset, filters, sorts)
	if err != nil {
		return nil, fmt.Errorf("build alias map failed: %w", err)
	} else {		
		log.Printf("Alias map for model '%s' built successfully", model.Name)              
	}
    return aliasMap, nil
}

// Построить FieldsAliasMap и FieldPaths для ВСЕХ пресетов во всём Registry.
// Запускать ПОСЛЕ: LoadModelsFromDir(...) → линковка _ModelRef → ValidateAllPresets().
func BuildPresetAliasMaps() error {
	for _, m := range Registry {
		if err := BuildPresetAliasMapsForModel(m); err != nil {
			return fmt.Errorf("build alias maps for model %s: %w", m.Name, err)
		}
	}
	return nil
}

// Построить FieldsAliasMap/FieldPaths для всех пресетов конкретной модели.
// обёртка + приватная рекурсивная функция с visited.
func BuildPresetAliasMapsForModel(m *Model) error {
    return buildPresetAliasMapsForModel(m, map[*Model]bool{})
}

func buildPresetAliasMapsForModel(m *Model, visited map[*Model]bool) error {
    if m == nil {
        return fmt.Errorf("nil model")
    }
    if visited[m] { // <-- ANTI-CYCLE ПО МОДЕЛЯМ
        return nil
    }
    visited[m] = true

    // Сначала — зависимые модели из nested-полей (чтобы у них тоже были свои карты)
    for _, dp := range m.Presets {
        for _, f := range dp.Fields {
            if strings.ToLower(strings.TrimSpace(f.Type)) != "preset" || f.NestedPreset == "" {
                continue
            }
            rel := m.Relations[f.Source]
            if rel == nil || rel._ModelRef == nil { continue }
            if err := buildPresetAliasMapsForModel(rel._ModelRef, visited); err != nil {
                return err
            }
        }
    }

    // Затем — строим карты для всех пресетов текущей модели (как раньше)
    for presetName, dp := range m.Presets {
        paths, err := collectPresetRelationPaths(m, presetName)
        if err != nil { return fmt.Errorf("%s.%s: %w", m.Name, presetName, err) }
        sort.Slice(paths, func(i, j int) bool {
            di, dj := strings.Count(paths[i], "."), strings.Count(paths[j], ".")
            if di != dj { return di < dj }
            return paths[i] < paths[j]
        })
        ptoa, atop := map[string]string{}, map[string]string{}
        for i, p := range paths {
            a := fmt.Sprintf("t%d", i)
            ptoa[p] = a
            atop[a] = p
        }
        //dp.FieldPaths = paths
        dp.FieldsAliasMap = &AliasMap{PathToAlias: ptoa, AliasToPath: atop}
    }
    return nil
}


// Собирает все relation-пути ("a", "a.b", "a.b.c") из NestedPreset-полей данного пресета.
// Учитывает политику ре-энтри по МОДЕЛИ: rel.Reentrant и лимит посещений модели effMax.
// По умолчанию effMax=1 (только одно посещение модели на пути; без возвратов).
func collectPresetRelationPaths(root *Model, presetName string) ([]string, error) {
    set := make(map[string]struct{})
    var dfs func(curr *Model, pName, currPath string, stack []*Model) error
    dfs = func(curr *Model, pName, currPath string, stack []*Model) error {
        pr := curr.Presets[pName]
        for _, f := range pr.Fields {
            if strings.ToLower(strings.TrimSpace(f.Type)) != "preset" || f.NestedPreset == "" { continue }
            rel := curr.Relations[f.Source]
            next := rel._ModelRef

            // фиксируем сам путь (алиас нужен уже на этом уровне)
            nextPath := f.Source
            if currPath != "" { nextPath = currPath + "." + f.Source }
            set[nextPath] = struct{}{}

            // --- ANTI-REENTRY ПО МОДЕЛИ ---
            repeats := 0
            for _, m := range stack { if m == next { repeats++ } }
            if repeats > 0 {
                eff := f.MaxDepth
                if eff <= 0 { eff = rel.MaxDepth }
                if eff <= 0 { eff = 1 } // трактуем как "макс. посещений модели на пути"
                if !rel.Reentrant || repeats >= eff {
                    // путь добавили, но глубже НЕ идём
                    continue
                }
            }
            // --------------------------------

            if err := dfs(next, f.NestedPreset, nextPath, append(stack, next)); err != nil {
                return err
            }
        }
        return nil
    }
    if err := dfs(root, presetName, "", []*Model{root}); err != nil {
        return nil, err
    }
    // собрать отсортированный срез
    paths := make([]string, 0, len(set))
    for p := range set { paths = append(paths, p) }
    return paths, nil
}




