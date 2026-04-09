package setup

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	log "github.com/sirupsen/logrus"
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
		"admin-key.html",
		"zoom.html",
		"slack.html",
		"advanced.html",
		"review.html",
		"complete.html",
	}
	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		templates[page] = template.Must(
			template.ParseFS(templateFS, "templates/layout.html", "templates/"+page),
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

	// Wizard pages
	r.Get("/setup", h.welcome)
	r.Get("/setup/welcome", h.welcome)
	r.Post("/setup/server-url", h.handleServerURL)
	r.Post("/setup/admin-key", h.handleAdminKey)
	r.Post("/setup/zoom", h.handleZoom)
	r.Post("/setup/slack", h.handleSlack)
	r.Post("/setup/advanced", h.handleAdvanced)
	r.Post("/setup/save", h.handleSave)

	// JS API
	r.Get("/setup/api/generate-key", h.generateKey)
	r.Get("/setup/api/manifest-url", h.manifestURL)

	return r
}

func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	tmpl, ok := h.templates[name]
	if !ok {
		log.Errorf("Template %q not found", name)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.WithError(err).Error("Failed to render template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) welcome(w http.ResponseWriter, r *http.Request) {
	h.render(w, "welcome.html", h.data)
}

func (h *Handler) handleServerURL(w http.ResponseWriter, r *http.Request) {
	h.data.ServerURL = r.FormValue("server_url")
	h.render(w, "admin-key.html", h.data)
}

func (h *Handler) handleAdminKey(w http.ResponseWriter, r *http.Request) {
	h.data.AdminAPIKey = r.FormValue("admin_api_key")
	h.render(w, "zoom.html", h.data)
}

func (h *Handler) handleZoom(w http.ResponseWriter, r *http.Request) {
	h.data.ZoomSecret = r.FormValue("zoom_secret")

	// slack.html expects .ManifestURL and .Data (SetupData)
	type slackPageData struct {
		ManifestURL string
		Data        *SetupData
	}
	h.render(w, "slack.html", slackPageData{
		ManifestURL: SlackManifestURL(h.data.ServerURL),
		Data:        h.data,
	})
}

func (h *Handler) handleSlack(w http.ResponseWriter, r *http.Request) {
	h.data.SlackClientID = r.FormValue("slack_client_id")
	h.data.SlackClientSecret = r.FormValue("slack_client_secret")
	h.data.SlackSigningSecret = r.FormValue("slack_signing_secret")
	h.render(w, "advanced.html", h.data)
}

func (h *Handler) handleAdvanced(w http.ResponseWriter, r *http.Request) {
	h.data.ServerHost = r.FormValue("server_host")
	if port, err := strconv.Atoi(r.FormValue("server_port")); err == nil {
		h.data.ServerPort = port
	}
	h.data.DatabasePath = r.FormValue("database_path")
	h.data.LogLevel = r.FormValue("log_level")
	h.render(w, "review.html", h.data)
}

func (h *Handler) handleSave(w http.ResponseWriter, r *http.Request) {
	if err := WriteConfig(h.configPath, h.data); err != nil {
		log.WithError(err).Error("Failed to write config")
		http.Error(w, "Failed to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.render(w, "complete.html", h.data)
}

func (h *Handler) generateKey(w http.ResponseWriter, r *http.Request) {
	key, err := GenerateAPIKey()
	if err != nil {
		http.Error(w, "Failed to generate key", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": key})
}

func (h *Handler) manifestURL(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server_url")
	if serverURL == "" {
		serverURL = h.data.ServerURL
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": SlackManifestURL(serverURL)})
}
