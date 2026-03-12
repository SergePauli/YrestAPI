package graphqlimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromPath_UpdatesExistingModelsWithPresets(t *testing.T) {
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "db")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("mkdir models dir: %v", err)
	}

	userModel := `table: users
relations:
  posts:
    type: has_many
    model: Post
    fk: author_id
presets:
  item:
    fields:
      - source: id
        type: UUID
      - source: email
        type: string
`
	postModel := `table: posts
presets:
  item:
    fields:
      - source: id
        type: UUID
      - source: title
        type: string
`
	if err := os.WriteFile(filepath.Join(modelsDir, "User.yml"), []byte(userModel), 0o644); err != nil {
		t.Fatalf("write user model: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "Post.yml"), []byte(postModel), 0o644); err != nil {
		t.Fatalf("write post model: %v", err)
	}

	queryPath := filepath.Join(dir, "query.graphql")
	query := `query GetUserCard {
  user {
    id
    displayEmail: email
    posts {
      id
      title
    }
  }
}`
	if err := os.WriteFile(queryPath, []byte(query), 0o644); err != nil {
		t.Fatalf("write query: %v", err)
	}

	res, err := ImportFromPath(queryPath, ImportOptions{ModelsDir: modelsDir})
	if err != nil {
		t.Fatalf("ImportFromPath: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", res.Warnings)
	}

	byFile := map[string]string{}
	for _, f := range res.Files {
		byFile[f.FileName] = string(f.Content)
	}

	rootSel := selection{
		Name: "user",
		Selections: []selection{
			{Name: "id"},
			{Name: "email", Alias: "displayEmail"},
			{Name: "posts", Selections: []selection{{Name: "id"}, {Name: "title"}}},
		},
	}
	rootPreset := presetNameForSelection(operation{Name: "GetUserCard"}, rootSel, nil)
	postsPreset := presetNameForSelection(operation{Name: "GetUserCard"}, rootSel.Selections[2], []string{"posts"})

	userOut := byFile["User.yml"]
	if !strings.Contains(userOut, rootPreset+":") {
		t.Fatalf("expected root preset %q in User.yml, got:\n%s", rootPreset, userOut)
	}
	if !strings.Contains(userOut, "alias: displayEmail") {
		t.Fatalf("expected GraphQL alias mapped to YAML alias, got:\n%s", userOut)
	}
	if !strings.Contains(userOut, "preset: "+postsPreset) {
		t.Fatalf("expected nested preset %q reference in User.yml, got:\n%s", postsPreset, userOut)
	}

	postOut := byFile["Post.yml"]
	if !strings.Contains(postOut, postsPreset+":") {
		t.Fatalf("expected nested preset %q in Post.yml, got:\n%s", postsPreset, postOut)
	}
}

func TestImportFromPath_WarnsOnMissingRelation(t *testing.T) {
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "db")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("mkdir models dir: %v", err)
	}
	userModel := `table: users
presets:
  item:
    fields:
      - source: id
        type: UUID
      - source: email
        type: string
`
	if err := os.WriteFile(filepath.Join(modelsDir, "User.yml"), []byte(userModel), 0o644); err != nil {
		t.Fatalf("write user model: %v", err)
	}
	queryPath := filepath.Join(dir, "query.graphql")
	query := `query GetUserCard { user { id posts { id } } }`
	if err := os.WriteFile(queryPath, []byte(query), 0o644); err != nil {
		t.Fatalf("write query: %v", err)
	}

	res, err := ImportFromPath(queryPath, ImportOptions{ModelsDir: modelsDir})
	if err != nil {
		t.Fatalf("ImportFromPath: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected warnings for missing relation")
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), `relation not found`) {
		t.Fatalf("expected missing relation warning, got: %#v", res.Warnings)
	}
}

func TestImportFromPath_SkipsExistingPresetWithoutReplace(t *testing.T) {
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "db")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("mkdir models dir: %v", err)
	}
	rootSel := selection{Name: "user", Selections: []selection{{Name: "id"}}}
	presetName := presetNameForSelection(operation{Name: "GetUserCard"}, rootSel, nil)
	userModel := `table: users
presets:
  item:
    fields:
      - source: id
        type: UUID
  ` + presetName + `:
    fields:
      - source: email
        type: string
`
	if err := os.WriteFile(filepath.Join(modelsDir, "User.yml"), []byte(userModel), 0o644); err != nil {
		t.Fatalf("write user model: %v", err)
	}
	queryPath := filepath.Join(dir, "query.graphql")
	query := `query GetUserCard { user { id } }`
	if err := os.WriteFile(queryPath, []byte(query), 0o644); err != nil {
		t.Fatalf("write query: %v", err)
	}

	res, err := ImportFromPath(queryPath, ImportOptions{ModelsDir: modelsDir})
	if err != nil {
		t.Fatalf("ImportFromPath: %v", err)
	}
	out := string(res.Files[0].Content)
	if !strings.Contains(out, "source: email") {
		t.Fatalf("expected existing preset to remain unchanged, got:\n%s", out)
	}
}
