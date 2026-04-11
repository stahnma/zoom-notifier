package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	apispec "github.com/stahnma/zoom-notifier/api"
	"github.com/stahnma/zoom-notifier/internal/api"
	"github.com/stahnma/zoom-notifier/internal/config"
	"github.com/stahnma/zoom-notifier/internal/irc"
	appmiddleware "github.com/stahnma/zoom-notifier/internal/middleware"
	"github.com/stahnma/zoom-notifier/internal/notify"
	"github.com/stahnma/zoom-notifier/internal/setup"
	appslack "github.com/stahnma/zoom-notifier/internal/slack"
	"github.com/stahnma/zoom-notifier/internal/store/sqlite"
	"github.com/stahnma/zoom-notifier/internal/zoom"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "Show version information")
	configPath := flag.String("config", "", "Path to config file")
	migrateOnly := flag.Bool("migrate", false, "Run migrations and exit")
	setupListen := flag.String("setup-listen", "localhost:8888", "Listen address for setup wizard (only used when config is missing)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Version: %s\nCommit: %s\nBuild Date: %s\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Check if setup is needed
	if setup.NeedsSetup(cfg) {
		configOutput := *configPath
		if configOutput == "" {
			configOutput = "./config.toml"
		}
		log.Infof("No configuration found. Setup wizard available at http://%s/setup", *setupListen)
		setupHandler := setup.NewHandler(configOutput)
		srv := &http.Server{
			Addr:    *setupListen,
			Handler: setupHandler.Router(),
		}
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			log.Info("shutting down setup wizard...")
			_ = srv.Shutdown(context.Background())
		}()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("setup server error: %v", err)
		}
		return
	}

	// Validate required config
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	// Setup logging
	level, err := log.ParseLevel(cfg.Log.Level)
	if err != nil {
		level = log.InfoLevel
	}
	log.SetLevel(level)
	if cfg.Log.Format == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}

	// Open SQLite store
	store, err := sqlite.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Run migrations
	if err := store.Migrate(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}
	log.Info("database migrations complete")

	if *migrateOnly {
		log.Info("migrations complete, exiting")
		os.Exit(0)
	}

	// Create notification backends
	slackSender := appslack.NewSender("")
	ircRelay := &irc.Relay{}

	// Create notification dispatcher
	dispatcher := notify.NewDispatcher(store, slackSender, ircRelay)

	// Create Zoom webhook handler
	zoomHandler := zoom.NewHandler(store, cfg.Zoom.WebhookSecret, func(tenantID, event string, payload zoom.WebhookPayload) {
		dispatcher.Dispatch(context.Background(), tenantID, payload)
	})

	// Create API server
	apiServer := api.NewServer(store, version, commit, buildDate, zoomHandler, cfg.Zoom.WebhookSecret)
	apiRouter := api.SetupRouter(apiServer, store, cfg.Admin.APIKey)

	// Build main router
	r := chi.NewRouter()
	r.Use(appmiddleware.Recovery)
	r.Use(appmiddleware.RequestLogger)

	// Landing page
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>zoom-notifier</title>
<style>
  body { font-family: system-ui, -apple-system, sans-serif; max-width: 560px; margin: 2rem auto; padding: 0 1rem; color: #333; background: #fafafa; }
  .container { background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 2rem; }
  h1 { color: #1a73e8; margin-bottom: 0.25rem; }
  .version { color: #888; font-size: 0.9rem; margin-bottom: 1.5rem; }
  h3 { margin-top: 1.5rem; margin-bottom: 0.5rem; color: #555; }
  ul { list-style: none; padding: 0; margin: 0; }
  li { margin: 0.5rem 0; padding: 0.4rem 0; }
  a { color: #1a73e8; text-decoration: none; font-weight: 500; }
  a:hover { text-decoration: underline; }
  code { background: #f0f0f0; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.9rem; }
  .endpoints { border-top: 1px solid #eee; margin-top: 1.5rem; padding-top: 1rem; }
  .endpoints li { color: #666; font-size: 0.9rem; }
</style>
</head>
<body>
<div class="container">
  <h1>zoom-notifier</h1>
  <p class="version">%s</p>

  <h3>Quick Links</h3>
  <ul>
    <li><a href="/slack/install">Install Slack App</a></li>
    <li><a href="/api/docs">API Documentation</a></li>
    <li><a href="/healthz">Health Check</a></li>
  </ul>

  <div class="endpoints">
    <h3>Endpoints</h3>
    <ul>
      <li><code>POST /webhook/zoom</code> &mdash; Zoom webhook</li>
      <li><code>POST /slack/commands</code> &mdash; Slash commands</li>
      <li><code>POST /slack/interactions</code> &mdash; Modal interactions</li>
      <li><code>/api/v1/...</code> &mdash; REST API</li>
    </ul>
  </div>
</div>
</body>
</html>`, version)
	})

	// Slack OAuth routes (outside generated API)
	if cfg.Slack.ClientID != "" {
		redirectURI := fmt.Sprintf("http://%s:%d/slack/callback", cfg.Server.Host, cfg.Server.Port)
		if cfg.Server.URL != "" {
			redirectURI = cfg.Server.URL + "/slack/callback"
		}
		oauthHandler := appslack.NewOAuthHandler(appslack.OAuthConfig{
			ClientID:     cfg.Slack.ClientID,
			ClientSecret: cfg.Slack.ClientSecret,
			RedirectURI:  redirectURI,
			Store:        store,
		})
		r.Get("/slack/install", oauthHandler.HandleInstall)
		r.Get("/slack/callback", oauthHandler.HandleCallback)
		log.Info("Slack OAuth routes enabled")
	}

	// Slack slash commands via HTTP
	if cfg.Slack.SigningSecret != "" {
		cmdHandler := appslack.NewCommandHandler(store)
		cmdHandler.SetServerURL(cfg.Server.URL)
		cmdHandler.SetModalOpener(appslack.NewSlackModalOpener("")) // enables modal support; per-tenant tokens used at runtime
		httpCmdHandler := appslack.NewHTTPCommandHandler(cmdHandler, cfg.Slack.SigningSecret)
		r.Post("/slack/commands", httpCmdHandler.ServeHTTP)
		log.Info("Slack slash command HTTP endpoint enabled")

		interactionHandler := appslack.NewInteractionHandler(store, cfg.Slack.SigningSecret)
		cmdHandler.RegisterModalHandlers(interactionHandler)
		r.Post("/slack/interactions", interactionHandler.ServeHTTP)
		log.Info("Slack interaction endpoint enabled")
	}

	// Per-tenant Zoom setup page
	tenantSetup := appslack.NewTenantSetupHandler(store)
	r.Get("/tenant/setup", tenantSetup.ServeHTTP)
	r.Post("/tenant/setup", tenantSetup.ServeHTTP)

	// API documentation
	r.Get("/api/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		_, _ = w.Write(apispec.OpenAPISpec)
	})
	r.Get("/api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>zoom-notifier API Docs</title>
<link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
<style>
  html { box-sizing: border-box; }
  *, *:before, *:after { box-sizing: inherit; }
  body { margin: 0; background: #fafafa; }
  .topbar { display: none; }
</style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({
  url: "/api/docs/openapi.yaml",
  dom_id: "#swagger-ui",
  deepLinking: true,
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
  layout: "BaseLayout"
});
</script>
</body>
</html>`)
	})

	// Probes and metrics
	r.Get("/livez", api.LivezHandler())
	r.Get("/readyz", api.ReadyzHandler(store))
	r.Handle("/metrics", promhttp.Handler())

	// Mount the generated API router (handles all /api/v1/*, /healthz, /webhook/zoom)
	r.Mount("/", apiRouter)

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Info("shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.WithError(err).Error("server shutdown error")
		}
	}()

	log.WithField("addr", addr).Info("starting HTTP server")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}

	log.Info("zoom-notifier stopped")
}
