package middleware

import (
	"net/http"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
)

// Recovery returns middleware that recovers from panics in HTTP handlers,
// logs the panic with a stack trace, and returns a 500 response.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.WithFields(log.Fields{
					"panic":  err,
					"method": r.Method,
					"path":   r.URL.Path,
					"stack":  string(debug.Stack()),
				}).Error("panic recovered in HTTP handler")

				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
