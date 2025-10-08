// aliasmap_build_complex_test.go
package model

import (
	"reflect"
	"sort"
	"testing"
)

// --- helpers ---

func newModel(name string) *Model {
	return &Model{
		Name:      name,
		Relations: map[string]*ModelRelation{},
		Presets:   map[string]*DataPreset{},
	}
}

func link(m *Model, relName, relType string, target *Model, reentrant bool, maxDepth int) {
	m.Relations[relName] = &ModelRelation{
		Type:       relType,
		Model:      target.Name,
		_ModelRef:  target,
		Reentrant:  reentrant,
		MaxDepth:   maxDepth,
	}
}

// preset.FieldsAliasMap с детерминированными алиасами
func presetWithAliases(pathsToAliases map[string]string) *DataPreset {
	aliasToPath := make(map[string]string, len(pathsToAliases))
	for p, a := range pathsToAliases {
		aliasToPath[a] = p
	}
	// просто для отладки
	paths := make([]string, 0, len(pathsToAliases))
	for p := range pathsToAliases {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	return &DataPreset{
		FieldsAliasMap: &AliasMap{
			PathToAlias: pathsToAliases,
			AliasToPath: aliasToPath,
		},
		//FieldPaths: paths,
	}
}

// --- test ---

// Максимально сложный кейс: рекурсивный фильтр + сортировка по полю вложенного пресета.
// Ожидаем точное совпадение AliasMap.
func TestBuildAliasMap_ComplexRecursiveFilterAndNestedSort_ExactMatch(t *testing.T) {
	// Модели
	contract := newModel("Contract")
	contragent := newModel("Contragent")
	address := newModel("Address")

	// Связи:
	// Contract --has_one--> Contragent  (разрешаем ре-энтри, т.к. путь возвращается в Contragent)
	link(contract, "contragent", "has_one", contragent, true, 3)
	// Contragent --has_many--> Contract (разрешаем ре-энтри для возврата в Contract, глубина = 3 посещения)
	link(contragent, "contracts", "has_many", contract, true, 3)
	// Contragent --belongs_to--> Address (обычный шаг, без ре-энтри)
	link(contragent, "address", "belongs_to", address, false, 0)

	// Базовый пресет у Contract: заранее зафиксируем алиасы для первых путей
	// t0: contragent
	// t1: contragent.contracts
	preset := presetWithAliases(map[string]string{
		"contragent":             "t0",
		"contragent.contracts":   "t1",
	})

	// Фильтр по рекурсивному пути: Contract → Contragent → Contracts → Contragent → Contracts
	filters := map[string]interface{}{
		"contragent.contracts.contragent.contracts.id__in": []int{1, 2, 3},
	}

	// Сортировка по полю вложенного пресета Contragent → Address.city
	sorts := []string{
		"contragent.address.city ASC",
	}

	am, err := BuildAliasMap(contract, preset, filters, sorts)
	if err != nil {
		t.Fatalf("BuildAliasMap error: %v", err)
	}

	// Ожидаемая карта:
	// - t0: contragent                      (из пресета)
	// - t1: contragent.contracts            (из пресета)
	// - t2: contragent.address              (новый путь из сортировки)
	// - t3: contragent.contracts.contragent (новый префикс из рекурсивного фильтра)
	// - t4: contragent.contracts.contragent.contracts (конечный путь рекурсивного фильтра)
	wantPathToAlias := map[string]string{
		"contragent":                                     "t0",
		"contragent.contracts":                           "t1",
		"contragent.address":                             "t2",
		"contragent.contracts.contragent":                "t3",
		"contragent.contracts.contragent.contracts":      "t4",
	}

	// Проверка точного совпадения PathToAlias (ровно эти и только эти пути)
	if !reflect.DeepEqual(am.PathToAlias, wantPathToAlias) {
		t.Fatalf("PathToAlias mismatch:\n got: %#v\nwant: %#v", am.PathToAlias, wantPathToAlias)
	}

	// И зеркальная проверка AliasToPath
	wantAliasToPath := map[string]string{
		"t0": "contragent",
		"t1": "contragent.contracts",
		"t2": "contragent.address",
		"t3": "contragent.contracts.contragent",
		"t4": "contragent.contracts.contragent.contracts",
	}
	if !reflect.DeepEqual(am.AliasToPath, wantAliasToPath) {
		t.Fatalf("AliasToPath mismatch:\n got: %#v\nwant: %#v", am.AliasToPath, wantAliasToPath)
	}
}
func TestBuildAliasMap_NoPresetNoFiltersNoSorts_Empty(t *testing.T) {
	// Мини-граф: он нам тут не важен, но пусть будет валидный root
	contract := newModel("Contract")
	contragent := newModel("Contragent")
	link(contract, "contragent", "has_one", contragent, false, 0)

	am, err := BuildAliasMap(contract, nil, nil, nil)
	if err != nil {
		t.Fatalf("BuildAliasMap error: %v", err)
	}
	if am == nil {
		t.Fatalf("BuildAliasMap returned nil AliasMap")
	}
	if len(am.PathToAlias) != 0 || len(am.AliasToPath) != 0 {
		t.Fatalf("expected empty AliasMap, got PathToAlias=%v, AliasToPath=%v",
			am.PathToAlias, am.AliasToPath)
	}
}

func TestBuildPresetAliasMapsForModel_PersonCard_IncludesPersonNameNaming(t *testing.T) {
	// Модели: Person → person_name: has_one → PersonName → naming: belongs_to → Naming
	person := newModel("Person")
	personName := newModel("PersonName")
	naming := newModel("Naming")

	link(person, "person_name", "has_one", personName, false, 0)
	link(personName, "naming", "belongs_to", naming, false, 0)

	// Пресеты:
	// Person.card содержит nested-поле person_name (preset=edit)
	person.Presets["card"] = &DataPreset{
		Name: "card",
		Fields: []Field{
			{Source: "id", Type: "int"},
			{Source: "person_name", Type: "preset", NestedPreset: "edit", Internal: true},
		},
	}
	// В Person.item это не нужно для теста, но в реальном конфиге есть — опускаем.

	// У PersonName.edit есть nested-поле naming (preset=head)
	personName.Presets["edit"] = &DataPreset{
		Name: "edit",
		Fields: []Field{
			{Source: "id", Type: "int", Alias: "id"},
			{Source: "naming", Type: "preset", NestedPreset: "head", Alias: "naming"},
		},
	}
	// Naming.head без вложенных пресетов — нам достаточно
	naming.Presets["head"] = &DataPreset{
		Name: "head",
		Fields: []Field{
			{Source: "id", Type: "int"},
		},
	}

	// Зарегистрируем модели (если Registry глобальный)
	Registry = map[string]*Model{
		"Person":     person,
		"PersonName": personName,
		"Naming":     naming,
	}

	// Запуск целевой функции
	if err := BuildPresetAliasMapsForModel(person); err != nil {
		t.Fatalf("BuildPresetAliasMapsForModel(Person): %v", err)
	}

	// Проверяем AliasMap пресета Person.card
	dp := person.Presets["card"]
	if dp == nil || dp.FieldsAliasMap == nil {
		t.Fatalf("FieldsAliasMap not built for Person.card")
	}

	got := dp.FieldsAliasMap.PathToAlias

	// Ожидаем ровно эти пути: базовый и вложенный belongs_to
	want := map[string]string{
		"person_name":          "t0", // порядок по глубине: сначала t0
		"person_name.naming":   "t1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PathToAlias mismatch\n got: %#v\nwant: %#v", got, want)
	}

	// Симметричная проверка обратной проекции
	wantRev := map[string]string{
		"t0": "person_name",
		"t1": "person_name.naming",
	}
	if !reflect.DeepEqual(dp.FieldsAliasMap.AliasToPath, wantRev) {
		t.Fatalf("AliasToPath mismatch\n got: %#v\nwant: %#v", dp.FieldsAliasMap.AliasToPath, wantRev)
	}
}