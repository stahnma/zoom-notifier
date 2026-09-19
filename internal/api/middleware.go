package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	log "github.com/sirupsen/logrus"
	apispec "github.com/stahnma/zoom-notifier/api"
	"github.com/stahnma/zoom-notifier/internal/store"
)

// Security scheme names as declared under components.securitySchemes in
// api/openapi.yaml. The validator hands these to authenticate so the spec
// stays the single source of truth for which operations require which key.
const (
	securitySchemeAdminKey  = "AdminKey"
	securitySchemeTenantKey = "TenantKey"
)

var (
	errUnauthorized = errors.New("unauthorized")
	errForbidden    = errors.New("forbidden")
)

// NewRequestValidator returns middleware that validates every request against
// the embedded OpenAPI spec (path params, bodies, and security requirements)
// and authenticates operations according to their declared security scheme:
//
//   - AdminKey: bearer token must equal the deployment admin key
//   - TenantKey: bearer token must equal the API key of the tenant named by
//     the {tenantId} path parameter
//
// Operations with no security requirement (healthz, webhook) are passed
// through after schema validation.
func NewRequestValidator(adminKey string, s store.Store) (MiddlewareFunc, error) {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(apispec.OpenAPISpec)
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}
	if err := spec.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("validate openapi spec: %w", err)
	}

	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: authenticate(adminKey, s),
		},
		ErrorHandlerWithOpts: handleValidationError,
		// The spec's servers entry is a local development example; the
		// deployed host must not be part of request matching.
		DoNotValidateServers: true,
		// The Zoom webhook handler answers 200 to everything so Zoom never
		// retries or disables the endpoint; keep the validator out of it.
		Skipper: func(r *http.Request) bool {
			return r.URL.Path == "/webhook/zoom"
		},
	}), nil
}

// authenticate builds the AuthenticationFunc the validator calls once per
// security scheme required by the matched operation.
func authenticate(adminKey string, s store.Store) openapi3filter.AuthenticationFunc {
	return func(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
		token := extractBearerToken(input.RequestValidationInput.Request)
		if token == "" {
			return errUnauthorized
		}

		switch input.SecuritySchemeName {
		case securitySchemeAdminKey:
			if !secureEqual(token, adminKey) {
				return errUnauthorized
			}
			return nil

		case securitySchemeTenantKey:
			tenantID := input.RequestValidationInput.PathParams["tenantId"]
			if tenantID == "" {
				return errUnauthorized
			}
			tenant, err := s.GetTenant(ctx, tenantID)
			if err != nil || tenant == nil {
				return errUnauthorized
			}
			if !secureEqual(token, tenant.APIKey) {
				return errForbidden
			}
			return nil

		default:
			return fmt.Errorf("unsupported security scheme %q", input.SecuritySchemeName)
		}
	}
}

// handleValidationError maps validator errors to the API's JSON error shape.
// Security failures become 401, or 403 when the caller presented a valid
// bearer token that does not belong to the requested tenant.
func handleValidationError(_ context.Context, err error, w http.ResponseWriter, _ *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
	var secErr *openapi3filter.SecurityRequirementsError
	if errors.As(err, &secErr) {
		for _, e := range secErr.Errors {
			if errors.Is(e, errForbidden) {
				writeJSON(w, http.StatusForbidden, ErrorResponse{Error: errForbidden.Error()})
				return
			}
		}
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: errUnauthorized.Error()})
		return
	}

	status := opts.StatusCode
	if status == 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, ErrorResponse{Error: err.Error()})
}

// secureEqual compares two secrets in constant time.
func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.WithError(err).Warn("failed to write JSON response")
	}
}
