package model

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
// keyOf как в резолвере: alias приоритетнее, иначе source
func keyOfField(f Field) string {
	if s := strings.TrimSpace(f.Alias); s != "" {
		return s
	}
	return f.Source
}

func keys(fields []Field) []string {
	out := make([]string, len(fields))
	for i := range fields {
		out[i] = keyOfField(fields[i])
	}
	return out
}

// --- tests ---

func TestLoadModelsFromDir_Inheritance_MultipleParents_OrderAndOverride(t *testing.T) {
	dir := t.TempDir()

	yaml := `
table: ownerships
presets:
  base:
    fields:
      - source: id
        type: int
      - source: name
        type: string
        alias: name
      - source: okopf
        type: string
        alias: okopf

  head:
    fields:
      # переопределим name (с тем же alias) и добавим head_only
      - source: full_name
        type: string
        alias: name
      - source: head_only
        type: string
        alias: head_only

  item:
    extends: base, head
    fields:
      # локально переопределим okopf (позиция из base должна сохраниться)
      - source: okopf
        type: int
        alias: okopf
      # и добавим своё поле
      - source: item_only
        type: string
        alias: item_only
`
	write(t, dir, "Ownership.yml", yaml)
	Registry = map[string]*Model{}

	if err := LoadModelsFromDir(dir); err != nil {
		t.Fatalf("LoadModelsFromDir: %v", err)
	}
	m := getModel(t, "Ownership")

	item := m.Presets["item"]
	if item == nil {
		t.Fatalf("preset 'item' not found")
	}

	gotKeys := keys(item.Fields)
	wantKeys := []string{"id", "name", "okopf", "head_only", "item_only"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("field order mismatch\n got: %v\nwant: %v", gotKeys, wantKeys)
	}

	// Проверим, что name взят из head (source=full_name)
	var name Field
	for _, f := range item.Fields {
		if keyOfField(f) == "name" {
			name = f
			break
		}
	}
	if name.Source != "full_name" {
		t.Fatalf("name override failed: got source=%q, want %q", name.Source, "full_name")
	}

	// Проверим, что okopf переопределён локально (type=int) и остался на позиции из base (индекс 2)
	if len(item.Fields) < 3 || keyOfField(item.Fields[2]) != "okopf" {
		t.Fatalf("okopf position not preserved at index 2")
	}
	var okopf Field
	for _, f := range item.Fields {
		if keyOfField(f) == "okopf" {
			okopf = f
			break
		}
	}
	if okopf.Type != "int" {
		t.Fatalf("okopf override failed: type=%q, want %q", okopf.Type, "int")
	}
}

func TestLoadModelsFromDir_Inheritance_MultipleParents_DuplicatesAndSpaces(t *testing.T) {
	dir := t.TempDir()

	yaml := `
table: ownerships
presets:
  base:
    fields:
      - source: id
        type: int
      - source: name
        type: string
        alias: name

  head:
    fields:
      - source: full_name
        type: string
        alias: name
      - source: x
        type: string
        alias: head_only

  item:
    extends: "  base ,  base , head  "   # дубли и пробелы
    fields:
      - source: flag
        type: string
        alias: flag
`
	write(t, dir, "Ownership.yml", yaml)
	Registry = map[string]*Model{}

	if err := LoadModelsFromDir(dir); err != nil {
		t.Fatalf("LoadModelsFromDir: %v", err)
	}
	m := getModel(t, "Ownership")

	item := m.Presets["item"]
	if item == nil {
		t.Fatalf("preset 'item' not found")
	}
	got := keys(item.Fields)
	// ожидаем, что base учтён один раз, затем head, затем локальные
	want := []string{"id", "name", "head_only", "flag"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected fields\n got: %v\nwant: %v", got, want)
	}
}

func TestLoadModelsFromDir_Inheritance_MultipleParents_MissingParentError(t *testing.T) {
	dir := t.TempDir()

	yaml := `
table: ownerships
presets:
  base:
    fields:
      - source: id
        type: int
  item:
    extends: base, missing_parent
    fields:
      - source: x
        type: string
`
	write(t, dir, "Ownership.yml", yaml)
	Registry = map[string]*Model{}

	if err := LoadModelsFromDir(dir); err == nil {
		t.Fatalf("expected error for missing parent in extends, got nil")
	}
}

func TestLoadModelsFromDir_Inheritance_MultipleParents_Cycle(t *testing.T) {
	dir := t.TempDir()

	yaml := `
table: cyclics
presets:
  A:
    extends: B, C
    fields:
      - source: a
        type: string
  B:
    extends: C
    fields:
      - source: b
        type: string
  C:
    extends: A  # цикл через несколько родителей
    fields:
      - source: c
        type: string
`
	write(t, dir, "Cyclic.yml", yaml)
	Registry = map[string]*Model{}

	if err := LoadModelsFromDir(dir); err == nil {
		t.Fatalf("expected cyclic extends error, got nil")
	}
}