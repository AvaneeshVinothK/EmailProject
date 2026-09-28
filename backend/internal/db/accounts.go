package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/AvaneeshVinothK/EmailProject/internal/models"
	"github.com/jackc/pgx/v5"
)

// GetOrCreateEmailAccount ensures an email account row exists for the given user and address.
func (s *Store) GetOrCreateEmailAccount(ctx context.Context, userID int, emailAddress string) (models.EmailAccount, error) {
	emailAddress = clean(emailAddress)
	const query = `
		INSERT INTO email_accounts (user_id, email_address)
		VALUES ($1, $2)
		ON CONFLICT (user_id, email_address) DO NOTHING
		RETURNING id, user_id, email_address, last_history_id, backfill_completed_at, created_at
	`

	var account models.EmailAccount
	err := s.pool.QueryRow(ctx, query, userID, emailAddress).Scan(
		&account.ID,
		&account.UserID,
		&account.EmailAddress,
		&account.LastHistoryID,
		&account.BackfillCompletedAt,
		&account.CreatedAt,
	)
	if err == nil {
		return account, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return models.EmailAccount{}, fmt.Errorf("creating email account %s for user %d: %w", emailAddress, userID, err)
	}

	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, email_address, last_history_id, backfill_completed_at, created_at
		FROM email_accounts
		WHERE user_id = $1 AND email_address = $2
	`, userID, emailAddress)
	if err := row.Scan(
		&account.ID,
		&account.UserID,
		&account.EmailAddress,
		&account.LastHistoryID,
		&account.BackfillCompletedAt,
		&account.CreatedAt,
	); err != nil {
		return models.EmailAccount{}, fmt.Errorf("loading email account %s: %w", emailAddress, err)
	}
	return account, nil
}

// UpdateSyncState persists the latest history cursor and the backfill completion state.
func (s *Store) UpdateSyncState(ctx context.Context, accountID int, lastHistoryID string, backfillDone bool) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE email_accounts
		SET last_history_id = $2,
		    backfill_completed_at = CASE WHEN $3 THEN now() ELSE backfill_completed_at END
		WHERE id = $1
	`, accountID, lastHistoryID, backfillDone)
	if err != nil {
		return fmt.Errorf("updating sync state: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("updating sync state: no email account found for id %d", accountID)
	}
	return nil
}
