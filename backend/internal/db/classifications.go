package db

import (
	"context"
	"fmt"

	"github.com/AvaneeshVinothK/EmailProject/internal/models"
)

// GetCategories returns the configured category names ordered by database id.
func (s *Store) GetCategories(ctx context.Context) ([]models.Category, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name
		FROM categories
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("loading categories: %w", err)
	}
	defer rows.Close()

	categories := make([]models.Category, 0)
	for rows.Next() {
		var category models.Category
		if err := rows.Scan(&category.ID, &category.Name); err != nil {
			return nil, fmt.Errorf("scanning category row: %w", err)
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating categories: %w", err)
	}
	return categories, nil
}

// UpsertClassification stores the current classification for an email, replacing any prior result.
func (s *Store) UpsertClassification(ctx context.Context, emailID, categoryID int, confidence float64, modelVersion string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO classifications (email_id, category_id, confidence, model_version)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email_id) DO UPDATE SET
			category_id = EXCLUDED.category_id,
			confidence = EXCLUDED.confidence,
			model_version = EXCLUDED.model_version,
			created_at = now()
	`, emailID, categoryID, confidence, modelVersion)
	if err != nil {
		return fmt.Errorf("upserting classification for email %d: %w", emailID, err)
	}
	return nil
}

// GetUnclassifiedEmailIDs returns email IDs for this account that do not yet have a classification row.
func (s *Store) GetUnclassifiedEmailIDs(ctx context.Context, accountID int) ([]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id
		FROM emails e
		LEFT JOIN classifications c ON c.email_id = e.id
		WHERE e.email_account_id = $1 AND c.id IS NULL
		ORDER BY e.received_at DESC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("loading unclassified email ids: %w", err)
	}
	defer rows.Close()

	ids := make([]int, 0)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning unclassified email ids: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unclassified email ids: %w", err)
	}
	return ids, nil
}

// GetEmailByID fetches a stored email by database ID for classification.
func (s *Store) GetEmailByID(ctx context.Context, id int) (models.Email, error) {
	var email models.Email
	if err := s.pool.QueryRow(ctx, `
		SELECT id, email_account_id, gmail_message_id, sender, subject, body, received_at
		FROM emails
		WHERE id = $1
	`, id).Scan(
		&email.ID,
		&email.EmailAccountID,
		&email.GmailMessageID,
		&email.Sender,
		&email.Subject,
		&email.Body,
		&email.ReceivedAt,
	); err != nil {
		return models.Email{}, fmt.Errorf("loading email %d: %w", id, err)
	}
	return email, nil
}
