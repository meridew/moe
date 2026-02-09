package intune

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenCache handles OAuth2 client credentials token acquisition and caching
// for Microsoft Entra ID (Azure AD).
type tokenCache struct {
	tenantID     string
	clientID     string
	clientSecret string

	mu      sync.Mutex
	token   string
	expires time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func newTokenCache(tenantID, clientID, clientSecret string) *tokenCache {
	return &tokenCache{
		tenantID:     tenantID,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// Token returns a valid access token, refreshing if expired or missing.
func (tc *tokenCache) Token() (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Return cached token if still valid (with 2 min buffer).
	if tc.token != "" && time.Now().Before(tc.expires.Add(-2*time.Minute)) {
		return tc.token, nil
	}

	token, expiresIn, err := tc.fetchToken()
	if err != nil {
		return "", err
	}

	tc.token = token
	tc.expires = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return tc.token, nil
}

func (tc *tokenCache) fetchToken() (string, int, error) {
	endpoint := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/token",
		tc.tenantID,
	)

	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {tc.clientID},
		"client_secret": {tc.clientSecret},
		"scope":         {"https://graph.microsoft.com/.default"},
	}

	resp, err := http.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("token error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, fmt.Errorf("parse token response: %w", err)
	}

	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("empty access token in response")
	}

	return tr.AccessToken, tr.ExpiresIn, nil
}
