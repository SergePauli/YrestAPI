package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool


func InitPostgres(dsn string) error {
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/app?sslmode=disable"
		log.Println("⚠️ Using default Postgres DSN")
	}

	// Настраиваем конфиг пула
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse pool config: %w", err)
	}

	// Пример настройки параметров пула
	cfg.MaxConns = 20                   // максимальное число соединений
	cfg.MinConns = 2                    // минимальное число соединений
	cfg.MaxConnLifetime = time.Hour     // время жизни соединения
	cfg.MaxConnIdleTime = time.Minute*5 // время простоя до закрытия соединения

	// Создаём пул
	Pool, err = pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("connect pgx pool: %w", err)
	}

	// Проверка подключения
	if err := Pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping pgx pool: %w", err)
	}

	log.Println("✅ Postgres pool connected")
	return nil
}