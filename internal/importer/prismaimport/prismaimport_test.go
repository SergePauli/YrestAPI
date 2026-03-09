package prismaimport

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateFromSchema_BasicRelationsAndPresets(t *testing.T) {
	schema := `
model User {
  id    Int    @id @default(autoincrement())
  name  String
  posts Post[]
  @@map("users")
}

model Post {
  id       Int   @id @default(autoincrement())
  title    String
  authorId Int
  author   User  @relation(fields: [authorId], references: [id])
  @@map("posts")
}
`
	files, err := GenerateFromSchema(schema)
	if err != nil {
		t.Fatalf("GenerateFromSchema returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 model files, got %d", len(files))
	}

	byName := map[string]string{}
	for _, f := range files {
		byName[f.FileName] = string(f.Content)
	}

	userYAML := byName["User.yml"]
	if userYAML == "" {
		t.Fatalf("missing User.yml output")
	}
	if !strings.Contains(userYAML, "table: users") {
		t.Fatalf("expected users table mapping, got:\n%s", userYAML)
	}
	if !strings.Contains(userYAML, "type: has_many") || !strings.Contains(userYAML, "model: Post") || !strings.Contains(userYAML, "fk: authorId") {
		t.Fatalf("expected reverse has_many relation in User.yml, got:\n%s", userYAML)
	}
	if !regexp.MustCompile(`(?m)^\s+posts:\s*$`).MatchString(userYAML) {
		t.Fatalf("expected reverse has_many relation key 'posts' in User.yml, got:\n%s", userYAML)
	}
	if !strings.Contains(userYAML, "with_posts:") {
		t.Fatalf("expected helper preset with_posts in User.yml, got:\n%s", userYAML)
	}

	postYAML := byName["Post.yml"]
	if postYAML == "" {
		t.Fatalf("missing Post.yml output")
	}
	if !strings.Contains(postYAML, "type: belongs_to") || !strings.Contains(postYAML, "model: User") || !strings.Contains(postYAML, "fk: authorId") {
		t.Fatalf("expected belongs_to relation in Post.yml, got:\n%s", postYAML)
	}
	if !regexp.MustCompile(`(?m)^\s+author:\s*$`).MatchString(postYAML) {
		t.Fatalf("expected relation alias 'author' derived from authorId, got:\n%s", postYAML)
	}
	if !strings.Contains(postYAML, "full_info:") {
		t.Fatalf("expected full_info preset in Post.yml, got:\n%s", postYAML)
	}
}

func TestGenerateFromSchema_EnumFieldLocalizeAndLocaleDefaults(t *testing.T) {
	schema := `
enum Role {
  USER
  ADMIN
}

model User {
  id    String @id
  role  Role   @default(USER)
  name  String?
  @@map("users")
}
`
	res, err := GenerateResultFromSchema(schema)
	if err != nil {
		t.Fatalf("GenerateResultFromSchema returned error: %v", err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("expected 1 model file, got %d", len(res.Files))
	}
	y := string(res.Files[0].Content)
	if !strings.Contains(y, "source: role") || !strings.Contains(y, "type: int") || !strings.Contains(y, "localize: true") {
		t.Fatalf("expected enum field role as int+localize in presets, got:\n%s", y)
	}
	roleMap, ok := res.LocaleDefaults["role"]
	if !ok {
		t.Fatalf("expected locale defaults for role, got: %#v", res.LocaleDefaults)
	}
	if roleMap[0] != "USER" || roleMap[1] != "ADMIN" {
		t.Fatalf("unexpected role locale mapping: %#v", roleMap)
	}
}

func TestMergeLocaleDefaults_MergesWithoutOverwrite(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "en.yml")
	initial := "role:\n  \"0\": USER_OLD\nname:\n  \"0\": ZERO\n"
	if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial locale: %v", err)
	}
	def := map[string]map[int]string{
		"role": {0: "USER", 1: "ADMIN"},
	}
	if err := MergeLocaleDefaults(p, def); err != nil {
		t.Fatalf("MergeLocaleDefaults: %v", err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read merged locale: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "USER_OLD") {
		t.Fatalf("expected existing value not overwritten, got:\n%s", got)
	}
	if !strings.Contains(got, "\"1\": ADMIN") && !strings.Contains(got, "1: ADMIN") {
		t.Fatalf("expected added enum key 1: ADMIN, got:\n%s", got)
	}
}

func TestGenerateFromSchema_SingleCustomPKFallback(t *testing.T) {
	schema := `
model Area {
  areaUuid String @id
  name     String
  @@map("areas")
}
`
	files, err := GenerateFromSchema(schema)
	if err != nil {
		t.Fatalf("GenerateFromSchema returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 model file, got %d", len(files))
	}
	out := string(files[0].Content)
	if !strings.Contains(out, "source: areaUuid") || !strings.Contains(out, "alias: id") {
		t.Fatalf("expected id alias fallback for custom PK, got:\n%s", out)
	}
}
