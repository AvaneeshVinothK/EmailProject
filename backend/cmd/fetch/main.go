package main

import (
	"context"
	"log"
	"time"

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
