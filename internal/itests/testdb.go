package itests

import (
	"YrestAPI/internal"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DeriveTestDSN: меняем имя БД на "test" и готовим admin-DSN к "postgres"
func DeriveTestDSN(baseDSN string) (testDSN, adminDSN, testDBName string, err error) {
	// safety: только URL-формат вида postgres://user:pass@host:port/db?...
	u, e := url.Parse(baseDSN)
	if e != nil {
		return "", "", "", fmt.Errorf("parse DSN: %w", e)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", "", "", errors.New("only URL DSN supported: postgres://...")
	}

	// safety: не позволяем удалённые хосты для тестов по умолчанию
	if host := u.Hostname(); host != "localhost" && host != "127.0.0.1" {
		return "", "", "", fmt.Errorf("refuse non-local host for tests: %s", host)
	}

	// заменяем path на /test
	u.Path = "/test"
	testDBName = "test"
	testDSN = u.String()

	// adminDSN -> та же конфигурация, но db=postgres
	u.Path = "/postgres"
	adminDSN = u.String()

	return testDSN, adminDSN, testDBName, nil
}

func CreateTestDatabase(adminDSN, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	var exists bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, dbName,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.ExecContext(ctx, `CREATE DATABASE `+pqIdent(dbName))
	return err
}

func DropTestDatabase(adminDSN, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	// убиваем активные коннекты к тестовой БД
	_, _ = db.ExecContext(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()
	`, dbName)

	_, err = db.ExecContext(ctx, `DROP DATABASE IF EXISTS `+pqIdent(dbName))
	return err
}

// простейшая экранизация идентификаторов (имени базы)
func pqIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
func applyMigrationsFromDir(testDSN string) error {
	root, err := internal.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("repo root not found: %w", err)
	}
	path := filepath.Join(root, "migrations")

	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("abs migrations: %w", err)
	}
	// golang-migrate с file:// требует абсолютный путь и прямые слэши
	src := "file://" + filepath.ToSlash(abs)

	m, err := migrate.New(src, testDSN)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// Хук, который ты позовёшь из TestMain: создаёт/дропает БД.
// initFunc обычно — твой db.InitPostgres(testDSN)
func SetupAndTeardownTestDB(baseDSN string, initFunc func(string) error) (teardown func() error, err error) {
	testDSN, adminDSN, testDB, err := DeriveTestDSN(baseDSN)
	if err != nil {
		return nil, err
	}

	// ещё одна защита от запуска в проде
	if os.Getenv("APP_ENV") == "production" {
		return nil, errors.New("APP_ENV=production — aborting tests")
	}

	if err := CreateTestDatabase(adminDSN, testDB); err != nil {
		return nil, fmt.Errorf("create DB %q: %w (PostgresDSN from config.go -> %s). Ensure Postgres is running or set POSTGRES_DSN", testDB, err, redactDSN(baseDSN))
	}
	log.Printf("test DB %q created", testDB)
	// миграции прямо сейчас, до любых тестов
	if err := applyMigrationsFromDir(testDSN); err != nil {
		_ = DropTestDatabase(adminDSN, testDB)
		return nil, err
	}
	log.Printf("migrations applied to test DB")
	if initFunc != nil {
		if err := initFunc(testDSN); err != nil {
			_ = DropTestDatabase(adminDSN, testDB)
			return nil, fmt.Errorf("InitPostgres failed: %w (PostgresDSN from config.go -> %s). Ensure Postgres is running or set POSTGRES_DSN", err, redactDSN(baseDSN))
		}
	}

	teardown = func() error {
		return DropTestDatabase(adminDSN, testDB)
	}
	log.Printf("teardown function ready to drop test DB %q", testDB)	
	return teardown, nil
}

func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	username := u.User.Username()
	if username == "" {
		return dsn
	}
	u.User = url.UserPassword(username, "******")
	return u.String()
}
