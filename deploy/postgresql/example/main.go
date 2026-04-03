package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	host := envOrDefault("PGHOST", "10.255.254.22")
	port := envOrDefault("PGPORT", "30432")
	user := envOrDefault("PGUSER", "upmdb")
	password := envOrDefault("PGPASSWORD", "upmdb")
	database := envOrDefault("PGDATABASE", "upmdb")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, database,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sample_items (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		log.Fatalf("create table failed: %v", err)
	}

	if _, err := conn.Exec(ctx, `
		INSERT INTO sample_items (name)
		VALUES ($1), ($2)
	`, "alpha", "beta"); err != nil {
		log.Fatalf("insert failed: %v", err)
	}

	rows, err := conn.Query(ctx, `
		SELECT id, name, created_at
		FROM sample_items
		ORDER BY id
	`)
	if err != nil {
		log.Fatalf("select failed: %v", err)
	}
	defer rows.Close()

	fmt.Println("sample_items rows:")
	for rows.Next() {
		var (
			id        int64
			name      string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &name, &createdAt); err != nil {
			log.Fatalf("scan failed: %v", err)
		}
		fmt.Printf("id=%d name=%s created_at=%s\n", id, name, createdAt.Format(time.RFC3339))
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("rows error: %v", err)
	}
}
