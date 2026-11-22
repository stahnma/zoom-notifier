package store

import (
	"context"
	"errors"
	"fmt"
)

// ErrZoomAccountManaged is returned when a tenant tries to self-service a Zoom
// account that the deployment operator manages (see SelfServiceZoomAccount).
var ErrZoomAccountManaged = errors.New("zoom account is managed by the operator")

// SelfServiceZoomAccount links tenant to the Zoom account accountID and, when
// clientID is non-empty, sets the account's API credentials. A blank
// clientSecret keeps the existing secret.
//
// This is the tenant-key path (PUT /tenants/{id}/zoom and /tenant/setup). It
// refuses accounts the operator manages: those linked to any other tenant, and
// those with a per-account webhook secret. Otherwise a tenant could attach
// itself to another workspace's Zoom account and receive its meeting events,
// or overwrite credentials that other tenants rely on. Operators link tenants
// to shared accounts with the admin API instead.
func SelfServiceZoomAccount(ctx context.Context, s Store, tenant *Tenant, accountID, clientID, clientSecret string) error {
	if accountID == "" {
		return errors.New("zoom account ID is required")
	}

	linked, err := s.GetTenantByZoomAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("list tenants for zoom account: %w", err)
	}
	for _, t := range linked {
		if t.ID != tenant.ID {
			return ErrZoomAccountManaged
		}
	}

	account, err := s.GetZoomAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get zoom account: %w", err)
	}
	if account != nil && account.WebhookSecret != "" {
		return ErrZoomAccountManaged
	}

	if tenant.ZoomAccountID != accountID {
		tenant.ZoomAccountID = accountID
		if err := s.UpdateTenant(ctx, tenant); err != nil {
			return fmt.Errorf("link tenant to zoom account: %w", err)
		}
	}

	if clientID == "" {
		return s.EnsureZoomAccount(ctx, accountID)
	}
	if account == nil {
		account = &ZoomAccount{AccountID: accountID}
	}
	account.ClientID = clientID
	if clientSecret != "" {
		account.ClientSecret = clientSecret
	}
	if err := s.UpsertZoomAccount(ctx, account); err != nil {
		return fmt.Errorf("save zoom API credentials: %w", err)
	}
	return nil
}
