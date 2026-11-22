package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stahnma/zoom-notifier/internal/store"
)

// call sends a JSON request with a bearer key and returns the recorder.
func call(t *testing.T, router http.Handler, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	} else {
		r = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeAccount(t *testing.T, w *httptest.ResponseRecorder) ZoomAccount {
	t.Helper()
	var a ZoomAccount
	if err := json.NewDecoder(w.Body).Decode(&a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestZoomAccountAdminCRUD(t *testing.T) {
	router, _ := setupTestRouter(t)

	w := call(t, router, http.MethodPut, "/api/v1/zoom-accounts/ACCT1", testAdminKey, ZoomAccountRequest{
		WebhookSecret: strPtr("wh-secret"), ClientId: strPtr("cid"), ClientSecret: strPtr("csecret"),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put account: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "wh-secret") || strings.Contains(w.Body.String(), "csecret") {
		t.Fatalf("secrets must not be returned: %s", w.Body.String())
	}
	a := decodeAccount(t, w)
	if !a.HasWebhookSecret || !a.HasClientSecret || a.ClientId == nil || *a.ClientId != "cid" || len(a.TenantIds) != 0 {
		t.Errorf("unexpected account: %+v", a)
	}

	// Omitted fields are kept; an empty string clears.
	w = call(t, router, http.MethodPut, "/api/v1/zoom-accounts/ACCT1", testAdminKey, ZoomAccountRequest{ClientSecret: strPtr("")})
	if w.Code != http.StatusOK {
		t.Fatalf("partial put: %d %s", w.Code, w.Body.String())
	}
	a = decodeAccount(t, w)
	if !a.HasWebhookSecret || a.HasClientSecret {
		t.Errorf("expected webhook secret kept and client secret cleared, got %+v", a)
	}

	// Link two tenants; both appear on the account.
	createTenantForAuth(t, router, "slack-a")
	createTenantForAuth(t, router, "slack-b")
	for _, tenant := range []string{"slack-a", "slack-b"} {
		w = call(t, router, http.MethodPut, "/api/v1/tenants/"+tenant+"/zoom-account", testAdminKey, TenantZoomAccountRequest{ZoomAccountId: "ACCT1"})
		if w.Code != http.StatusOK {
			t.Fatalf("link %s: %d %s", tenant, w.Code, w.Body.String())
		}
	}
	w = call(t, router, http.MethodGet, "/api/v1/zoom-accounts/ACCT1", testAdminKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get account: %d %s", w.Code, w.Body.String())
	}
	if a = decodeAccount(t, w); strings.Join(a.TenantIds, ",") != "slack-a,slack-b" {
		t.Errorf("expected linked tenants [slack-a slack-b], got %v", a.TenantIds)
	}

	w = call(t, router, http.MethodGet, "/api/v1/zoom-accounts", testAdminKey, nil)
	var list []ZoomAccount
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AccountId != "ACCT1" {
		t.Errorf("unexpected list: %+v", list)
	}

	// Delete refused while linked, allowed after unlinking.
	if w = call(t, router, http.MethodDelete, "/api/v1/zoom-accounts/ACCT1", testAdminKey, nil); w.Code != http.StatusConflict {
		t.Fatalf("delete linked account: expected 409, got %d", w.Code)
	}
	for _, tenant := range []string{"slack-a", "slack-b"} {
		if w = call(t, router, http.MethodPut, "/api/v1/tenants/"+tenant+"/zoom-account", testAdminKey, TenantZoomAccountRequest{ZoomAccountId: ""}); w.Code != http.StatusOK {
			t.Fatalf("unlink %s: %d", tenant, w.Code)
		}
	}
	if w = call(t, router, http.MethodDelete, "/api/v1/zoom-accounts/ACCT1", testAdminKey, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete account: expected 204, got %d %s", w.Code, w.Body.String())
	}
	if w = call(t, router, http.MethodGet, "/api/v1/zoom-accounts/ACCT1", testAdminKey, nil); w.Code != http.StatusNotFound {
		t.Fatalf("get deleted account: expected 404, got %d", w.Code)
	}
}

func TestZoomAccountEndpointsRequireAdminKey(t *testing.T) {
	router, _ := setupTestRouter(t)
	tenantKey := createTenantForAuth(t, router, "t1")

	checks := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/zoom-accounts"},
		{http.MethodGet, "/api/v1/zoom-accounts/A"},
		{http.MethodPut, "/api/v1/zoom-accounts/A"},
		{http.MethodDelete, "/api/v1/zoom-accounts/A"},
		{http.MethodPut, "/api/v1/tenants/t1/zoom-account"},
	}
	for _, c := range checks {
		var body any
		if c.method == http.MethodPut {
			body = map[string]string{"zoom_account_id": "A"}
		}
		if w := call(t, router, c.method, c.path, tenantKey, body); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with tenant key: expected 401, got %d", c.method, c.path, w.Code)
		}
	}
}

func TestPutZoomCredentialsRefusesSharedAccount(t *testing.T) {
	router, _ := setupTestRouter(t)
	createTenantForAuth(t, router, "owner")
	intruderKey := createTenantForAuth(t, router, "intruder")
	if w := call(t, router, http.MethodPut, "/api/v1/tenants/owner/zoom-account", testAdminKey, TenantZoomAccountRequest{ZoomAccountId: "OWNED"}); w.Code != http.StatusOK {
		t.Fatalf("link owner: %d", w.Code)
	}

	w := call(t, router, http.MethodPut, "/api/v1/tenants/intruder/zoom", intruderKey, ZoomCredentialsRequest{
		AccountId: "OWNED", ClientId: "evil", ClientSecret: "evil",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", w.Code, w.Body.String())
	}

	// A fresh, unshared account is fine, and links the tenant.
	w = call(t, router, http.MethodPut, "/api/v1/tenants/intruder/zoom", intruderKey, ZoomCredentialsRequest{
		AccountId: "MINE", ClientId: "cid", ClientSecret: "secret",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	w = call(t, router, http.MethodGet, "/api/v1/tenants/intruder", intruderKey, nil)
	var tenant Tenant
	if err := json.NewDecoder(w.Body).Decode(&tenant); err != nil {
		t.Fatal(err)
	}
	if tenant.ZoomAccountId == nil || *tenant.ZoomAccountId != "MINE" {
		t.Errorf("expected tenant linked to MINE, got %v", tenant.ZoomAccountId)
	}
}

// A tenant can only read participants of meetings on its own Zoom account.
func TestListParticipantsScopedToTenantAccount(t *testing.T) {
	router, s := setupTestRouter(t)
	ctx := context.Background()
	keyA := createTenantForAuth(t, router, "tenant-a")
	createTenantForAuth(t, router, "tenant-b")
	for tenant, acct := range map[string]string{"tenant-a": "ACCT-A", "tenant-b": "ACCT-B"} {
		if w := call(t, router, http.MethodPut, "/api/v1/tenants/"+tenant+"/zoom-account", testAdminKey, TenantZoomAccountRequest{ZoomAccountId: acct}); w.Code != http.StatusOK {
			t.Fatalf("link %s: %d", tenant, w.Code)
		}
	}
	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{ZoomAccountID: "ACCT-B", MeetingID: "secret-meeting", Topic: "B only", StartTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddParticipant(ctx, &store.Participant{ZoomAccountID: "ACCT-B", MeetingID: "secret-meeting", UserName: "Bob", JoinTime: time.Now()}); err != nil {
		t.Fatal(err)
	}

	w := call(t, router, http.MethodGet, "/api/v1/tenants/tenant-a/meetings/secret-meeting/participants", keyA, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another account's meeting, got %d %s", w.Code, w.Body.String())
	}

	// Same meeting ID on tenant A's own account is visible.
	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{ZoomAccountID: "ACCT-A", MeetingID: "secret-meeting", Topic: "A's", StartTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddParticipant(ctx, &store.Participant{ZoomAccountID: "ACCT-A", MeetingID: "secret-meeting", UserName: "Alice", JoinTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	w = call(t, router, http.MethodGet, "/api/v1/tenants/tenant-a/meetings/secret-meeting/participants", keyA, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	var participants []Participant
	if err := json.NewDecoder(w.Body).Decode(&participants); err != nil {
		t.Fatal(err)
	}
	if len(participants) != 1 || participants[0].UserName != "Alice" {
		t.Errorf("expected only Alice, got %+v", participants)
	}

	w = call(t, router, http.MethodGet, "/api/v1/tenants/tenant-a/meetings", keyA, nil)
	var meetings []ActiveMeeting
	if err := json.NewDecoder(w.Body).Decode(&meetings); err != nil {
		t.Fatal(err)
	}
	if len(meetings) != 1 || meetings[0].TenantId != "tenant-a" || meetings[0].ZoomAccountId != "ACCT-A" {
		t.Errorf("unexpected meetings: %+v", meetings)
	}
}
