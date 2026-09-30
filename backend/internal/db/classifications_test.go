//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpsertClassificationOverwritesExistingRow(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set; skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	store := NewStore(pool)

	emailAddr := "classification.integration." + time.Now().Format("20060102150405") + "@example.com"
	userID, err := store.GetOrCreateUser(ctx, emailAddr)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	account, err := store.GetOrCreateEmailAccount(ctx, userID, emailAddr)
	if err != nil {
		t.Fatalf("GetOrCreateEmailAccount: %v", err)
	}

	msgID := "integration-classification-" + time.Now().Format("20060102150405")
	emailID, _, err := store.UpsertEmail(ctx, models.Email{
		EmailAccountID: account.ID,
		GmailMessageID: msgID,
		Sender:         "sender@example.com",
		Subject:        "Email classification test",
		Body:           "Test body",
		ReceivedAt:     time.Now(),
	})
	if err != nil {
		t.Fatalf("UpsertEmail: %v", err)
	}

	categories, err := store.GetCategories(ctx)
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if len(categories) < 2 {
		t.Fatalf("expected at least 2 categories, got %d", len(categories))
	}

	if err := store.UpsertClassification(ctx, emailID, categories[0].ID, 0.80, "gemini-test"); err != nil {
		t.Fatalf("first UpsertClassification: %v", err)
	}
	if err := store.UpsertClassification(ctx, emailID, categories[1].ID, 0.91, "gemini-test-2"); err != nil {
		t.Fatalf("second UpsertClassification: %v", err)
	}

	var categoryID int
	var confidence float64
	var modelVersion string
	if err := pool.QueryRow(ctx, `SELECT category_id, confidence, model_version FROM classifications WHERE email_id = $1`, emailID).Scan(&categoryID, &confidence, &modelVersion); err != nil {
		t.Fatalf("SELECT updated classification: %v", err)
	}
	if categoryID != categories[1].ID {
		t.Fatalf("expected category_id=%d after update, got %d", categories[1].ID, categoryID)
	}
	if confidence != 0.91 {
		t.Fatalf("expected confidence=0.91 after update, got %v", confidence)
	}
	if modelVersion != "gemini-test-2" {
		t.Fatalf("expected model_version=gemini-test-2 after update, got %s", modelVersion)
	}
}
