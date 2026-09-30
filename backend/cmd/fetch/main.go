package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/classifier"
	"github.com/AvaneeshVinothK/EmailProject/internal/db"
	"github.com/AvaneeshVinothK/EmailProject/internal/gmail"
	"github.com/AvaneeshVinothK/EmailProject/internal/models"
	"github.com/AvaneeshVinothK/EmailProject/internal/sync"
	"github.com/joho/godotenv"
	gmailapi "google.golang.org/api/gmail/v1"
)

// main bootstraps the Gmail sync flow for the connected account and runs the sync orchestration.
func main() {
	if err := godotenv.Load("../../../.env"); err != nil {
		log.Printf("dotenv not loaded: %v", err)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	cfg, err := gmail.LoadGoogleConfig()
	if err != nil {
		log.Fatal(err)
	}

	tokenPath := gmail.TokenPath()
	token, err := gmail.LoadToken(tokenPath)
	if err != nil {
		log.Fatal("run cmd/auth first")
	}
	if token.RefreshToken == "" && token.AccessToken == "" {
		log.Fatal("run cmd/auth first")
	}

	config := gmail.NewOAuthConfig(cfg, cfg.RedirectURL)
	service, err := gmail.NewService(ctx, config, token)
	if err != nil {
		log.Fatal(err)
	}

	emailAddr, _, err := gmail.GetProfile(ctx, service)
	if err != nil {
		log.Fatal(err)
	}

	store := db.NewStore(pool)
	userID, err := store.GetOrCreateUser(ctx, emailAddr)
	if err != nil {
		log.Fatal(err)
	}

	account, err := store.GetOrCreateEmailAccount(ctx, userID, emailAddr)
	if err != nil {
		log.Fatal(err)
	}

	result, err := sync.Sync(ctx, gmailSyncClient{service: service}, store, account)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("sync result: listed=%d alreadyKnown=%d fetched=%d inserted=%d skipped=%d new_ids=%v", result.Listed, result.AlreadyKnown, result.Fetched, result.Inserted, result.Skipped, result.NewEmailIDs)

	idsToClassify := make([]int, 0, len(result.NewEmailIDs))
	seen := map[int]bool{}
	for _, id := range result.NewEmailIDs {
		if !seen[id] {
			idsToClassify = append(idsToClassify, id)
			seen[id] = true
		}
	}

	unclassified, err := store.GetUnclassifiedEmailIDs(ctx, account.ID)
	if err != nil {
		log.Fatal(err)
	}
	for _, id := range unclassified {
		if !seen[id] {
			idsToClassify = append(idsToClassify, id)
			seen[id] = true
		}
	}

	if len(idsToClassify) == 0 {
		log.Println("classification skipped: no eligible emails to classify")
		return
	}

	gemini, err := classifier.New(ctx)
	if err != nil {
		log.Fatal(err)
	}

	categoryRows, err := store.GetCategories(ctx)
	if err != nil {
		log.Fatal(err)
	}
	categories := make([]classifier.Category, 0, len(categoryRows))
	for _, category := range categoryRows {
		// Description will be populated once user-defined categories exist in the schema.
		categories = append(categories, classifier.Category{Name: category.Name})
	}

	successCount := 0
	failedCount := 0
	for _, id := range idsToClassify {
		email, err := store.GetEmailByID(ctx, id)
		if err != nil {
			log.Printf("classification failed for email id %d: %v", id, err)
			failedCount++
			continue
		}

		res, err := gemini.Classify(ctx, email.Subject, email.Sender, email.Body, categories)
		if err != nil {
			if errors.Is(err, classifier.ErrDailyQuotaExhausted) {
				log.Printf("classification daily quota exhausted; stopping for this run: %v", err)
				break
			}
			log.Printf("classification failed for email id %d (%s): %v", id, email.Subject, err)
			failedCount++
			continue
		}

		var categoryID int
		for _, category := range categoryRows {
			if category.Name == res.Category {
				categoryID = category.ID
				break
			}
		}
		if categoryID == 0 {
			log.Printf("classification failed for email id %d: no matching DB category for %q", id, res.Category)
			failedCount++
			continue
		}

		if err := store.UpsertClassification(ctx, id, categoryID, res.Confidence, classifier.ModelVersion()); err != nil {
			log.Printf("saving classification for email id %d: %v", id, err)
			failedCount++
			continue
		}
		successCount++
	}

	log.Printf("classification result: total=%d successful=%d failed=%d", len(idsToClassify), successCount, failedCount)
}

// gmailSyncClient adapts the Gmail API client to the sync.MailClient interface.
type gmailSyncClient struct {
	service *gmailapi.Service
}

func (g gmailSyncClient) GetProfile(ctx context.Context) (string, uint64, error) {
	return gmail.GetProfile(ctx, g.service)
}

func (g gmailSyncClient) ListMessageIDs(ctx context.Context, after time.Time) ([]string, error) {
	return gmail.ListMessageIDs(ctx, g.service, after)
}

func (g gmailSyncClient) ListHistory(ctx context.Context, startHistoryID uint64) ([]string, uint64, error) {
	return gmail.ListHistory(ctx, g.service, startHistoryID)
}

func (g gmailSyncClient) GetMessage(ctx context.Context, id string) (models.Email, error) {
	return gmail.GetMessage(ctx, g.service, id)
}
