package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/stahnma/zoom-notifier/internal/store"
)

type contextKey string

const tenantIDKey contextKey = "tenantID"

// TenantIDFromContext extracts the authenticated tenant ID from the request context.
func TenantIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tenantIDKey).(string)
	return v
}

// AuthMiddleware checks the Authorization header based on the security scope
// set by the generated handler wrapper (AdminKeyScopes or TenantKeyScopes).
func AuthMiddleware(adminKey string, s store.Store) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Admin-scoped endpoints
			if r.Context().Value(AdminKeyScopes) != nil {
				token := extractBearerToken(r)
				if token == "" || token != adminKey {
					writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Tenant-scoped endpoints
			if r.Context().Value(TenantKeyScopes) != nil {
				token := extractBearerToken(r)
				if token == "" {
					writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
					return
				}

				tenantID := extractTenantIDFromPath(r.URL.Path)
				if tenantID == "" {
					writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
					return
				}

				tenant, err := s.GetTenant(r.Context(), tenantID)
				if err != nil || tenant == nil {
					writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
					return
				}

				if tenant.APIKey != token {
					writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "forbidden"})
					return
				}

				ctx := context.WithValue(r.Context(), tenantIDKey, tenantID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// No security scope (healthz, webhook)
			next.ServeHTTP(w, r)
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

func extractTenantIDFromPath(path string) string {
	// Path pattern: /api/v1/tenants/{tenantId}/...
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// Expected: api, v1, tenants, {tenantId}, ...
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "tenants" {
		return parts[3]
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
