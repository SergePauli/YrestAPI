package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveModelsDir_UsesConfiguredDirWhenItHasModels(t *testing.T) {
	tmp := t.TempDir()
	dbDir := filepath.Join(tmp, "db")
	testDir := filepath.Join(tmp, "test_db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir db: %v", err)
	}
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatalf("mkdir test_db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dbDir, "Area.yml"), []byte("table: areas\n"), 0o644); err != nil {
		t.Fatalf("write db model: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "Area.yml"), []byte("table: areas\n"), 0o644); err != nil {
		t.Fatalf("write test model: %v", err)
	}

	prevWD, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })
	prevEnv, hadEnv := os.LookupEnv("MODELS_DIR")
	_ = os.Unsetenv("MODELS_DIR")
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("MODELS_DIR", prevEnv)
		} else {
			_ = os.Unsetenv("MODELS_DIR")
		}
	})

	got := resolveModelsDir("./db")
	if filepath.Clean(got) != filepath.Clean("./db") {
		t.Fatalf("resolveModelsDir=%q, want %q", got, "./db")
	}
}

func TestResolveModelsDir_FallsBackToTestDBWhenDefaultDBEmpty(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "db"), 0o755); err != nil {
		t.Fatalf("mkdir db: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "test_db"), 0o755); err != nil {
		t.Fatalf("mkdir test_db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "test_db", "Area.yml"), []byte("table: areas\n"), 0o644); err != nil {
		t.Fatalf("write test model: %v", err)
	}

	prevWD, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })
	prevEnv, hadEnv := os.LookupEnv("MODELS_DIR")
	_ = os.Unsetenv("MODELS_DIR")
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("MODELS_DIR", prevEnv)
		} else {
			_ = os.Unsetenv("MODELS_DIR")
		}
	})

	got := resolveModelsDir("./db")
	if filepath.Clean(got) != filepath.Clean("./test_db") {
		t.Fatalf("resolveModelsDir=%q, want %q", got, "./test_db")
	}
}

func TestResolveModelsDir_DoesNotFallbackWhenExplicitlyConfigured(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "custom_models"), 0o755); err != nil {
		t.Fatalf("mkdir custom_models: %v", err)
	}

	prevWD, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })
	t.Setenv("MODELS_DIR", "./custom_models")

	got := resolveModelsDir("./custom_models")
	if filepath.Clean(got) != filepath.Clean("./custom_models") {
		t.Fatalf("resolveModelsDir=%q, want %q", got, "./custom_models")
	}
}
