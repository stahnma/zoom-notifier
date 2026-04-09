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
	log "github.com/sirupsen/logrus"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/api"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/config"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/irc"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/notify"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/setup"
	appslack "github.com/stahnma/mandatoryFun/zoom-notifier/internal/slack"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store/sqlite"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/zoom"
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
			srv.Shutdown(context.Background())
		}()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("setup server error: %v", err)
		}
		return
	}

	// Setup logging
	level, err := log.ParseLevel(cfg.Log.Level)
	if err != nil {
		level = log.InfoLevel
	}
	log.SetLevel(level)

	// Open SQLite store
	store, err := sqlite.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

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
	apiServer := api.NewServer(store, version, zoomHandler, cfg.Zoom.WebhookSecret)
	apiRouter := api.SetupRouter(apiServer, store, cfg.Admin.APIKey)

	// Build main router
	r := chi.NewRouter()

	// Slack OAuth routes (outside generated API)
	if cfg.Slack.ClientID != "" {
		oauthHandler := appslack.NewOAuthHandler(appslack.OAuthConfig{
			ClientID:     cfg.Slack.ClientID,
			ClientSecret: cfg.Slack.ClientSecret,
			RedirectURI:  fmt.Sprintf("http://%s:%d/slack/callback", cfg.Server.Host, cfg.Server.Port),
			Store:        store,
		})
		r.Get("/slack/install", oauthHandler.HandleInstall)
		r.Get("/slack/callback", oauthHandler.HandleCallback)
		log.Info("Slack OAuth routes enabled")
	}

	// Slack slash commands via HTTP
	if cfg.Slack.SigningSecret != "" {
		cmdHandler := appslack.NewCommandHandler(store)
		httpCmdHandler := appslack.NewHTTPCommandHandler(cmdHandler, cfg.Slack.SigningSecret)
		r.Post("/slack/commands", httpCmdHandler.ServeHTTP)
		log.Info("Slack slash command HTTP endpoint enabled")
	}

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
