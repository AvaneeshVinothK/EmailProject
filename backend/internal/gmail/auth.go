package gmail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
)

// GoogleCredentialsFile is the top-level JSON structure in the Google OAuth credentials file.
type GoogleCredentialsFile struct {
	Web GoogleCredentials `json:"web"`
}

// GoogleCredentials contains the OAuth client settings Google emits for a web app.
type GoogleCredentials struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
	RedirectURIs []string `json:"redirect_uris"`
}

// LoadedGoogleConfig is the runtime-usable subset of OAuth client settings.
type LoadedGoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// ProjectRoot finds the directory containing the local Google credentials file.
func ProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}

	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "google_credentials.json")
		if _, err := os.Stat(candidate); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return wd
}

func projectFilePath(name string) string {
	root := ProjectRoot()
	return filepath.Join(root, name)
}

// GoogleCredentialsPath returns the location of the Google OAuth client credentials file.
func GoogleCredentialsPath() string {
	return projectFilePath("google_credentials.json")
}

// TokenPath returns the location of the serialized OAuth token for the current project.
func TokenPath() string {
	return projectFilePath("token.json")
}

// LoadGoogleConfig reads the OAuth client config from env vars or the local credentials file.
func LoadGoogleConfig() (*LoadedGoogleConfig, error) {
	if clientID := os.Getenv("GOOGLE_CLIENT_ID"); clientID != "" {
		return &LoadedGoogleConfig{
			ClientID:     clientID,
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		}, nil
	}

	credentialsPath := GoogleCredentialsPath()
	paths := []string{
		credentialsPath,
		filepath.Join("..", "google_credentials.json"),
		"google_credentials.json",
		filepath.Join(".", "google_credentials.json"),
	}

	var lastErr error
	for _, candidate := range paths {
		data, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = err
			continue
		}

		var creds GoogleCredentialsFile
		if err := json.Unmarshal(data, &creds); err != nil {
			return nil, fmt.Errorf("invalid google_credentials.json: %w", err)
		}

		if strings.TrimSpace(creds.Web.ClientID) == "" || strings.TrimSpace(creds.Web.ClientSecret) == "" {
			return nil, errors.New("google_credentials.json is missing client_id or client_secret")
		}

		redirectURL := ""
		if len(creds.Web.RedirectURIs) > 0 {
			redirectURL = creds.Web.RedirectURIs[0]
		}

		return &LoadedGoogleConfig{
			ClientID:     creds.Web.ClientID,
			ClientSecret: creds.Web.ClientSecret,
			RedirectURL:  redirectURL,
		}, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("unable to read google_credentials.json: %w", lastErr)
	}

	return nil, errors.New("no google credentials file found")
}

// LoadToken reads the previously saved OAuth token from disk.
func LoadToken(tokenPath string) (*oauth2.Token, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, err
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// SaveToken writes the refreshed OAuth token to disk so future runs can reuse it.
func SaveToken(tokenPath string, token *oauth2.Token) error {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokenPath, data, 0o600)
}
