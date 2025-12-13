package resolver

import (
	"YrestAPI/internal/model"
	"fmt"
	"strings"
)

// buildOrderSorts преобразует rel.Order в список Sorts дочернего запроса.
// Пример: ".last_name ASC, .first_name DESC" -> ["<prefix>last_name ASC", "<prefix>first_name DESC", "<prefix>id ASC"].
// prefix пустой для прямого has_ и равен "<unwrapKey>." для through.
func buildOrderSorts(order string, prefix string) []string {
	order = strings.TrimSpace(order)
	if order == "" {
		if prefix == "" {
			return []string{"id ASC"}
		}
		return []string{prefix + "id ASC"}
	}
	var sorts []string
	for _, part := range strings.Split(order, ",") {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		dir := "ASC"
		up := strings.ToUpper(s)
		switch {
		case strings.HasSuffix(up, " DESC"):
			dir = "DESC"
			s = strings.TrimSpace(s[:len(s)-4])
		case strings.HasSuffix(up, " ASC"):
			dir = "ASC"
			s = strings.TrimSpace(s[:len(s)-3])
		}
		if strings.HasPrefix(s, ".") {
			s = s[1:]
		}
		if s == "" {
			continue
		}
		if prefix != "" {
			sorts = append(sorts, prefix+s+" "+dir)
		} else {
			sorts = append(sorts, s+" "+dir)
		}
	}
	// стабильный тай-брейкер
	if prefix != "" {
		sorts = append(sorts, prefix+"id ASC")
	} else {
		sorts = append(sorts, "id ASC")
	}
	return sorts
}
// internal/resolver/through.go
func MakeThroughChildRequest(
	parent *model.Model,         // родительская модель (напр. Project)
	rel    *model.ModelRelation, // связь из родителя (напр. persons/email/owner)
	nestedPreset string,         // пресет конечной модели (напр. "item")
	parentIDs []any,             // список PK родителя из главного селекта
) (IndexRequest, error) {

	through := rel.GetThroughRef() // промежуточная модель (напр. ProjectMember / PersonContact)
	final   := rel.GetModelRef()   // конечная модель (напр. Person / Contact)
	if through == nil || final == nil {
		return IndexRequest{}, fmt.Errorf("through or final model is nil")
	}

	// FK промежуточной к родителю
	fk := strings.TrimSpace(rel.FK)
	if fk == "" {
		// дефолт: <parent.table>_id
		fk = parent.Table + "_id"
	}

	// belongs_to из through к final (unwrapKey)
	unwrapKey := ""
	for key, r := range through.Relations {
		if r != nil && r.Type == "belongs_to" && r.GetModelRef() == final {
			unwrapKey = key
			break
		}
	}
	if unwrapKey == "" {
		return IndexRequest{}, fmt.Errorf("no belongs_to from %s to %s found",
			through.Table, final.Table)
	}

	// найти пресет конечной модели и положить указатель
	nested := final.Presets[nestedPreset]
	if nested == nil {
		return IndexRequest{}, fmt.Errorf("nested preset %q not found in model %q",
			nestedPreset, final.Table)
	}

	// синтетический пресет промежуточной модели:
	// - включаем FK (нужен для группировки/склейки на уровне родителя)
	// - одно preset-поле на final по unwrapKey с нужным nestedPreset
	belongsField := model.Field{
		Source:       unwrapKey,
		Type:         "preset",
		Alias:        unwrapKey,
		NestedPreset: nestedPreset,
	}
	belongsField.SetPresetRef(nested)

	synthetic := &model.DataPreset{
		Name: nestedPreset,
		Fields: []model.Field{
			{Source: fk, Type: "int", Alias: fk},
			belongsField,
		},
		FieldsAliasMap: &model.AliasMap{
			PathToAlias: map[string]string{unwrapKey: "t0"},
			AliasToPath: map[string]string{"t0": unwrapKey},
		},
	}

	// логическое имя промежуточной модели
	throughModelName := ""
	for name, ptr := range model.Registry {
		if ptr == through {
			throughModelName = name
			break
		}
	}
	if throughModelName == "" {
		throughModelName = through.Table
	}

	req := IndexRequest{
		Model:      throughModelName,
		Preset:     "",
		PresetObj:  synthetic,
		Filters:    map[string]any{fk + "__in": parentIDs},
		Sorts:      buildOrderSorts(rel.Order, unwrapKey+"."),
		Offset:     0,
		Limit:      maxLimit,
		UnwrapField: unwrapKey, // просим развернуть контейнер до конечной модели
	}

	// through_where (фильтр на промежуточной)
	if strings.TrimSpace(rel.ThroughWhere) != "" {
		if key, val, ok := parseCondition(rel.ThroughWhere); ok {
			req.Filters[key] = val
		}
	}
	// where (фильтр на конечной модели)
	if strings.TrimSpace(rel.Where) != "" {
		if key, val, ok := parseCondition(rel.Where); ok {
			req.Filters[unwrapKey+"."+key] = val
		}
	}	
	return req, nil
}

func makeSyntheticPreset(orig *model.DataPreset, fk string) *model.DataPreset {
    // Копируем оригинальные поля
    fields := make([]model.Field, 0, len(orig.Fields)+1)
    
    // Добавляем FK первым полем
    fields = append(fields, model.Field{
        Source: fk,
        Type:   "int",
        Alias:  fk,
    })

    // Копируем остальные
    fields = append(fields, orig.Fields...)

    return &model.DataPreset{
				Name: orig.Name, 
        Fields: fields,
				FieldsAliasMap: orig.FieldsAliasMap, // сохраняем карту алиасов
    }
}