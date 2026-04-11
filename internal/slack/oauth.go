package slack

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/setup"
	"github.com/stahnma/zoom-notifier/internal/store"
)

//go:embed templates/install_success.html
var installSuccessFS embed.FS

var installSuccessTmpl = template.Must(template.ParseFS(installSuccessFS, "templates/install_success.html"))

const (
	defaultSlackOAuthURL = "https://slack.com/oauth/v2/authorize"
	defaultSlackAPIURL   = "https://slack.com/api/"
	// slackOAuthTimeout is the timeout for Slack OAuth token exchange requests.
	slackOAuthTimeout = 30 * time.Second
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
	httpClient   *http.Client
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
		httpClient:   &http.Client{Timeout: slackOAuthTimeout},
	}
}

// HandleInstall redirects the user to Slack's OAuth authorization page.
func (h *OAuthHandler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		log.WithError(err).Error("failed to generate OAuth state token")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	params := url.Values{
		"client_id":    {h.clientID},
		"scope":        {h.scopes},
		"redirect_uri": {h.redirectURI},
		"state":        {state},
	}
	authorizeURL := h.oauthURL + "?" + params.Encode()
	log.WithField("redirect_uri", h.redirectURI).Debug("starting Slack OAuth install flow")
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// HandleCallback exchanges the OAuth code for a bot token and creates the tenant.
func (h *OAuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	log.Debug("received Slack OAuth callback")

	// Verify OAuth state parameter to prevent CSRF
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value == "" {
		http.Error(w, "missing OAuth state cookie", http.StatusBadRequest)
		return
	}
	stateParam := r.URL.Query().Get("state")
	if stateParam == "" || stateParam != stateCookie.Value {
		http.Error(w, "OAuth state mismatch", http.StatusBadRequest)
		return
	}
	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := installSuccessTmpl.Execute(w, struct{ TeamName string }{teamName}); err != nil {
		log.WithError(err).Warn("failed to write install success page")
	}
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

	resp, err := h.httpClient.PostForm(h.apiURL+"oauth.v2.access", data)
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
	return setup.GenerateAPIKey()
}
