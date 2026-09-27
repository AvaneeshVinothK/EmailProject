package gmail

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

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

func FetchRecentMessages(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) error {
	tokenSource := cfg.TokenSource(ctx, token)
	refreshedToken, err := tokenSource.Token()
	if err != nil {
		return err
	}
	if refreshedToken != nil && refreshedToken.AccessToken != "" {
		token = refreshedToken
		if err := SaveToken(TokenPath(), token); err != nil {
			log.Printf("warning: failed to persist refreshed token: %v", err)
		}
	}

	client := oauth2.NewClient(ctx, tokenSource)
	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	profile, err := service.Users.GetProfile("me").Do()
	if err != nil {
		return fmt.Errorf("failed to load Gmail profile: %w", err)
	}
	fmt.Printf("Authenticated Gmail account: %s\n", profile.EmailAddress)

	list, err := service.Users.Messages.List("me").
		LabelIds("INBOX").
		Q("in:inbox newer_than:30d").
		MaxResults(20).
		Do()
	if err != nil {
		return fmt.Errorf("failed to list Gmail messages: %w", err)
	}
	if len(list.Messages) == 0 {
		fmt.Println("No recent messages found in INBOX for the authenticated Gmail account in the last 30 days.")
		return nil
	}

	for _, msgRef := range list.Messages {
		msg, err := service.Users.Messages.Get("me", msgRef.Id).Format("full").Do()
		if err != nil {
			log.Printf("message %s failed: %v", msgRef.Id, err)
			continue
		}

		subject := HeaderValue(msg.Payload.Headers, "Subject")
		sender := HeaderValue(msg.Payload.Headers, "From")
		body := ExtractMessageBody(msg.Payload)

		fmt.Printf("\nMessage ID: %s\nSubject: %s\nFrom: %s\nBody:\n%s\n---\n", msgRef.Id, subject, sender, body)
	}

	return nil
}
