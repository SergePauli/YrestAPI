package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Хелпер: запись файла
func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

// Хелпер: получить модель из Registry
func getModel(t *testing.T, name string) *Model {
	t.Helper()
	m, ok := Registry[name]
	if !ok || m == nil {
		t.Fatalf("model %q not found in Registry", name)
	}
	return m
}

func TestLoadModelsFromDir_Inheritance_SingleModel(t *testing.T) {
	dir := t.TempDir()

	// YAML c extends (без заранее влитых полей!)
	ownershipYAML := `
table: ownerships
presets:
  base:
    fields:
      - source: id
        type: int
      - source: name
        type: string
      - source: okopf
        type: string

  item:
    extends: base
    fields:
      - source: full_name
        type: string
`
	write(t, dir, "Ownership.yml", ownershipYAML)

	// Чистим реестр между тестами (если глобальный)
	Registry = map[string]*Model{}

	if err := LoadModelsFromDir(dir); err != nil {
		t.Fatalf("LoadModelsFromDir: %v", err)
	}

	m := getModel(t, "Ownership")

	// Проверим, что поля item получили наследование из base + свои поля
	item, ok := m.Presets["item"]
	if !ok {
		t.Fatalf("preset 'item' not found")
	}

	// Ожидаем порядок: id, name, okopf, full_name
	want := []Field{
		{Source: "id", Type: "int"},
		{Source: "name", Type: "string"},
		{Source: "okopf", Type: "string"},
		{Source: "full_name", Type: "string"},
	}

	if !fieldsEqual(item.Fields, want) {
		t.Fatalf("item.Fields mismatch.\nwant: %#v\ngot:  %#v", want, item.Fields)
	}
}

func TestLoadModelsFromDir_Inheritance_CycleError(t *testing.T) {
	dir := t.TempDir()

	// Два пресета, образующих цикл
	cyclicYAML := `
table: cyclics
  presets:
    A:
      extends: B
      fields:
        - source: a
          type: string
    B:
      extends: A
      fields:
        - source: b
          type: string
`
	write(t, dir, "Cyclic.yml", cyclicYAML)

	Registry = map[string]*Model{}

	if err := LoadModelsFromDir(dir); err == nil {
		t.Fatalf("expected error due to cyclic extends, got nil")
	}
}

// Сравнение списков полей с учётом только значимых для теста атрибутов.
// Если есть служебные поля/ссылки — игнорим их.
func fieldsEqual(got, want []Field) bool {
	type view struct {
		Source string
		Type   string
		Alias  string
		// добавьте сюда, если хотите сравнивать больше
	}
	var (
		gg = make([]view, len(got))
		ww = make([]view, len(want))
	)
	for i := range got {
		gg[i] = view{Source: got[i].Source, Type: got[i].Type, Alias: got[i].Alias}
	}
	for i := range want {
		ww[i] = view{Source: want[i].Source, Type: want[i].Type, Alias: want[i].Alias}
	}
	return reflect.DeepEqual(gg, ww)
}