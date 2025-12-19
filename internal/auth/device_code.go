package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientID      = "51f81489-12ee-4a9e-aaae-a2591f45987d"
	authorityBase = "https://login.microsoftonline.com/common/oauth2/v2.0"
)

// DeviceCodeResponse represents the device code response from Azure AD
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// TokenResponse represents the token response from Azure AD
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// RequestDeviceCode initiates the device code flow
func RequestDeviceCode(orgURL string) (*DeviceCodeResponse, error) {
	scope := orgURL + "/.default"

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("scope", scope)

	resp, err := http.Post(
		authorityBase+"/devicecode",
		"application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed: %s", string(body))
	}

	var dcResp DeviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, err
	}

	return &dcResp, nil
}

// PollForToken polls for token after user authenticates
func PollForToken(deviceCode string, orgURL string, interval int) (*Token, error) {
	scope := orgURL + "/.default"

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("device_code", deviceCode)
	data.Set("scope", scope)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-timeout:
			return nil, errors.New("authentication timed out")
		case <-ticker.C:
			resp, err := http.Post(
				authorityBase+"/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(data.Encode()),
			)
			if err != nil {
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			var tokenResp TokenResponse
			if err := json.Unmarshal(body, &tokenResp); err != nil {
				continue
			}

			if tokenResp.Error == "authorization_pending" {
				continue
			}

			if tokenResp.Error != "" {
				return nil, fmt.Errorf("token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
			}

			if tokenResp.AccessToken != "" {
				return &Token{
					AccessToken:  tokenResp.AccessToken,
					RefreshToken: tokenResp.RefreshToken,
					ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
				}, nil
			}
		}
	}
}

// RefreshAccessToken refreshes an expired token
func RefreshAccessToken(refreshToken, orgURL string) (*Token, error) {
	scope := orgURL + "/.default"

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("scope", scope)

	resp, err := http.Post(
		authorityBase+"/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("refresh error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}
