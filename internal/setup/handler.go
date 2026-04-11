package setup

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/zoom"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Handler serves the setup wizard web UI.
type Handler struct {
	configPath string
	templates  map[string]*template.Template
	data       *SetupData
}

// NewHandler creates a new setup wizard handler that will write configuration
// to the given configPath.
func NewHandler(configPath string) *Handler {
	// Parse each page template individually with the layout so that each
	// page gets its own "content" definition. Parsing all files together
	// would cause the last-defined "content" block to win.
	pages := []string{
		"welcome.html",
		"zoom.html",
		"slack.html",
		"advanced.html",
		"review.html",
		"complete.html",
	}
	funcMap := template.FuncMap{
		"stepClass": func(stepIndex, currentStep int) string {
			if stepIndex == currentStep {
				return "active"
			}
			if stepIndex < currentStep {
				return "completed"
			}
			return ""
		},
	}
	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		templates[page] = template.Must(
			template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+page),
		)
	}

	apiKey, _ := GenerateAPIKey()
	return &Handler{
		configPath: configPath,
		templates:  templates,
		data: &SetupData{
			AdminAPIKey:  apiKey,
			ServerHost:   "localhost",
			ServerPort:   8888,
			DatabasePath: "./zoom-notifier.db",
			LogLevel:     "info",
		},
	}
}

// Router returns an http.Handler with all setup wizard routes mounted.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	// Static assets
	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/setup/static/*", http.StripPrefix("/setup/static/", http.FileServer(http.FS(staticSub))))

	// Redirect root to setup wizard
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/setup", http.StatusFound)
	})

	// Wizard pages
	r.Get("/setup", h.welcome)
	r.Get("/setup/welcome", h.welcome)
	r.Post("/setup/server-url", h.handleServerURL)
	r.Post("/setup/zoom", h.handleZoom)
	r.Post("/setup/slack", h.handleSlack)
	r.Post("/setup/advanced", h.handleAdvanced)
	r.Post("/setup/save", h.handleSave)

	// JS API
	r.Get("/setup/api/generate-key", h.generateKey)
	r.Get("/setup/api/manifest-url", h.manifestURL)
	r.Post("/setup/api/save-zoom-secret", h.saveZoomSecret)

	// Zoom CRC validation — allows Zoom to validate the webhook endpoint during setup
	r.Post("/webhook/zoom", h.handleWebhookZoomCRC)

	return r
}

// pageData wraps template data with the current step number for the progress stepper.
type pageData struct {
	Step        int // 0=welcome, 1=zoom, 2=slack, 3=advanced, 4=review, 5=complete
	Data        *SetupData
	ManifestURL string // only set for the Slack step
}

func (h *Handler) render(w http.ResponseWriter, name string, pd pageData) {
	tmpl, ok := h.templates[name]
	if !ok {
		log.Errorf("Template %q not found", name)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, pd); err != nil {
		log.WithError(err).Error("Failed to render template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) welcome(w http.ResponseWriter, r *http.Request) {
	h.render(w, "welcome.html", pageData{Step: 0, Data: h.data})
}

func (h *Handler) handleServerURL(w http.ResponseWriter, r *http.Request) {
	h.data.ServerURL = r.FormValue("server_url")
	h.render(w, "zoom.html", pageData{Step: 1, Data: h.data})
}

func (h *Handler) handleZoom(w http.ResponseWriter, r *http.Request) {
	h.data.ZoomSecret = r.FormValue("zoom_secret")
	h.data.ZoomAccountID = r.FormValue("zoom_account_id")
	h.data.ZoomClientID = r.FormValue("zoom_client_id")
	h.data.ZoomClientSecret = r.FormValue("zoom_client_secret")
	h.render(w, "slack.html", pageData{
		Step:        2,
		Data:        h.data,
		ManifestURL: SlackManifestURL(h.data.ServerURL),
	})
}

func (h *Handler) handleSlack(w http.ResponseWriter, r *http.Request) {
	h.data.SlackClientID = r.FormValue("slack_client_id")
	h.data.SlackClientSecret = r.FormValue("slack_client_secret")
	h.data.SlackSigningSecret = r.FormValue("slack_signing_secret")
	h.render(w, "advanced.html", pageData{Step: 3, Data: h.data})
}

func (h *Handler) handleAdvanced(w http.ResponseWriter, r *http.Request) {
	h.data.ServerHost = r.FormValue("server_host")
	if port, err := strconv.Atoi(r.FormValue("server_port")); err == nil {
		h.data.ServerPort = port
	}
	h.data.DatabasePath = r.FormValue("database_path")
	h.data.LogLevel = r.FormValue("log_level")
	// Pick up optional admin API key override from advanced settings
	if key := r.FormValue("admin_api_key"); key != "" {
		h.data.AdminAPIKey = key
	}
	h.render(w, "review.html", pageData{Step: 4, Data: h.data})
}

func (h *Handler) handleSave(w http.ResponseWriter, r *http.Request) {
	// Validate required fields before writing
	var missing []string
	if h.data.AdminAPIKey == "" {
		missing = append(missing, "Admin API Key")
	}
	if h.data.ZoomSecret == "" {
		missing = append(missing, "Zoom Webhook Secret")
	}
	if h.data.ZoomAccountID == "" {
		missing = append(missing, "Zoom Account ID")
	}
	if h.data.SlackClientID == "" {
		missing = append(missing, "Slack Client ID")
	}
	if h.data.SlackClientSecret == "" {
		missing = append(missing, "Slack Client Secret")
	}
	if h.data.SlackSigningSecret == "" {
		missing = append(missing, "Slack Signing Secret")
	}
	if len(missing) > 0 {
		log.WithField("missing", missing).Warn("Setup wizard submitted with missing fields")
		http.Error(w, "Missing required fields: "+strings.Join(missing, ", "), http.StatusBadRequest)
		return
	}

	if err := WriteConfig(h.configPath, h.data); err != nil {
		log.WithError(err).Error("Failed to write config")
		http.Error(w, "Failed to save configuration. Check that the path is writable.", http.StatusInternalServerError)
		return
	}
	h.render(w, "complete.html", pageData{Step: 5, Data: h.data})
}

func (h *Handler) generateKey(w http.ResponseWriter, r *http.Request) {
	key, err := GenerateAPIKey()
	if err != nil {
		http.Error(w, "Failed to generate key", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"key": key})
}

func (h *Handler) manifestURL(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server_url")
	if serverURL == "" {
		serverURL = h.data.ServerURL
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": SlackManifestURL(serverURL)})
}

func (h *Handler) saveZoomSecret(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Secret == "" {
		http.Error(w, "missing secret", http.StatusBadRequest)
		return
	}
	h.data.ZoomSecret = body.Secret
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) handleWebhookZoomCRC(w http.ResponseWriter, r *http.Request) {
	if h.data.ZoomSecret == "" {
		log.Warn("Zoom CRC validation attempted but no secret configured yet")
		http.Error(w, "webhook secret not configured", http.StatusServiceUnavailable)
		return
	}

	var payload zoom.WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	if payload.Event != zoom.EventURLValidation {
		// During setup, only handle CRC validation — ignore all other events
		w.WriteHeader(http.StatusOK)
		return
	}

	resp, err := zoom.ValidateCRC(payload, h.data.ZoomSecret)
	if err != nil {
		log.WithError(err).Error("CRC validation failed during setup")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
