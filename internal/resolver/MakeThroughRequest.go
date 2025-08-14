package resolver

import (
	"YrestAPI/internal/model"
	"fmt"
)

// internal/resolver/through.go
func MakeThroughChildRequest(
	parent *model.Model,        // родительская модель (напр. Person)
	rel    *model.ModelRelation, // связь из родителя (напр. email/phone)
	nestedPreset string,         // пресет конечной модели (напр. "item")
	parentIDs []any,             // список PK родителя из главного селекта
) (IndexRequest, error) {

	through := rel.GetThroughRef()      // промежуточная модель (PersonContact)
	final   := rel.GetModelRef()        // конечная модель (Contact)
	if through == nil || final == nil {
		return IndexRequest{}, fmt.Errorf("through or final model is nil")
	}

	// FK промежуточной к родителю (если пусто — <parent.table>_id)
	fk := rel.FK
	

	// Ключ связи в промежуточной, указывающей на конечную модель (обычно "contact")
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

	// Найдём пресет конечной модели и положим указатель в _PresetRef
	nested := final.Presets[nestedPreset]
	if nested == nil {
		return IndexRequest{}, fmt.Errorf("nested preset '%s' not found in model '%s'",
			nestedPreset, final.Table)
	}

	// Синтетический пресет промежуточной:
	//  - включаем FK для группировки на уровне родителя
	//  - включаем только один preset-поле: belongs_to -> final (с нужным nestedPreset)
	belongsField := model.Field{
		Source:       unwrapKey,
		Type:         "preset",
		Alias:        unwrapKey,
		NestedPreset: nestedPreset,		
	}
	belongsField.SetPresetRef(nested) // важно, чтобы не искать по имени позже
	synthetic := &model.DataPreset{		
		Fields: []model.Field{
			{Source: fk, Type: "int", Alias: fk},
			belongsField, // поле с belongs_to на конечную модель
		},
	}

	

	// Логическое имя промежуточной модели (ключ в Registry)
	throughModelName := ""
	for name, ptr := range model.Registry {
		if ptr == through { throughModelName = name; break }
	}
	if throughModelName == "" {
		throughModelName = through.Table // запасной вариант
	}

	// Обычный вызов Resolver: модель = промежуточная, пресет = синтетический,
	// фильтр простой: fk__in = parentIDs, и просим развернуть unwrapKey.
	req := IndexRequest{
		Model:     throughModelName,
		Preset: "",
		PresetObj: synthetic,
		Filters:   map[string]any{fk + "__in": parentIDs},
		Sorts:     nil,
		Offset:    0,
		Limit:     maxLimit,
		UnwrapField: unwrapKey, // например "contact"
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
        Fields: fields,
    }
}