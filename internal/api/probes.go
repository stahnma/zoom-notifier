package api

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/store"
)

// LivezHandler returns 200 if the process is alive. No dependency checks.
func LivezHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			log.WithError(err).Error("failed to encode livez response")
		}
	}
}

// ReadyzHandler returns 200 if the service is ready to accept traffic.
// Checks database connectivity.
func ReadyzHandler(s store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check database by listing tenants (lightweight query)
		if _, err := s.ListTenants(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			if encErr := json.NewEncoder(w).Encode(map[string]string{
				"status":   "not ready",
				"database": err.Error(),
			}); encErr != nil {
				log.WithError(encErr).Error("failed to encode readyz response")
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"status":   "ok",
			"database": "ok",
		}); err != nil {
			log.WithError(err).Error("failed to encode readyz response")
		}
	}
}
