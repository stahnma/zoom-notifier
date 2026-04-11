package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
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

//go:embed templates/*.html
var templateFS embed.FS

var (
	landingTmpl = template.Must(template.ParseFS(templateFS, "templates/landing.html"))
	swaggerHTML = mustReadFile(templateFS, "templates/swagger.html")
)

func mustReadFile(fs embed.FS, name string) []byte {
	data, err := fs.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

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
			if err := srv.Shutdown(context.Background()); err != nil {
					log.WithError(err).Warn("setup wizard shutdown error")
				}
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
	defer func() {
		if err := store.Close(); err != nil {
			log.WithError(err).Warn("failed to close database")
		}
	}()

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
		if err := landingTmpl.Execute(w, struct{ Version string }{version}); err != nil {
			log.WithError(err).Warn("failed to write landing page")
		}
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
		if _, err := w.Write(apispec.OpenAPISpec); err != nil {
			log.WithError(err).Warn("failed to write OpenAPI spec")
		}
	})
	r.Get("/api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(swaggerHTML); err != nil {
			log.WithError(err).Warn("failed to write Swagger UI page")
		}
	})

	// Probes and metrics
	r.Get("/livez", api.LivezHandler())
	r.Get("/readyz", api.ReadyzHandler(store))
	r.Handle("/metrics", promhttp.Handler())

	// Rate-limited webhook endpoint (applied before the generated router mount)
	webhookLimiter := appmiddleware.NewWebhookRateLimiter()
	r.With(func(next http.Handler) http.Handler {
		return appmiddleware.RateLimit(webhookLimiter, next)
	}).Post("/webhook/zoom", zoomHandler.ServeHTTP)

	// Mount the generated API router (handles /api/v1/*, /healthz)
	r.Mount("/", apiRouter)

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
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
