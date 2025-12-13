package config

// Package config provides configuration loading for the application.
import (
	"YrestAPI/internal"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)
type Config struct {
	Port        string
	PostgresDSN string
	RedisAddr   string
	ModelsDir   string
	Locale      string
}
func LoadConfig() *Config {
	// ищем корень проекта (там где go.mod)
	root, _ := internal.FindRepoRoot()
	

	// пробуем загрузить .env из корня
	_ = godotenv.Load(filepath.Join(root, ".env"))

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		PostgresDSN: getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/app?sslmode=disable"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		ModelsDir:   getEnv("MODELS_DIR", "./db"),
		Locale:      getEnv("LOCALE", "en"),
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	log.Printf("⚠️ env %s not set, using default: %s", key, fallback)
	return fallback
}
