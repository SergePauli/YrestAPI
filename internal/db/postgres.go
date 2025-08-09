package db

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

var Conn *pgx.Conn

func InitPostgres(dsn string) error {
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/app?sslmode=disable"
		log.Println("⚠️ Using default Postgres DSN")
	}

	var err error
	Conn, err = pgx.Connect(context.Background(), dsn)
	if err != nil {
		return fmt.Errorf("connect pgx: %w", err)
	}

	// Проверка подключения
	if err := Conn.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping pgx: %w", err)
	}

	return nil
}
