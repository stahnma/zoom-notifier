package slack

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/store"
)

// TenantSetupHandler serves a web form for linking a tenant to its Zoom account
// and setting that account's API credentials (self-service; see
// store.SelfServiceZoomAccount for the rules).
type TenantSetupHandler struct {
	store store.Store
	tmpl  *template.Template
}

// NewTenantSetupHandler creates a new TenantSetupHandler.
func NewTenantSetupHandler(s store.Store) *TenantSetupHandler {
	tmpl := template.Must(template.New("tenant_setup").Parse(tenantSetupTemplate))
	return &TenantSetupHandler{store: s, tmpl: tmpl}
}

// ServeHTTP handles GET and POST requests for the tenant setup page.
func (h *TenantSetupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("key")
	if apiKey == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	tenant, err := h.findTenantByAPIKey(r, apiKey)
	if err != nil {
		log.WithError(err).Error("failed to look up tenants")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if tenant == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	log.WithField("tenant_id", tenant.ID).Debug("tenant setup page accessed")

	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, tenant, apiKey)
	case http.MethodPost:
		h.handlePost(w, r, tenant, apiKey)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *TenantSetupHandler) findTenantByAPIKey(r *http.Request, apiKey string) (*store.Tenant, error) {
	tenants, err := h.store.ListTenants(r.Context())
	if err != nil {
		return nil, err
	}
	for _, t := range tenants {
		if t.APIKey == apiKey {
			return t, nil
		}
	}
	return nil, nil
}

type tenantSetupData struct {
	TeamName        string
	ZoomAccountID   string
	ZoomClientID    string
	HasClientSecret bool // the secret itself is never rendered
	Managed         bool // account is operator-managed; form is read-only
	APIKey          string
	Success         string
	Error           string
}

// loadAccount fills the Zoom account fields of data for accountID.
func (h *TenantSetupHandler) loadAccount(r *http.Request, tenant *store.Tenant, accountID string, data *tenantSetupData) error {
	if accountID == "" {
		return nil
	}
	account, err := h.store.GetZoomAccount(r.Context(), accountID)
	if err != nil {
		return fmt.Errorf("get zoom account: %w", err)
	}
	if account != nil {
		data.ZoomClientID = account.ClientID
		data.HasClientSecret = account.ClientSecret != ""
		data.Managed = account.WebhookSecret != ""
	}
	linked, err := h.store.GetTenantByZoomAccount(r.Context(), accountID)
	if err != nil {
		return fmt.Errorf("list tenants for zoom account: %w", err)
	}
	for _, t := range linked {
		if t.ID != tenant.ID {
			data.Managed = true
		}
	}
	return nil
}

func (h *TenantSetupHandler) render(w http.ResponseWriter, data tenantSetupData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.Execute(w, data); err != nil {
		log.WithError(err).Error("failed to render tenant setup template")
	}
}

func (h *TenantSetupHandler) handleGet(w http.ResponseWriter, r *http.Request, tenant *store.Tenant, apiKey string) {
	data := tenantSetupData{
		TeamName:      tenant.TeamName,
		ZoomAccountID: tenant.ZoomAccountID,
		APIKey:        apiKey,
	}
	if err := h.loadAccount(r, tenant, tenant.ZoomAccountID, &data); err != nil {
		log.WithError(err).WithField("tenant_id", tenant.ID).Error("failed to load zoom account for setup page")
		data.Error = "Failed to load Zoom settings."
	}
	h.render(w, data)
}

func (h *TenantSetupHandler) handlePost(w http.ResponseWriter, r *http.Request, tenant *store.Tenant, apiKey string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	accountID := strings.TrimSpace(r.FormValue("zoom_account_id"))
	clientID := strings.TrimSpace(r.FormValue("zoom_client_id"))
	clientSecret := r.FormValue("zoom_client_secret")

	data := tenantSetupData{
		TeamName:      tenant.TeamName,
		ZoomAccountID: accountID,
		ZoomClientID:  clientID,
		APIKey:        apiKey,
	}

	if accountID == "" {
		log.WithField("tenant_id", tenant.ID).Warn("tenant setup validation failed: missing Zoom Account ID")
		data.Error = "Zoom Account ID is required."
		h.render(w, data)
		return
	}

	err := store.SelfServiceZoomAccount(r.Context(), h.store, tenant, accountID, clientID, clientSecret)
	if errors.Is(err, store.ErrZoomAccountManaged) {
		log.WithFields(log.Fields{"tenant_id": tenant.ID, "account_id": accountID}).Warn("tenant setup refused: zoom account is operator-managed")
		// Show the tenant's actual (unchanged) account, not the one it tried.
		data = tenantSetupData{TeamName: tenant.TeamName, ZoomAccountID: tenant.ZoomAccountID, APIKey: apiKey}
		if loadErr := h.loadAccount(r, tenant, tenant.ZoomAccountID, &data); loadErr != nil {
			log.WithError(loadErr).WithField("tenant_id", tenant.ID).Warn("failed to reload zoom account for setup page")
		}
		data.Error = fmt.Sprintf("Zoom account %s is managed by the operator (shared with other workspaces or has its own webhook secret). Ask the operator to link this workspace to it.", accountID)
		h.render(w, data)
		return
	}
	if loadErr := h.loadAccount(r, tenant, accountID, &data); loadErr != nil {
		log.WithError(loadErr).WithField("tenant_id", tenant.ID).Warn("failed to reload zoom account for setup page")
	}
	switch {
	case err != nil:
		log.WithError(err).WithField("tenant_id", tenant.ID).Error("failed to save tenant zoom settings")
		data.Error = "Failed to save Zoom settings."
	default:
		log.WithField("tenant_id", tenant.ID).Info("tenant Zoom credentials updated via setup page")
		data.Success = "Zoom settings saved successfully."
	}
	h.render(w, data)
}

const tenantSetupTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Zoom Setup — {{.TeamName}}</title>
<style>
  body { font-family: system-ui, -apple-system, sans-serif; max-width: 560px; margin: 2rem auto; padding: 0 1rem; color: #333; background: #fafafa; }
  h1 { margin-bottom: 0.25rem; }
  .subtitle { color: #666; margin-top: 0; }
  form { background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 1.5rem; }
  label { display: block; font-weight: 600; margin-top: 1rem; margin-bottom: 0.25rem; }
  input[type="text"], input[type="password"] { width: 100%; padding: 0.5rem; border: 1px solid #ccc; border-radius: 4px; font-size: 0.95rem; box-sizing: border-box; }
  .help { font-size: 0.85rem; color: #666; margin-top: 0.25rem; }
  button { margin-top: 1.5rem; padding: 0.6rem 1.5rem; background: #1a73e8; color: #fff; border: none; border-radius: 4px; font-size: 1rem; cursor: pointer; }
  button:hover { background: #1557b0; }
  .success { background: #e6f4ea; border: 1px solid #34a853; color: #137333; padding: 0.75rem; border-radius: 4px; margin-bottom: 1rem; }
  .error { background: #fce8e6; border: 1px solid #ea4335; color: #c5221f; padding: 0.75rem; border-radius: 4px; margin-bottom: 1rem; }
</style>
</head>
<body>
<h1>Zoom Setup</h1>
<p class="subtitle">{{.TeamName}}</p>

{{if .Success}}
<div class="success">{{.Success}}</div>
<button onclick="window.close()" style="margin-bottom:1.5rem;">Done — Close This Tab</button>
<p class="help">If the button doesn't work, you can safely close this tab.</p>
{{end}}
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}

{{if .Managed}}
<div class="error">This Zoom account is managed by the operator (it is shared with other workspaces or has its own webhook secret). Ask the operator to change its settings.</div>
<form>
  <label>Zoom Account ID</label>
  <input type="text" value="{{.ZoomAccountID}}" disabled>
</form>
{{else}}
<form method="POST" action="/tenant/setup?key={{.APIKey}}">
  <label for="zoom_account_id">Zoom Account ID</label>
  <input type="text" id="zoom_account_id" name="zoom_account_id" value="{{.ZoomAccountID}}" required>
  <p class="help">Your Zoom Account ID. Found in the Zoom App Marketplace under your Server-to-Server OAuth app's settings.</p>

  <label for="zoom_client_id">Zoom Client ID</label>
  <input type="text" id="zoom_client_id" name="zoom_client_id" value="{{.ZoomClientID}}">
  <p class="help">Optional. The Client ID from your Zoom Server-to-Server OAuth app. Required to fetch meeting join links.</p>

  <label for="zoom_client_secret">Zoom Client Secret</label>
  <input type="password" id="zoom_client_secret" name="zoom_client_secret" value="" placeholder="{{if .HasClientSecret}}(unchanged){{end}}" autocomplete="off">
  <p class="help">Optional. The Client Secret from your Zoom Server-to-Server OAuth app. Required to fetch meeting join links.{{if .HasClientSecret}} Leave blank to keep the current secret.{{end}}</p>

  <button type="submit">Save</button>
</form>
{{end}}

<p style="margin-top:1.5rem;font-size:0.85rem;color:#999;">Do not share this page URL — it contains your API key.</p>
</body>
</html>`
