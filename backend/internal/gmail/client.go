package gmail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/models"
	"github.com/AvaneeshVinothK/EmailProject/internal/throttle"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// ErrHistoryExpired means Gmail's history cursor is stale and must be reset via backfill.
var ErrHistoryExpired = errors.New("gmail history expired")

// ErrMessageNotFound means the message was deleted or no longer available from Gmail.
var ErrMessageNotFound = errors.New("gmail message not found")

// NewOAuthConfig builds the OAuth client config for Gmail API access.
func NewOAuthConfig(cfg *LoadedGoogleConfig, redirectURL string) *oauth2.Config {
	if redirectURL == "" {
		redirectURL = "http://localhost:8080/oauth/callback"
	}

	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"openid",
			"email",
			"profile",
			"https://www.googleapis.com/auth/gmail.readonly",
		},
		Endpoint: google.Endpoint,
	}
}

// NewService creates a Gmail API client using the provided OAuth configuration and token.
func NewService(ctx context.Context, oauthCfg *oauth2.Config, token *oauth2.Token) (*gmailapi.Service, error) {
	if oauthCfg == nil {
		return nil, errors.New("oauth config is nil")
	}
	if token == nil {
		return nil, errors.New("oauth token is nil")
	}

	tokenSource := oauthCfg.TokenSource(ctx, token)
	refreshedToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("refreshing Gmail token: %w", err)
	}
	if refreshedToken != nil && refreshedToken.AccessToken != "" {
		token = refreshedToken
	}

	client := oauth2.NewClient(ctx, tokenSource)
	client.Transport = throttle.New(client.Transport, 10, 1, gmailClassify)
	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("creating Gmail service: %w", err)
	}
	return service, nil
}

// GetProfile loads the signed-in account email and the current Gmail history ID.
func GetProfile(ctx context.Context, svc *gmailapi.Service) (string, uint64, error) {
	profile, err := svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return "", 0, wrapAPIError("loading Gmail profile", err)
	}
	return profile.EmailAddress, profile.HistoryId, nil
}

// ListMessageIDs returns all message IDs for inbox messages newer than the cutoff time.
func ListMessageIDs(ctx context.Context, svc *gmailapi.Service, after time.Time) ([]string, error) {
	query := fmt.Sprintf("in:inbox after:%d", after.Unix())
	ids := make([]string, 0)
	call := svc.Users.Messages.List("me").Q(query).MaxResults(500).Context(ctx)
	if err := call.Pages(ctx, func(resp *gmailapi.ListMessagesResponse) error {
		for _, msg := range resp.Messages {
			if msg != nil && msg.Id != "" {
				ids = append(ids, msg.Id)
			}
		}
		return nil
	}); err != nil {
		return nil, wrapAPIError("listing Gmail messages", err)
	}
	return ids, nil
}

// ListHistory fetches the change set since a Gmail history cursor and returns the newest cursor.
func ListHistory(ctx context.Context, svc *gmailapi.Service, startHistoryID uint64) ([]string, uint64, error) {
	if svc == nil {
		return nil, startHistoryID, errors.New("gmail service is nil")
	}

	seen := map[string]struct{}{}
	ids := make([]string, 0)
	newestHistoryID := startHistoryID
	call := svc.Users.History.List("me").
		StartHistoryId(startHistoryID).
		HistoryTypes("messageAdded", "labelAdded").
		Context(ctx)

	err := call.Pages(ctx, func(resp *gmailapi.ListHistoryResponse) error {
		if resp == nil {
			return nil
		}
		if resp.HistoryId > newestHistoryID {
			newestHistoryID = resp.HistoryId
		}
		for _, history := range resp.History {
			if history == nil {
				continue
			}
			for _, added := range history.MessagesAdded {
				if added == nil || added.Message == nil || added.Message.Id == "" {
					continue
				}
				if !hasInboxLabelID(added.Message.LabelIds) {
					continue
				}
				if _, ok := seen[added.Message.Id]; !ok {
					seen[added.Message.Id] = struct{}{}
					ids = append(ids, added.Message.Id)
				}
			}
			for _, labelAdded := range history.LabelsAdded {
				if labelAdded == nil || labelAdded.Message == nil || labelAdded.Message.Id == "" {
					continue
				}
				if !hasInboxLabelID(labelAdded.LabelIds) {
					continue
				}
				if _, ok := seen[labelAdded.Message.Id]; !ok {
					seen[labelAdded.Message.Id] = struct{}{}
					ids = append(ids, labelAdded.Message.Id)
				}
			}
		}
		return nil
	})
	if err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == httpStatusNotFound {
			return nil, 0, ErrHistoryExpired
		}
		return nil, 0, wrapAPIError("listing Gmail history", err)
	}
	if newestHistoryID == 0 {
		newestHistoryID = startHistoryID
	}
	return ids, newestHistoryID, nil
}

// GetMessage retrieves one message body and headers and normalizes it into the app model.
func GetMessage(ctx context.Context, svc *gmailapi.Service, id string) (models.Email, error) {
	msg, err := svc.Users.Messages.Get("me", id).Format("full").Context(ctx).Do()
	if err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == httpStatusNotFound {
			return models.Email{}, ErrMessageNotFound
		}
		return models.Email{}, wrapAPIError(fmt.Sprintf("loading Gmail message %s", id), err)
	}

	email := models.Email{
		GmailMessageID: id,
		Body:           "",
		ReceivedAt:     time.UnixMilli(msg.InternalDate),
	}
	if msg != nil && msg.Payload != nil {
		email.Sender = HeaderValue(msg.Payload.Headers, "From")
		email.Subject = HeaderValue(msg.Payload.Headers, "Subject")
		email.Body = ExtractMessageBody(msg.Payload)
	}
	return email, nil
}

func hasInboxLabelID(labelIDs []string) bool {
	for _, id := range labelIDs {
		if id == "INBOX" {
			return true
		}
	}
	return false
}

const httpStatusNotFound = 404
