package itests

import (
	"testing"

	"YrestAPI/internal/model"
)

// Мини-проверки самых сложных моделей/связей.
// Если у тебя другой способ доступа к реестру (например, GetModel),
// просто поменяй accessor внизу.
func Test_Registry_Sanity_OnComplexRelations(t *testing.T) {	
	// Registry уже загружен в TestMain — здесь лишь сверяем несколько связей
	person := model.Registry["Person"]
	if person == nil {
		t.Fatalf("Person model missing in registry")
	}
	if rel := person.Relations["contacts"]; rel == nil || rel.Through != "PersonContact" {
		t.Fatalf("Person.contacts must go through PersonContact, got: %#v", rel)
	}
	if rel := person.Relations["projects"]; rel == nil || rel.Through != "ProjectMember" {
		t.Fatalf("Person.projects must go through ProjectMember, got: %#v", rel)
	}
	dept := model.Registry["Department"]
	if dept == nil {
		t.Fatalf("Department model missing in registry")
	}
	if p := dept.Relations["parent"]; p == nil || !p.Reentrant || p.MaxDepth == 0 {
		t.Fatalf("Department.parent must be reentrant with max_depth, got: %#v", p)
	}

}
