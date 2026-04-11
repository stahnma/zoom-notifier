package middleware

import (
	"net/http"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
)

// Recovery returns middleware that recovers from panics in HTTP handlers,
// logs the panic with a stack trace, and returns a 500 response.
// Panics with http.ErrAbortHandler are re-panicked to preserve Go's
// special handling (abort without logging).
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// http.ErrAbortHandler is a sentinel used to abort the
				// response — Go's net/http handles it specially. Re-panic
				// so the server can do its thing.
				if err == http.ErrAbortHandler {
					panic(err)
				}

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
