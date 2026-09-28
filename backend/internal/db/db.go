package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the repository used to persist Gmail sync state and email rows.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store bound to a database pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// clean removes NUL bytes and invalid UTF-8 so Postgres TEXT columns accept the value.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.ToValidUTF8(s, "")
}

// Connect opens a PostgreSQL pool and verifies it is usable.
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, errors.New("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return pool, nil
}
