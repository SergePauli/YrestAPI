package config

// Package config provides configuration loading for the application.
import (
	"YrestAPI/internal"
	"YrestAPI/internal/logger"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	PostgresDSN string
	ModelsDir   string
	Locale      string
	AliasCache  AliasCacheConfig
	CORS        CORSConfig
	Auth        AuthConfig
}

type AliasCacheConfig struct {
	MaxBytes int64
}

type CORSConfig struct {
	AllowOrigin string
	AllowCredentials bool
}

type AuthConfig struct {
	Enabled bool
	JWT     JWTConfig
}

type JWTConfig struct {
	ValidationType string
	Issuer         string
	Audience       string
	HMACSecret     string
	PublicKeyPEM   string
	PublicKeyPath  string
	ClockSkewSec   int64
}

func LoadConfig() *Config {
	// ищем корень проекта (там где go.mod)
	root, _ := internal.FindRepoRoot()

	// пробуем загрузить .env из корня
	_ = godotenv.Load(filepath.Join(root, ".env"))

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		PostgresDSN: getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/app?sslmode=disable"),
		ModelsDir:   getEnv("MODELS_DIR", "./db"),
		Locale:      getEnv("LOCALE", "en"),
		AliasCache: AliasCacheConfig{
			MaxBytes: getEnvInt64("ALIAS_CACHE_MAX_BYTES", 0),
		},
		CORS: CORSConfig{
			AllowOrigin: getEnv("CORS_ALLOW_ORIGIN", "*"),
			AllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", false),
		},
		Auth: AuthConfig{
			Enabled: getEnvBool("AUTH_ENABLED", false),
			JWT: JWTConfig{
				ValidationType: strings.ToUpper(getEnv("AUTH_JWT_VALIDATION_TYPE", "HS256")),
				Issuer:         getEnvOptional("AUTH_JWT_ISSUER"),
				Audience:       getEnvOptional("AUTH_JWT_AUDIENCE"),
				HMACSecret:     getEnvOptional("AUTH_JWT_HMAC_SECRET"),
				PublicKeyPEM:   getEnvOptional("AUTH_JWT_PUBLIC_KEY"),
				PublicKeyPath:  getEnvOptional("AUTH_JWT_PUBLIC_KEY_PATH"),
				ClockSkewSec:   getEnvInt64("AUTH_JWT_CLOCK_SKEW_SEC", 60),
			},
		},
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	logger.Warn("env_default", map[string]any{
		"key":      key,
		"fallback": fallback,
	})
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		logger.Warn("env_invalid_bool", map[string]any{
			"key":      key,
			"value":    value,
			"fallback": fallback,
		})
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		logger.Warn("env_invalid_int", map[string]any{
			"key":      key,
			"value":    value,
			"fallback": fallback,
		})
		return fallback
	}
	return parsed
}

func getEnvOptional(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
