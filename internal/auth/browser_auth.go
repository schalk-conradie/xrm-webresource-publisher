package auth

import (
	"context"
	"fmt"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

const (
	ClientID      = "51f81489-12ee-4a9e-aaae-a2591f45987d"
	RedirectURL   = "http://localhost:8400"
	AuthorityBase = "https://login.microsoftonline.com/common"
)

// AcquireTokenInteractive authenticates the user via browser and returns a token
func AcquireTokenInteractive(orgURL string) (*Token, error) {
	scope := orgURL + "/.default"

	app, err := public.New(ClientID, public.WithAuthority(AuthorityBase))
	if err != nil {
		return nil, fmt.Errorf("failed to create public client: %w", err)
	}

	// Check if we have cached accounts
	accounts, err := app.Accounts(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	var result public.AuthResult

	// Try silent authentication first if we have accounts
	if len(accounts) > 0 {
		result, err = app.AcquireTokenSilent(context.Background(), []string{scope}, public.WithSilentAccount(accounts[0]))
		if err == nil {
			return convertAuthResult(result), nil
		}
		// If silent auth fails, fall through to interactive auth
	}

	// Acquire token interactively - MSAL will handle opening browser and local server
	result, err = app.AcquireTokenInteractive(
		context.Background(),
		[]string{scope},
		public.WithRedirectURI(RedirectURL),
	)
	if err != nil {
		return nil, fmt.Errorf("interactive authentication failed: %w", err)
	}

	return convertAuthResult(result), nil
}

// RefreshAccessToken refreshes an expired token using MSAL
func RefreshAccessToken(refreshToken, orgURL string) (*Token, error) {
	scope := orgURL + "/.default"

	// Create public client application
	app, err := public.New(ClientID, public.WithAuthority(AuthorityBase))
	if err != nil {
		return nil, fmt.Errorf("failed to create public client: %w", err)
	}

	// Get accounts from cache
	accounts, err := app.Accounts(context.Background())
	if err != nil || len(accounts) == 0 {
		return nil, fmt.Errorf("no cached accounts found, re-authentication required")
	}

	// Try to acquire token silently
	result, err := app.AcquireTokenSilent(
		context.Background(),
		[]string{scope},
		public.WithSilentAccount(accounts[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}

	return convertAuthResult(result), nil
}

// convertAuthResult converts MSAL AuthResult to our Token type
func convertAuthResult(result public.AuthResult) *Token {
	return &Token{
		AccessToken:  result.AccessToken,
		RefreshToken: "",
		ExpiresAt:    result.ExpiresOn,
	}
}
