package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetOrCreateUser ensures a user row exists for the given email address.
func (s *Store) GetOrCreateUser(ctx context.Context, email string) (int, error) {
	email = clean(email)
	const query = `
		INSERT INTO users (email)
		VALUES ($1)
		ON CONFLICT (email) DO NOTHING
		RETURNING id
	`

	var id int
	err := s.pool.QueryRow(ctx, query, email).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("creating user %s: %w", email, err)
	}

	if err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		return 0, fmt.Errorf("loading user %s: %w", email, err)
	}
	return id, nil
}
