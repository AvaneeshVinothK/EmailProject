package gmail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestProjectRootAndPaths(t *testing.T) {
	root := setupTempProject(t, `{"web":{"client_id":"cid","client_secret":"secret"}}`)

	if got := ProjectRoot(); got != root {
		t.Fatalf("ProjectRoot() = %q, want %q", got, root)
	}
	if got := GoogleCredentialsPath(); got != filepath.Join(root, "google_credentials.json") {
		t.Fatalf("GoogleCredentialsPath() = %q, want %q", got, filepath.Join(root, "google_credentials.json"))
	}
	if got := TokenPath(); got != filepath.Join(root, "token.json") {
		t.Fatalf("TokenPath() = %q, want %q", got, filepath.Join(root, "token.json"))
	}
}

func TestLoadGoogleConfigFromEnv(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "env-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "env-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/oauth/callback")

	cfg, err := LoadGoogleConfig()
	if err != nil {
		t.Fatalf("LoadGoogleConfig() unexpected error = %v", err)
	}
	if cfg.ClientID != "env-client-id" || cfg.ClientSecret != "env-client-secret" {
		t.Fatalf("LoadGoogleConfig() = %+v, want env values", cfg)
	}
	if cfg.RedirectURL != "http://localhost:8080/oauth/callback" {
		t.Fatalf("LoadGoogleConfig().RedirectURL = %q, want callback URL", cfg.RedirectURL)
	}
}

func TestLoadGoogleConfigFromFile(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("GOOGLE_CLIENT_SECRET", "")
	t.Setenv("GOOGLE_REDIRECT_URL", "")

	setupTempProject(t, `{"web":{"client_id":"file-client-id","client_secret":"file-client-secret","redirect_uris":["http://localhost:9000/oauth/callback"]}}`)

	cfg, err := LoadGoogleConfig()
	if err != nil {
		t.Fatalf("LoadGoogleConfig() unexpected error = %v", err)
	}
	if cfg.ClientID != "file-client-id" || cfg.ClientSecret != "file-client-secret" {
		t.Fatalf("LoadGoogleConfig() = %+v, want file values", cfg)
	}
	if cfg.RedirectURL != "http://localhost:9000/oauth/callback" {
		t.Fatalf("LoadGoogleConfig().RedirectURL = %q, want callback URL", cfg.RedirectURL)
	}
}

func TestSaveAndLoadTokenRoundTrip(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	token := &oauth2.Token{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}

	if err := SaveToken(tokenPath, token); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	loaded, err := LoadToken(tokenPath)
	if err != nil {
		t.Fatalf("LoadToken() error = %v", err)
	}
	if loaded.AccessToken != token.AccessToken || loaded.RefreshToken != token.RefreshToken || loaded.TokenType != token.TokenType {
		t.Fatalf("LoadToken() = %+v, want %+v", loaded, token)
	}
	if !loaded.Expiry.After(time.Now()) {
		t.Fatalf("LoadToken().Expiry = %v, want future expiry", loaded.Expiry)
	}
}

func TestNewOAuthConfig(t *testing.T) {
	cfg := &LoadedGoogleConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}

	result := NewOAuthConfig(cfg, "")
	if result.ClientID != "client-id" || result.ClientSecret != "client-secret" {
		t.Fatalf("NewOAuthConfig() = %+v, want client id and secret", result)
	}
	if result.RedirectURL != "http://localhost:8080/oauth/callback" {
		t.Fatalf("NewOAuthConfig().RedirectURL = %q, want default callback URL", result.RedirectURL)
	}
	if len(result.Scopes) == 0 || result.Scopes[0] != "openid" {
		t.Fatalf("NewOAuthConfig().Scopes = %v, want openid first", result.Scopes)
	}

	custom := NewOAuthConfig(cfg, "http://localhost:9090/oauth/callback")
	if custom.RedirectURL != "http://localhost:9090/oauth/callback" {
		t.Fatalf("NewOAuthConfig() custom RedirectURL = %q, want custom URL", custom.RedirectURL)
	}
}

func setupTempProject(t *testing.T, credsJSON string) string {
	t.Helper()

	root := t.TempDir()
	subdir := filepath.Join(root, "backend", "internal", "gmail")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	credentialsPath := filepath.Join(root, "google_credentials.json")
	if err := os.WriteFile(credentialsPath, []byte(credsJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Chdir(subdir)
	return root
}
