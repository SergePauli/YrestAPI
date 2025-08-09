package config

// Package config provides configuration loading for the application.
import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	PostgresDSN string
	RedisAddr   string
	ModelsDir  string
}

func LoadConfig() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		PostgresDSN: getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/app?sslmode=disable"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		ModelsDir:   getEnv("MODELS_DIR", "./db"),
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
