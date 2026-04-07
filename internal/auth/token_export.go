package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type exportedToken struct {
	AccessToken string `json:"accessToken"`
	ExpiresOn   string `json:"expiresOn"`
	ExpiresUnix int64  `json:"expires_on"`
	Tenant      string `json:"tenant"`
	TokenType   string `json:"tokenType"`
}

type tokenClaims struct {
	TenantID string `json:"tid"`
}

// ExportAccessToken writes a token.json file compatible with the Azure CLI JSON shape.
func ExportAccessToken(rootDir string, token *Token) error {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return errors.New("token export directory is empty")
	}
	if token == nil || token.AccessToken == "" {
		return errors.New("token is empty")
	}

	claims, err := parseTokenClaims(token.AccessToken)
	if err != nil {
		return fmt.Errorf("parse token claims: %w", err)
	}
	if claims.TenantID == "" {
		return errors.New("token is missing tenant claim")
	}

	if err := os.MkdirAll(rootDir, 0700); err != nil {
		return err
	}

	export := exportedToken{
		AccessToken: token.AccessToken,
		ExpiresOn:   token.ExpiresAt.Local().Format("2006-01-02 15:04:05.000000"),
		ExpiresUnix: token.ExpiresAt.Unix(),
		Tenant:      claims.TenantID,
		TokenType:   "Bearer",
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(rootDir, "token.json"), data, 0600)
}

func parseTokenClaims(accessToken string) (*tokenClaims, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid jwt format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}

	return &claims, nil
}
