package prismaimport

import (
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
