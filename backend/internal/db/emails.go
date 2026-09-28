package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/AvaneeshVinothK/EmailProject/internal/models"
	"github.com/jackc/pgx/v5"
)

// ExistingMessageIDs returns the set of Gmail message IDs already stored for this account.
func (s *Store) ExistingMessageIDs(ctx context.Context, accountID int, ids []string) (map[string]bool, error) {
	if len(ids) == 0 {
		return map[string]bool{}, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT gmail_message_id
		FROM emails
		WHERE email_account_id = $1 AND gmail_message_id = ANY($2)
	`, accountID, ids)
	if err != nil {
		return nil, fmt.Errorf("loading existing message ids: %w", err)
	}
	defer rows.Close()

	known := make(map[string]bool, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning existing message ids: %w", err)
		}
		known[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating existing message ids: %w", err)
	}
	return known, nil
}

// UpsertEmail inserts or ignores an email row while returning the inserted row ID when new.
func (s *Store) UpsertEmail(ctx context.Context, e models.Email) (int, bool, error) {
	e.GmailMessageID = clean(e.GmailMessageID)
	e.Sender = clean(e.Sender)
	e.Subject = clean(e.Subject)
	e.Body = clean(e.Body)

	var id int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO emails (email_account_id, gmail_message_id, sender, subject, body, received_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (email_account_id, gmail_message_id) DO NOTHING
		RETURNING id
	`, e.EmailAccountID, e.GmailMessageID, e.Sender, e.Subject, e.Body, e.ReceivedAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("upserting email %s: %w", e.GmailMessageID, err)
	}
	return id, true, nil
}
