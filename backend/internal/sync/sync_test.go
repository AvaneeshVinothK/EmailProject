package sync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/gmail"
	"github.com/AvaneeshVinothK/EmailProject/internal/models"
)

func TestSync_BackfillAndIncremental(t *testing.T) {
	ctx := context.Background()
	mail := &fakeMailClient{
		profileEmail:     "user@example.com",
		profileHistoryID: 101,
		messageIDs:       []string{"m1", "m2"},
		messages: map[string]models.Email{
			"m1": {GmailMessageID: "m1", Sender: "a@example.com", Subject: "job", Body: "body1", ReceivedAt: time.Now()},
			"m2": {GmailMessageID: "m2", Sender: "b@example.com", Subject: "second", Body: "body2", ReceivedAt: time.Now()},
		},
	}
	store := &fakeStore{existing: map[string]bool{}}
	account := models.EmailAccount{ID: 7, EmailAddress: "user@example.com"}

	res, err := Sync(ctx, mail, store, account)
	if err != nil {
		t.Fatalf("Sync() returned error: %v", err)
	}
	if res.Inserted != 2 || res.Listed != 2 || res.Fetched != 2 || len(res.NewEmailIDs) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got := store.lastHistoryID; got != "101" {
		t.Fatalf("backfill cursor = %q, want %q", got, "101")
	}
	if !store.backfillDone {
		t.Fatal("backfillDone was not set")
	}

	mail.historyForStart = map[uint64][]string{101: {"m3"}}
	mail.messages["m3"] = models.Email{GmailMessageID: "m3", Sender: "c@example.com", Subject: "new", Body: "body3", ReceivedAt: time.Now()}
	account.LastHistoryID = strPtr("101")
	account.BackfillCompletedAt = timePtr(time.Now())
	store.existing = map[string]bool{"m1": true, "m2": true}
	store.lastHistoryID = "101"
	store.backfillDone = true
	store.updateCalls = 0

	res, err = Sync(ctx, mail, store, account)
	if err != nil {
		t.Fatalf("second Sync() returned error: %v", err)
	}
	if res.Inserted != 1 || res.Listed != 1 || res.Fetched != 1 || len(res.NewEmailIDs) != 1 {
		t.Fatalf("unexpected incremental result: %+v", res)
	}
	if got := store.lastHistoryID; got != "102" {
		t.Fatalf("incremental cursor = %q, want %q", got, "102")
	}
}

func TestSync_HistoryExpiredFallsBackToBackfill(t *testing.T) {
	ctx := context.Background()
	mail := &fakeMailClient{
		profileEmail:     "user@example.com",
		profileHistoryID: 77,
		messageIDs:       []string{"m1"},
		messages: map[string]models.Email{
			"m1": {GmailMessageID: "m1", Sender: "s@example.com", Subject: "subject", Body: "body", ReceivedAt: time.Now()},
		},
		historyErr: gmail.ErrHistoryExpired,
	}
	store := &fakeStore{existing: map[string]bool{}}
	account := models.EmailAccount{ID: 3, EmailAddress: "user@example.com", LastHistoryID: strPtr("55"), BackfillCompletedAt: timePtr(time.Now())}

	res, err := Sync(ctx, mail, store, account)
	if err != nil {
		t.Fatalf("Sync() returned error: %v", err)
	}
	if res.Inserted != 1 {
		t.Fatalf("Sync() inserted count = %d, want 1", res.Inserted)
	}
	if got := store.lastHistoryID; got != "77" {
		t.Fatalf("cursor after fallback = %q, want %q", got, "77")
	}
}

func TestSync_HistoryErrorDoesNotBackfillOrAdvanceCursor(t *testing.T) {
	ctx := context.Background()
	mail := &fakeMailClient{profileEmail: "user@example.com", profileHistoryID: 66, historyErr: errors.New("boom")}
	store := &fakeStore{existing: map[string]bool{}}
	account := models.EmailAccount{ID: 2, EmailAddress: "user@example.com", LastHistoryID: strPtr("50"), BackfillCompletedAt: timePtr(time.Now())}

	_, err := Sync(ctx, mail, store, account)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Sync() error = %v, want boom", err)
	}
	if got := store.lastHistoryID; got != "" {
		t.Fatalf("cursor after error = %q, want empty", got)
	}
	if mail.profileCalls != 0 {
		t.Fatalf("GetProfile() called %d times during history error; want 0", mail.profileCalls)
	}
}

func TestSync_UpsertFailurePreventsCursorAdvance(t *testing.T) {
	ctx := context.Background()
	mail := &fakeMailClient{
		profileEmail:     "user@example.com",
		profileHistoryID: 444,
		messageIDs:       []string{"m1"},
		messages: map[string]models.Email{
			"m1": {GmailMessageID: "m1", Sender: "a@example.com", Subject: "x", Body: "body", ReceivedAt: time.Now()},
		},
	}
	store := &fakeStore{existing: map[string]bool{}, upsertErr: errors.New("db broken")}
	account := models.EmailAccount{ID: 8, EmailAddress: "user@example.com"}

	res, err := Sync(ctx, mail, store, account)
	if err == nil || err.Error() != "1 messages failed; cursor not advanced" {
		t.Fatalf("Sync() error = %v, want %q", err, "1 messages failed; cursor not advanced")
	}
	if res.Inserted != 0 {
		t.Fatalf("result.Inserted = %d, want 0", res.Inserted)
	}
	if got := store.lastHistoryID; got != "" {
		t.Fatalf("cursor after upsert failure = %q, want empty", got)
	}
	if len(res.NewEmailIDs) != 0 {
		t.Fatalf("result.NewEmailIDs = %v, want empty", res.NewEmailIDs)
	}
	if res.Failed != 1 {
		t.Fatalf("result.Failed = %d, want 1", res.Failed)
	}
}

func TestSync_KnownIdsAreNotFetchedAndMessageNotFoundIsSkipped(t *testing.T) {
	ctx := context.Background()
	mail := &fakeMailClient{
		profileEmail:     "user@example.com",
		profileHistoryID: 50,
		messageIDs:       []string{"known", "missing", "new"},
		messages: map[string]models.Email{
			"new": {GmailMessageID: "new", Sender: "a@example.com", Subject: "hello", Body: "body", ReceivedAt: time.Now()},
		},
		msgErrs: map[string]error{"missing": gmail.ErrMessageNotFound},
	}
	store := &fakeStore{existing: map[string]bool{"known": true}}
	account := models.EmailAccount{ID: 15, EmailAddress: "user@example.com"}

	res, err := Sync(ctx, mail, store, account)
	if err != nil {
		t.Fatalf("Sync() returned error: %v", err)
	}
	if mail.getMessageCalls["known"] || !mail.getMessageCalls["missing"] || !mail.getMessageCalls["new"] {
		t.Fatalf("unexpected getMessage calls: %+v", mail.getMessageCalls)
	}
	if res.Skipped != 1 || res.Inserted != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestSyncIncremental_RateLimitedStopsAndLeavesCursorUnchanged(t *testing.T) {
	ctx := context.Background()
	mail := &fakeMailClient{
		historyForStart: map[uint64][]string{55: {"m1", "m2"}},
		msgErrs:         map[string]error{"m1": gmail.ErrRateLimited},
		messages: map[string]models.Email{
			"m2": {GmailMessageID: "m2", Sender: "b@example.com", Subject: "later", Body: "body2", ReceivedAt: time.Now()},
		},
	}
	store := &fakeStore{existing: map[string]bool{}}
	account := models.EmailAccount{ID: 22, EmailAddress: "user@example.com", LastHistoryID: strPtr("55"), BackfillCompletedAt: timePtr(time.Now())}

	res, err := syncIncremental(ctx, mail, store, account)
	if !errors.Is(err, gmail.ErrRateLimited) {
		t.Fatalf("syncIncremental() error = %v, want ErrRateLimited", err)
	}
	if mail.getMessageCalls["m2"] {
		t.Fatalf("GetMessage called for later id after rate limit; calls = %+v", mail.getMessageCalls)
	}
	if store.lastHistoryID != "" {
		t.Fatalf("cursor after rate limit = %q, want empty", store.lastHistoryID)
	}
	if res.Failed != 0 {
		t.Fatalf("result.Failed = %d, want 0", res.Failed)
	}
}

func TestSyncIncremental_OrdinaryFailureSetsFailedAndLeavesCursorUnchanged(t *testing.T) {
	ctx := context.Background()
	mail := &fakeMailClient{
		historyForStart: map[uint64][]string{10: {"m1"}},
		msgErrs:         map[string]error{"m1": errors.New("boom")},
	}
	store := &fakeStore{existing: map[string]bool{}}
	account := models.EmailAccount{ID: 9, EmailAddress: "user@example.com", LastHistoryID: strPtr("10"), BackfillCompletedAt: timePtr(time.Now())}

	res, err := syncIncremental(ctx, mail, store, account)
	if err == nil || err.Error() != "1 messages failed; cursor not advanced" {
		t.Fatalf("syncIncremental() error = %v, want %q", err, "1 messages failed; cursor not advanced")
	}
	if res.Failed != 1 {
		t.Fatalf("result.Failed = %d, want 1", res.Failed)
	}
	if store.lastHistoryID != "" {
		t.Fatalf("cursor after ordinary failure = %q, want empty", store.lastHistoryID)
	}
}

type fakeMailClient struct {
	profileEmail     string
	profileHistoryID uint64
	messageIDs       []string
	messages         map[string]models.Email
	msgErrs          map[string]error
	historyForStart  map[uint64][]string
	historyErr       error
	profileCalls     int
	getMessageCalls  map[string]bool
}

func (f *fakeMailClient) GetProfile(ctx context.Context) (string, uint64, error) {
	f.profileCalls++
	return f.profileEmail, f.profileHistoryID, nil
}

func (f *fakeMailClient) ListMessageIDs(ctx context.Context, after time.Time) ([]string, error) {
	return f.messageIDs, nil
}

func (f *fakeMailClient) ListHistory(ctx context.Context, startHistoryID uint64) ([]string, uint64, error) {
	if f.historyErr != nil {
		return nil, 0, f.historyErr
	}
	ids := f.historyForStart[startHistoryID]
	if len(ids) == 0 {
		return nil, startHistoryID, nil
	}
	return ids, 102, nil
}

func (f *fakeMailClient) GetMessage(ctx context.Context, id string) (models.Email, error) {
	if f.getMessageCalls == nil {
		f.getMessageCalls = map[string]bool{}
	}
	f.getMessageCalls[id] = true
	if err, ok := f.msgErrs[id]; ok {
		return models.Email{}, err
	}
	msg, ok := f.messages[id]
	if !ok {
		return models.Email{}, fmt.Errorf("missing")
	}
	msg.GmailMessageID = id
	return msg, nil
}

type fakeStore struct {
	existing       map[string]bool
	lastHistoryID  string
	backfillDone   bool
	updateCalls    int
	upsertErr      error
	upsertReturned map[string]int
}

func (f *fakeStore) ExistingMessageIDs(ctx context.Context, accountID int, ids []string) (map[string]bool, error) {
	result := map[string]bool{}
	for _, id := range ids {
		if f.existing[id] {
			result[id] = true
		}
	}
	return result, nil
}

func (f *fakeStore) UpsertEmail(ctx context.Context, e models.Email) (int, bool, error) {
	if f.upsertErr != nil {
		return 0, false, f.upsertErr
	}
	if f.upsertReturned == nil {
		f.upsertReturned = map[string]int{}
	}
	id := len(f.upsertReturned) + 1
	f.upsertReturned[e.GmailMessageID] = id
	if f.existing[e.GmailMessageID] {
		return id, false, nil
	}
	f.existing[e.GmailMessageID] = true
	return id, true, nil
}

func (f *fakeStore) UpdateSyncState(ctx context.Context, accountID int, lastHistoryID string, backfillDone bool) error {
	f.updateCalls++
	f.lastHistoryID = lastHistoryID
	f.backfillDone = backfillDone
	return nil
}

func strPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
