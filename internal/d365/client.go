package d365

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrUnauthorized is returned when the API returns a 401 status
var ErrUnauthorized = errors.New("unauthorized: token may be expired")

// TokenRefreshFunc is a callback function that attempts to refresh the token
// It should return the new access token or an error
type TokenRefreshFunc func() (string, error)

// Client represents a Dynamics 365 Web API client
type Client struct {
	baseURL      string
	accessToken  string
	httpClient   *http.Client
	tokenRefresh TokenRefreshFunc
}

// NewClient creates a new Dynamics 365 client
func NewClient(orgURL, accessToken string) *Client {
	return &Client{
		baseURL:     orgURL + "/api/data/v9.2",
		accessToken: accessToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetTokenRefreshFunc sets the callback function for token refresh
func (c *Client) SetTokenRefreshFunc(fn TokenRefreshFunc) {
	c.tokenRefresh = fn
}

// UpdateToken updates the access token
func (c *Client) UpdateToken(token string) {
	c.accessToken = token
}

// doRequest performs an HTTP request with authorization
func (c *Client) doRequest(method, path string, body any) ([]byte, error) {
	return c.doRequestWithRetry(method, path, body, true)
}

// doRequestWithRetry performs an HTTP request with optional token refresh retry
func (c *Client) doRequestWithRetry(method, path string, body any, allowRetry bool) ([]byte, error) {
	// Store body for potential retry
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	var reqBody io.Reader
	if bodyBytes != nil {
		reqBody = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OData-MaxVersion", "4.0")
	req.Header.Set("OData-Version", "4.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Handle 401 Unauthorized - attempt token refresh
	if resp.StatusCode == http.StatusUnauthorized {
		if allowRetry && c.tokenRefresh != nil {
			newToken, refreshErr := c.tokenRefresh()
			if refreshErr == nil && newToken != "" {
				c.accessToken = newToken
				// Retry the request with the new token
				return c.doRequestWithRetry(method, path, body, false)
			}
		}
		return nil, ErrUnauthorized
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
