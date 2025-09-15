// file: apply_all_formatters_test.go
package resolver // ← если функция applyAllFormatters в другом пакете, поменяй

import (
	"reflect"
	"strings"
	"testing"

	"YrestAPI/internal/model" // ← подставь реальный импорт пакета с моделями
)

// ---------- ТЕСТ 1: порядок вычисления + through для email/phone ----------

func TestApplyAllFormatters_Person_Edit_LogLike(t *testing.T) {
	// Contact; PersonContact -> belongs_to Contact
	contact := &model.Model{Table: "contacts", Relations: map[string]*model.ModelRelation{}}
	personContact := &model.Model{Table: "person_contacts", Relations: map[string]*model.ModelRelation{}}
	{
		r := &model.ModelRelation{Type: "belongs_to"}
		r.SetModelRef(contact) // setter
		personContact.Relations["contact"] = r
	}

	// Naming; PersonName -> belongs_to Naming
	naming := &model.Model{Table: "namings", Relations: map[string]*model.ModelRelation{}}
	personName := &model.Model{Table: "person_names", Relations: map[string]*model.ModelRelation{}}
	{
		r := &model.ModelRelation{Type: "belongs_to"}
		r.SetModelRef(naming) // setter
		personName.Relations["naming"] = r
	}

	// Person relations
	person := &model.Model{Table: "people", Relations: map[string]*model.ModelRelation{}}
	{
		r := &model.ModelRelation{Type: "belongs_to"}
		r.SetModelRef(naming) // setter
		person.Relations["naming"] = r
	}
	{
		r := &model.ModelRelation{Type: "belongs_to"}
		r.SetModelRef(personName) // setter
		person.Relations["person_name"] = r
	}
	{
		r := &model.ModelRelation{Type: "has_one", Through: "person_contacts"}
		r.SetThroughRef(personContact) // setter
		r.SetModelRef(contact)         // setter
		person.Relations["email"] = r
	}
	{
		r := &model.ModelRelation{Type: "has_one", Through: "person_contacts"}
		r.SetThroughRef(personContact) // setter
		r.SetModelRef(contact)         // setter
		person.Relations["phone"] = r
	}

	// preset "naming.edit"
	namingEdit := &model.DataPreset{
		Name: "edit",
		Fields: []model.Field{
			{Type: "string", Source: "surname"},
			{Type: "string", Source: "name"},
			{Type: "string", Source: "patrname"},
		},
	}

	// Поле preset для naming + привяжем preset через setter
	fNaming := model.Field{Type: "preset", Source: "naming", Alias: "naming", NestedPreset: "edit"}
	fNaming.SetPresetRef(namingEdit) // setter

	// Корневой пресет person.edit
	p := &model.DataPreset{
		Name: "edit",
		Fields: []model.Field{
			fNaming,
			// name из naming.*
			{Type: "formatter", Alias: "name", Source: "{naming.surname} {naming.name}[0].{naming.patrname}[0..1]."},
			// email / phone как preset'ы (через through)
			{Type: "preset", Source: "email", Alias: "email"},
			{Type: "preset", Source: "phone", Alias: "phone"},
			// второй name из person_name.name
			{Type: "formatter", Alias: "name", Source: "{person_name.name}"},
			// full_name из person_name.naming.*
			{Type: "formatter", Alias: "full_name", Source: "{person_name.naming.surname} {person_name.naming.name} {person_name.naming.patrname}"},
		},
	}

	// Данные (по мотивам логов)
	items := []map[string]any{
		{
			"id": 2205,
			"naming": map[string]any{
				"id": 2006, "surname": "Котов", "name": "Артем", "patrname": "Игоревич",
			},
			"person_name": map[string]any{
				"id": 2203, "name": "Котов А.И.",
				"naming": map[string]any{"id": 2006, "surname": "Котов", "name": "Артем", "patrname": "Игоревич"},
			},
			"email": nil,
			"phone": map[string]any{"contact": map[string]any{"id": 3963, "type": "Phone", "value": "8-963-258-9191"}},
		},
		{
			"id": 2206,
			"naming": map[string]any{
				"id": 2007, "surname": "Журавлёв", "name": "Семен", "patrname": "",
			},
			"person_name": map[string]any{
				"id": 2204, "name": "Журавлёв С.",
				"naming": map[string]any{"id": 2007, "surname": "Журавлёв", "name": "Семен", "patrname": ""},
			},
			"email": map[string]any{"contact": map[string]any{"id": 3966, "type": "Email", "value": "sem.zh@example.com"}},
			"phone": nil,
		},
	}

	if err := applyAllFormatters(person, p, items, ""); err != nil {
		t.Fatalf("applyAllFormatters error: %v", err)
	}

	got := []map[string]string{
		{"name": items[0]["name"].(string), "full_name": items[0]["full_name"].(string)},
		{"name": items[1]["name"].(string), "full_name": items[1]["full_name"].(string)},
	}
	want := []map[string]string{
		{"name": "Котов А.И.", "full_name": "Котов Артем Игоревич"},
		{"name": "Журавлёв С.", "full_name": "Журавлёв Семен "},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// ---------- ТЕСТ 2: верхний head с {name} + through {email.value}/{phone.value} ----------

func TestApplyAllFormatters_Head_UsesName_And_Through(t *testing.T) {
	contact := &model.Model{Table: "contacts", Relations: map[string]*model.ModelRelation{}}
	personContact := &model.Model{Table: "person_contacts", Relations: map[string]*model.ModelRelation{}}
	{
		r := &model.ModelRelation{Type: "belongs_to"}
		r.SetModelRef(contact)
		personContact.Relations["contact"] = r
	}
	naming := &model.Model{Table: "namings", Relations: map[string]*model.ModelRelation{}}

	person := &model.Model{Table: "people", Relations: map[string]*model.ModelRelation{}}
	{
		r := &model.ModelRelation{Type: "belongs_to"}
		r.SetModelRef(naming)
		person.Relations["naming"] = r
	}
	{
		r := &model.ModelRelation{Type: "has_one", Through: "person_contacts"}
		r.SetThroughRef(personContact)
		r.SetModelRef(contact)
		person.Relations["email"] = r
	}
	{
		r := &model.ModelRelation{Type: "has_one", Through: "person_contacts"}
		r.SetThroughRef(personContact)
		r.SetModelRef(contact)
		person.Relations["phone"] = r
	}

	// preset naming.edit
	namingEdit := &model.DataPreset{
		Name: "edit",
		Fields: []model.Field{
			{Type: "string", Source: "surname"},
			{Type: "string", Source: "name"},
			{Type: "string", Source: "patrname"},
		},
	}
	fNaming := model.Field{Type: "preset", Source: "naming", Alias: "naming", NestedPreset: "edit"}
	fNaming.SetPresetRef(namingEdit)

	p := &model.DataPreset{
		Name: "edit",
		Fields: []model.Field{
			fNaming,
			{Type: "formatter", Alias: "name", Source: "{naming.surname} {naming.name}[0].{naming.patrname}[0..1]."},
			{Type: "preset", Source: "email", Alias: "email"},
			{Type: "preset", Source: "phone", Alias: "phone"},
			{Type: "formatter", Alias: "head", Source: "{name} {email.value} {phone.value}"},
		},
	}

	items := []map[string]any{
		{
			"id": 1,
			"naming": map[string]any{"surname": "Фролов", "name": "Илья", "patrname": "Сергеевич"},
			"email":  map[string]any{"contact": map[string]any{"value": "ilya.f@example.com"}},
			"phone":  nil,
		},
		{
			"id": 2,
			"naming": map[string]any{"surname": "Маркова", "name": "Анна", "patrname": ""},
			"email":  nil,
			"phone":  map[string]any{"contact": map[string]any{"value": "8 (914) 270-81-23"}},
		},
	}

	if err := applyAllFormatters(person, p, items, ""); err != nil {
		t.Fatalf("applyAllFormatters error: %v", err)
	}

	got := []string{items[0]["head"].(string), items[1]["head"].(string)}
	want := []string{"Фролов И.С. ilya.f@example.com ", "Маркова А..  8 (914) 270-81-23"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("head mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// ---------- ТЕСТ 3: preset formatter форматирует контейнер (belongs_to без through) ----------

func TestApplyAllFormatters_PresetFormatter_WritesToAliasName(t *testing.T) {
    // Модель Organization с пресетом "item"
    org := &model.Model{
        Table:     "organizations",				
        Relations: map[string]*model.ModelRelation{},
        Presets:   map[string]*model.DataPreset{},
    }
    itemPreset := &model.DataPreset{
        Name: "item",
        Fields: []model.Field{
            {Type: "int",    Source: "id"},
            {Type: "string", Source: "full_name"},
            {Type: "string", Source: "inn"},
        },
    }
    org.Presets["item"] = itemPreset

    // Contragent -> belongs_to Organization
    contr := &model.Model{Table: "contragents", Relations: map[string]*model.ModelRelation{}}
    {
        r := &model.ModelRelation{Type: "belongs_to", Model: "Organization"}
        r.SetModelRef(org)
        contr.Relations["organization"] = r
    }

    // Поле 1: preset organization (контейнер), рекурсивно "item"
    fOrgPreset := model.Field{Type: "preset", Source: "organization", Alias: "organization", NestedPreset: "item"}
    fOrgPreset.SetPresetRef(itemPreset)

    // Поле 2: preset organization, но alias = "name", форматтер пишет строку в name
    fOrgName := model.Field{Type: "preset", Source: "organization", Alias: "name", Formatter: "{full_name}", NestedPreset: "item"}
    fOrgName.SetPresetRef(itemPreset)

    p := &model.DataPreset{
        Name:   "edit",
        Fields: []model.Field{fOrgPreset, fOrgName},
    }
		

    items := []map[string]any{
        {"id": 7, "organization": map[string]any{"id": 5, "full_name": "ООО «Сириус»", "inn": "1234567890"}},
    }

    if err := applyAllFormatters(contr, p, items, ""); err != nil {
        t.Fatalf("applyAllFormatters error: %v", err)
    }

    // Проверяем: значение попало в alias "name"
    gotName, _ := items[0]["name"].(string)
    if strings.TrimSpace(gotName) != "ООО «Сириус»" {
        t.Fatalf("preset->formatter should write into alias 'name', got %#v", items[0]["name"])
    }

    // И контейнер 'organization' остаётся map, не строка
    if _, ok := items[0]["organization"].(map[string]any); !ok {
        t.Fatalf("'organization' container must remain an object, got %#v", items[0]["organization"])
    }
}


// ---------- ТЕСТ 4: вложенный preset с локальным formatter внутри ветки ----------

func TestApplyAllFormatters_NestedPreset_LocalFormatter(t *testing.T) {
	// naming.edit со своим локальным форматтером
	namingEdit := &model.DataPreset{
		Name: "edit",
		Fields: []model.Field{
			{Type: "string", Source: "surname"},
			{Type: "string", Source: "name"},
			{Type: "string", Source: "patrname"},
			{Type: "formatter", Alias: "head", Source: "{surname} {name}[0].{patrname}[0..1]."},
		},
	}
	naming := &model.Model{Table: "namings", Relations: map[string]*model.ModelRelation{}, 
		Presets: map[string]*model.DataPreset{"edit": namingEdit},}
	person := &model.Model{Table: "people", Relations: map[string]*model.ModelRelation{}}
	{
		r := &model.ModelRelation{Type: "belongs_to"}
		r.SetModelRef(naming)
		person.Relations["naming"] = r
	}

	fNaming := model.Field{Type: "preset", Source: "naming", Alias: "naming", NestedPreset: "edit"}
	fNaming.SetPresetRef(namingEdit)

	p := &model.DataPreset{
		Name:   "edit",
		Fields: []model.Field{fNaming},
	}

	items := []map[string]any{
		{"id": 1, "naming": map[string]any{"surname": "Соболева", "name": "Мария", "patrname": "Дмитриевна"}},
	}

	if err := applyAllFormatters(person, p, items, ""); err != nil {
		t.Fatalf("applyAllFormatters error: %v", err)
	}

	nm, _ := items[0]["naming"].(map[string]any)
	if nm == nil || items[0]["head"] != "Соболева М.Д." {
		t.Fatalf("nested formatter failed: got %#v", items[0])
	}
}