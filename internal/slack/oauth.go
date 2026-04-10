package slack

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/store"
)

const (
	defaultSlackOAuthURL = "https://slack.com/oauth/v2/authorize"
	defaultSlackAPIURL   = "https://slack.com/api/"
)

// OAuthHandler handles the Slack OAuth install flow.
type OAuthHandler struct {
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       string
	store        store.Store
	oauthURL     string // authorize URL (overridable for testing)
	apiURL       string // API base URL (overridable for testing)
}

// OAuthConfig configures the Slack OAuth handler.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Store        store.Store
	OAuthURL     string // optional, defaults to Slack's URL
	APIURL       string // optional, defaults to Slack's URL
}

func NewOAuthHandler(cfg OAuthConfig) *OAuthHandler {
	oauthURL := cfg.OAuthURL
	if oauthURL == "" {
		oauthURL = defaultSlackOAuthURL
	}
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = defaultSlackAPIURL
	}
	return &OAuthHandler{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURI:  cfg.RedirectURI,
		scopes:       "commands,chat:write,chat:write.public,channels:read,groups:read,im:write,im:read",
		store:        cfg.Store,
		oauthURL:     oauthURL,
		apiURL:       apiURL,
	}
}

// HandleInstall redirects the user to Slack's OAuth authorization page.
func (h *OAuthHandler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	params := url.Values{
		"client_id":    {h.clientID},
		"scope":        {h.scopes},
		"redirect_uri": {h.redirectURI},
	}
	authorizeURL := h.oauthURL + "?" + params.Encode()
	log.WithField("redirect_uri", h.redirectURI).Debug("starting Slack OAuth install flow")
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// HandleCallback exchanges the OAuth code for a bot token and creates the tenant.
func (h *OAuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	log.Debug("received Slack OAuth callback")
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	errParam := r.URL.Query().Get("error")
	if errParam != "" {
		http.Error(w, fmt.Sprintf("Slack error: %s", errParam), http.StatusBadRequest)
		return
	}

	resp, err := h.exchangeCode(code)
	if err != nil {
		log.WithError(err).Error("failed to exchange OAuth code")
		http.Error(w, "OAuth exchange failed", http.StatusInternalServerError)
		return
	}

	if !resp.OK {
		log.WithField("error", resp.Error).Error("Slack OAuth response not ok")
		http.Error(w, fmt.Sprintf("Slack error: %s", resp.Error), http.StatusBadRequest)
		return
	}

	apiKey, err := generateOAuthAPIKey()
	if err != nil {
		log.WithError(err).Error("failed to generate API key")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	botToken := resp.AccessToken
	teamID := resp.Team.ID
	teamName := resp.Team.Name

	tenant := &store.Tenant{
		ID:          teamID,
		TeamName:    teamName,
		BotToken:    &botToken,
		APIKey:      apiKey,
		InstalledAt: time.Now(),
	}

	// Check if tenant already exists (re-install)
	existing, err := h.store.GetTenant(r.Context(), teamID)
	if err != nil {
		log.WithError(err).Error("failed to check existing tenant")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if existing != nil {
		// Re-install: update the bot token
		existing.BotToken = &botToken
		existing.TeamName = teamName
		if err := h.store.UpdateTenant(r.Context(), existing); err != nil {
			log.WithError(err).Error("failed to update tenant on re-install")
			http.Error(w, "failed to update tenant", http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.store.CreateTenant(r.Context(), tenant); err != nil {
			log.WithError(err).Error("failed to create tenant from OAuth")
			http.Error(w, "failed to create tenant", http.StatusInternalServerError)
			return
		}
	}

	// Make the installing user an admin
	if authedUserID := resp.AuthedUser.ID; authedUserID != "" {
		if err := h.store.AddAdmin(r.Context(), teamID, authedUserID); err != nil {
			log.WithError(err).WithField("user_id", authedUserID).Warn("failed to add installing user as admin (may already be admin)")
		} else {
			log.WithFields(log.Fields{
				"team_id": teamID,
				"user_id": authedUserID,
			}).Info("added installing user as admin")
		}
	}

	log.WithFields(log.Fields{
		"team_id":   teamID,
		"team_name": teamName,
	}).Info("Slack app installed successfully")

	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>zoom-notifier — Installed</title>
<style>
  body { font-family: system-ui, -apple-system, sans-serif; max-width: 560px; margin: 2rem auto; padding: 0 1rem; color: #333; background: #fafafa; }
  .container { background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 2rem; text-align: center; }
  h1 { color: #1a73e8; margin-bottom: 0.5rem; }
  .team-name { font-size: 1.1rem; color: #666; margin-bottom: 1.5rem; }
  .next-steps { text-align: left; background: #f0f7ff; border: 1px solid #c2deff; border-radius: 6px; padding: 1rem 1.25rem; margin: 1.5rem 0; }
  .next-steps h3 { margin-top: 0; color: #1a73e8; }
  .next-steps li { margin: 0.4rem 0; }
  .btn { display: inline-block; padding: 0.6rem 1.5rem; background: #1a73e8; color: #fff; text-decoration: none; border-radius: 4px; font-size: 1rem; margin-top: 1rem; }
  .btn:hover { background: #1557b0; }
  code { background: #f0f0f0; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.9rem; }
</style>
</head>
<body>
<div class="container">
  <h1>zoom-notifier installed</h1>
  <p class="team-name">%s</p>
  <div class="next-steps">
    <h3>Next steps</h3>
    <ol>
      <li>Run <code>/zoom-notifier setup</code> in Slack to configure your Zoom account</li>
      <li>Subscribe channels with <code>/zoom-notifier subscribe #channel</code></li>
      <li>Optionally add filters with <code>/zoom-notifier filter "Topic"</code></li>
    </ol>
  </div>
  <a href="/" class="btn">Back to zoom-notifier</a>
</div>
</body>
</html>`, teamName)
}

// oauthV2Response is the response from Slack's oauth.v2.access endpoint.
type oauthV2Response struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Team        struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	AuthedUser struct {
		ID string `json:"id"`
	} `json:"authed_user"`
}

func (h *OAuthHandler) exchangeCode(code string) (*oauthV2Response, error) {
	data := url.Values{
		"client_id":     {h.clientID},
		"client_secret": {h.clientSecret},
		"code":          {code},
		"redirect_uri":  {h.redirectURI},
	}

	resp, err := http.PostForm(h.apiURL+"oauth.v2.access", data)
	if err != nil {
		return nil, fmt.Errorf("POST oauth.v2.access: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result oauthV2Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode oauth response: %w", err)
	}
	return &result, nil
}

func generateOAuthAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(b), nil
}
