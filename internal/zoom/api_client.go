package zoom

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/stahnma/zoom-notifier/internal/metrics"
)

const (
	// oauthTimeout is the timeout for OAuth token exchange requests.
	oauthTimeout = 30 * time.Second
	// apiTimeout is the timeout for Zoom API requests (meeting info, etc).
	apiTimeout = 10 * time.Second
)

type APIClient struct {
	oauthBaseURL string
	apiBaseURL   string
	clientID     string
	clientSecret string
	accountID    string

	tokenClient *http.Client // HTTP client for OAuth token requests
	apiClient   *http.Client // HTTP client for API requests

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type meetingResponse struct {
	JoinURL string `json:"join_url"`
}

// NewAPIClient creates a Zoom API client with configurable base URLs for testing.
// For production use: NewAPIClient("https://zoom.us", "https://api.zoom.us", clientID, clientSecret, accountID)
func NewAPIClient(oauthBaseURL, apiBaseURL, clientID, clientSecret, accountID string) *APIClient {
	return &APIClient{
		oauthBaseURL: oauthBaseURL,
		apiBaseURL:   apiBaseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		accountID:    accountID,
		tokenClient:  &http.Client{Timeout: oauthTimeout},
		apiClient:    &http.Client{Timeout: apiTimeout},
	}
}

// GetAccessToken returns a cached token or fetches a new one.
func (c *APIClient) GetAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with 60s buffer)
	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry.Add(-60*time.Second)) {
		return c.cachedToken, nil
	}

	authString := fmt.Sprintf("%s:%s", c.clientID, c.clientSecret)
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))

	data := url.Values{}
	data.Set("grant_type", "account_credentials")
	data.Set("account_id", c.accountID)

	req, err := http.NewRequest(http.MethodPost, c.oauthBaseURL+"/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+authEncoded)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.tokenClient.Do(req)
	if err != nil {
		metrics.ZoomTokenRefreshes.WithLabelValues("error").Inc()
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		metrics.ZoomTokenRefreshes.WithLabelValues("error").Inc()
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("token API error (status %d), failed to read body: %w", resp.StatusCode, readErr)
		}
		return "", fmt.Errorf("token API error (status %d): %s", resp.StatusCode, body)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		metrics.ZoomTokenRefreshes.WithLabelValues("error").Inc()
		return "", fmt.Errorf("decode token response: %w", err)
	}

	metrics.ZoomTokenRefreshes.WithLabelValues("success").Inc()
	c.cachedToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return c.cachedToken, nil
}

// GetMeetingJoinLink fetches the join URL for a meeting.
func (c *APIClient) GetMeetingJoinLink(meetingID string) (string, error) {
	token, err := c.GetAccessToken()
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v2/meetings/%s", c.apiBaseURL, meetingID), nil)
	if err != nil {
		return "", fmt.Errorf("create meeting request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.apiClient.Do(req)
	if err != nil {
		metrics.ZoomAPIRequests.WithLabelValues("meetings", "error").Inc()
		return "", fmt.Errorf("meeting request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		metrics.ZoomAPIRequests.WithLabelValues("meetings", "error").Inc()
		return "", fmt.Errorf("meeting API error: status %d", resp.StatusCode)
	}
	metrics.ZoomAPIRequests.WithLabelValues("meetings", "success").Inc()

	var meetingResp meetingResponse
	if err := json.NewDecoder(resp.Body).Decode(&meetingResp); err != nil {
		return "", fmt.Errorf("decode meeting response: %w", err)
	}

	return meetingResp.JoinURL, nil
}
