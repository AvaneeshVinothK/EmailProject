package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/AvaneeshVinothK/EmailProject/internal/gmail"
	"golang.org/x/oauth2"
)

const defaultPort = ":8080"

var stateMu sync.Mutex
var validStates = map[string]bool{}

// main starts the local Google OAuth consent server used to authenticate a Gmail account.
func main() {
	ctx := context.Background()
	cfg, err := gmail.LoadGoogleConfig()
	if err != nil {
		log.Fatal(err)
	}

	config := gmail.NewOAuthConfig(cfg, cfg.RedirectURL)
	tokenPath := gmail.TokenPath()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		state := newState()
		url := config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
		fmt.Fprintf(w, "<html><body><a href=\"%s\">Sign in with Google</a></body></html>", url)
	})

	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		state := r.FormValue("state")
		if !verifyState(state) {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
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

		service, err := gmail.NewService(ctx, config, token)
		if err != nil {
			http.Error(w, "failed to initialize Gmail service: "+err.Error(), http.StatusInternalServerError)
			return
		}
		email, _, err := gmail.GetProfile(ctx, service)
		if err != nil {
			http.Error(w, "failed to fetch connected Gmail address: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Connected Google account: %s. You can close this tab.", email)
	})

	log.Printf("Google OAuth auth server running at http://localhost%s", defaultPort)
	log.Fatal(http.ListenAndServe(defaultPort, mux))
}

// newState creates a random OAuth state token and tracks it for later verification.
func newState() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	state := hex.EncodeToString(buf)
	stateMu.Lock()
	validStates[state] = true
	stateMu.Unlock()
	return state
}

// verifyState confirms the callback state token belongs to this request flow and then invalidates it.
func verifyState(state string) bool {
	stateMu.Lock()
	defer stateMu.Unlock()
	if !validStates[state] {
		return false
	}
	delete(validStates, state)
	return true
}
