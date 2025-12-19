package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"d365tui/internal/config"
)

// Token represents an OAuth token
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired checks if the token is expired or about to expire
func (t *Token) IsExpired() bool {
	// Consider expired if within 5 minutes of expiry
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// tokenFilePath returns the path for the environment-specific token file
func tokenFilePath(envName string) string {
	safeName := strings.ReplaceAll(envName, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	return filepath.Join(config.GetConfigDir(), fmt.Sprintf("token-%s.json", safeName))
}

// LoadToken loads a token for a specific environment
func LoadToken(envName string) (*Token, error) {
	path := tokenFilePath(envName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// SaveToken saves a token for a specific environment
func SaveToken(envName string, token *Token) error {
	if err := os.MkdirAll(config.GetConfigDir(), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(tokenFilePath(envName), data, 0600)
}

// DeleteToken removes a token file for an environment
func DeleteToken(envName string) error {
	path := tokenFilePath(envName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
