package sync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/gmail"
	"github.com/AvaneeshVinothK/EmailProject/internal/models"
)

const backfillWindowDays = 60

// Result summarizes the outcome of a sync attempt for one account.
type Result struct {
	Listed       int
	AlreadyKnown int
	Fetched      int
	Inserted     int
	Failed       int
	Skipped      int
	NewEmailIDs  []int
}

// MailClient defines the Gmail-facing behavior required to sync an account.
type MailClient interface {
	GetProfile(ctx context.Context) (string, uint64, error)
	ListMessageIDs(ctx context.Context, after time.Time) ([]string, error)
	ListHistory(ctx context.Context, startHistoryID uint64) ([]string, uint64, error)
	GetMessage(ctx context.Context, id string) (models.Email, error)
}

// Store is the persistence boundary used by the sync layer.
type Store interface {
	ExistingMessageIDs(ctx context.Context, accountID int, ids []string) (map[string]bool, error)
	UpsertEmail(ctx context.Context, e models.Email) (int, bool, error)
	UpdateSyncState(ctx context.Context, accountID int, lastHistoryID string, backfillDone bool) error
}

// Sync chooses the right strategy for an account: full backfill when state is missing,
// otherwise incremental history sync with an automatic backfill fallback if Gmail reports
// an expired history cursor.
func Sync(ctx context.Context, mail MailClient, store Store, account models.EmailAccount) (Result, error) {
	if account.LastHistoryID == nil || account.BackfillCompletedAt == nil {
		return syncBackfill(ctx, mail, store, account)
	}

	result, err := syncIncremental(ctx, mail, store, account)
	if err == nil {
		return result, nil
	}
	if errors.Is(err, gmail.ErrHistoryExpired) {
		log.Printf("gmail history expired for account %d; falling back to backfill", account.ID)
		return syncBackfill(ctx, mail, store, account)
	}
	return result, err
}

// syncBackfill loads recent inbox IDs and persistently stores the full message body for each
// newly seen message, while retaining the last known Gmail history ID once the pass succeeds.
func syncBackfill(ctx context.Context, mail MailClient, store Store, account models.EmailAccount) (Result, error) {
	result := Result{}
	profileEmail, historyID, err := mail.GetProfile(ctx)
	if err != nil {
		return result, fmt.Errorf("getting Gmail profile: %w", err)
	}
	_ = profileEmail

	cutoff := time.Now().AddDate(0, 0, -backfillWindowDays)
	ids, err := mail.ListMessageIDs(ctx, cutoff)
	if err != nil {
		return result, fmt.Errorf("listing backfill ids: %w", err)
	}
	result.Listed = len(ids)

	if err := processIDs(ctx, mail, store, account.ID, ids, &result); err != nil {
		return result, err
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("%d messages failed; cursor not advanced", result.Failed)
	}
	if err := store.UpdateSyncState(ctx, account.ID, strconv.FormatUint(historyID, 10), true); err != nil {
		return result, fmt.Errorf("updating sync state after backfill: %w", err)
	}

	return result, nil
}

// syncIncremental fetches only the Gmail history delta since the last cursor and updates the
// stored cursor only if every step in the incremental pass succeeded.
func syncIncremental(ctx context.Context, mail MailClient, store Store, account models.EmailAccount) (Result, error) {
	result := Result{}
	startHistoryID, err := strconv.ParseUint(*account.LastHistoryID, 10, 64)
	if err != nil {
		return result, fmt.Errorf("parsing last_history_id %q: %w", *account.LastHistoryID, err)
	}

	ids, newestHistoryID, err := mail.ListHistory(ctx, startHistoryID)
	if err != nil {
		return result, err
	}
	result.Listed = len(ids)

	if err := processIDs(ctx, mail, store, account.ID, ids, &result); err != nil {
		return result, err
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("%d messages failed; cursor not advanced", result.Failed)
	}
	cursor := *account.LastHistoryID
	if newestHistoryID != 0 {
		cursor = strconv.FormatUint(newestHistoryID, 10)
	}
	if err := store.UpdateSyncState(ctx, account.ID, cursor, false); err != nil {
		return result, fmt.Errorf("updating incremental sync state: %w", err)
	}

	return result, nil
}

func processIDs(ctx context.Context, mail MailClient, store Store, accountID int, ids []string, result *Result) error {
	known, err := store.ExistingMessageIDs(ctx, accountID, ids)
	if err != nil {
		return fmt.Errorf("finding known messages: %w", err)
	}

	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if known[id] {
			result.AlreadyKnown++
			continue
		}

		email, err := mail.GetMessage(ctx, id)
		if errors.Is(err, gmail.ErrMessageNotFound) {
			result.Skipped++
			continue
		}
		if errors.Is(err, gmail.ErrRateLimited) {
			return err
		}
		if err != nil {
			log.Printf("message %s fetch failed during sync: %v", id, err)
			result.Failed++
			continue
		}
		result.Fetched++
		email.EmailAccountID = accountID

		newID, inserted, err := store.UpsertEmail(ctx, email)
		if err != nil {
			log.Printf("message %s insert failed during sync: %v", id, err)
			result.Failed++
			continue
		}
		if inserted {
			result.Inserted++
			result.NewEmailIDs = append(result.NewEmailIDs, newID)
			continue
		}
		result.AlreadyKnown++
	}
	return nil
}
