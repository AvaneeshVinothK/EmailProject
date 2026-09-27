package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/AvaneeshVinothK/EmailProject/internal/gmail"
	"golang.org/x/oauth2"
)

const (
	defaultPort = ":8080"
)

func main() {
	ctx := context.Background()
	cfg, err := gmail.LoadGoogleConfig()
	if err != nil {
		log.Fatal(err)
	}

	config := gmail.NewOAuthConfig(cfg, cfg.RedirectURL)
	tokenPath := gmail.TokenPath()

	if token, err := gmail.LoadToken(tokenPath); err == nil && token.Valid() {
		log.Println("Using saved Google token from token.json")
		if err := gmail.FetchRecentMessages(ctx, config, token); err != nil {
			log.Printf("Gmail fetch failed: %v", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if token, err := gmail.LoadToken(tokenPath); err == nil && token.Valid() {
			fmt.Fprintln(w, "Already authenticated using saved token. Refreshing Gmail messages...")
			if err := gmail.FetchRecentMessages(ctx, config, token); err != nil {
				fmt.Fprintf(w, "Gmail fetch failed: %v\n", err)
			}
			return
		}

		url := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
		fmt.Fprintf(w, "<html><body><a href=\"%s\">Sign in with Google</a></body></html>", url)
	})

	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid callback request", http.StatusBadRequest)
			return
		}

		code := r.FormValue("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			return
		}

		token, err := config.Exchange(ctx, code)
		if err != nil {
			http.Error(w, "failed to exchange code for token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := gmail.SaveToken(tokenPath, token); err != nil {
			http.Error(w, "failed to save token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := gmail.FetchRecentMessages(ctx, config, token); err != nil {
			http.Error(w, "failed to fetch Gmail messages: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Google login successful. Token saved to %s", tokenPath)
	})

	log.Printf("Google OAuth example running at http://localhost%s", defaultPort)
	log.Fatal(http.ListenAndServe(defaultPort, mux))
}
