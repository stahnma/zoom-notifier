package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/zoom"
)

// Server implements the generated StrictServerInterface.
type Server struct {
	store         store.Store
	version       string
	zoomHandler   *zoom.Handler
	webhookSecret string
}

var _ StrictServerInterface = (*Server)(nil)

func NewServer(s store.Store, version string, zoomHandler *zoom.Handler, webhookSecret string) *Server {
	return &Server{
		store:         s,
		version:       version,
		zoomHandler:   zoomHandler,
		webhookSecret: webhookSecret,
	}
}

// SetupRouter creates an http.Handler with the generated routes and auth middleware.
func SetupRouter(server StrictServerInterface, s store.Store, adminKey string) http.Handler {
	strictHandler := NewStrictHandler(server, nil)
	return HandlerWithOptions(strictHandler, ChiServerOptions{
		Middlewares: []MiddlewareFunc{
			AuthMiddleware(adminKey, s),
		},
	})
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// --- Health ---

func (s *Server) GetHealthz(ctx context.Context, request GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{
		Status:  "ok",
		Version: s.version,
	}, nil
}

// --- Tenants ---

func (s *Server) ListTenants(ctx context.Context, request ListTenantsRequestObject) (ListTenantsResponseObject, error) {
	tenants, err := s.store.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	result := make(ListTenants200JSONResponse, 0, len(tenants))
	for _, t := range tenants {
		result = append(result, toAPITenant(t))
	}
	return result, nil
}

func (s *Server) CreateTenant(ctx context.Context, request CreateTenantRequestObject) (CreateTenantResponseObject, error) {
	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	if request.Body.Id != nil && *request.Body.Id != "" {
		id = *request.Body.Id
	}

	tenant := &store.Tenant{
		ID:          id,
		APIKey:      apiKey,
		InstalledAt: time.Now(),
	}
	if request.Body.TeamName != nil {
		tenant.TeamName = *request.Body.TeamName
	}
	if request.Body.ZoomAccountId != nil {
		tenant.ZoomAccountID = *request.Body.ZoomAccountId
	}

	if err := s.store.CreateTenant(ctx, tenant); err != nil {
		return nil, err
	}

	return CreateTenant201JSONResponse{
		Id:     id,
		ApiKey: apiKey,
	}, nil
}

func (s *Server) GetTenant(ctx context.Context, request GetTenantRequestObject) (GetTenantResponseObject, error) {
	tenant, err := s.store.GetTenant(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return GetTenant404JSONResponse{NotFoundJSONResponse{Error: "tenant not found"}}, nil
	}
	return GetTenant200JSONResponse(toAPITenant(tenant)), nil
}

func (s *Server) DeleteTenant(ctx context.Context, request DeleteTenantRequestObject) (DeleteTenantResponseObject, error) {
	tenant, err := s.store.GetTenant(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return DeleteTenant404JSONResponse{NotFoundJSONResponse{Error: "tenant not found"}}, nil
	}
	if err := s.store.DeleteTenant(ctx, request.TenantId); err != nil {
		return nil, err
	}
	return DeleteTenant204Response{}, nil
}

// --- Admins ---

func (s *Server) ListAdmins(ctx context.Context, request ListAdminsRequestObject) (ListAdminsResponseObject, error) {
	admins, err := s.store.ListAdmins(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	result := make(ListAdmins200JSONResponse, 0, len(admins))
	for _, a := range admins {
		result = append(result, TenantAdmin{
			TenantId:    a.TenantID,
			SlackUserId: a.SlackUserID,
		})
	}
	return result, nil
}

func (s *Server) AddAdmin(ctx context.Context, request AddAdminRequestObject) (AddAdminResponseObject, error) {
	if err := s.store.AddAdmin(ctx, request.TenantId, request.Body.SlackUserId); err != nil {
		return nil, err
	}
	return AddAdmin201Response{}, nil
}

func (s *Server) RemoveAdmin(ctx context.Context, request RemoveAdminRequestObject) (RemoveAdminResponseObject, error) {
	if err := s.store.RemoveAdmin(ctx, request.TenantId, request.UserId); err != nil {
		return nil, err
	}
	return RemoveAdmin204Response{}, nil
}

// --- Subscriptions ---

func (s *Server) ListSubscriptions(ctx context.Context, request ListSubscriptionsRequestObject) (ListSubscriptionsResponseObject, error) {
	subs, err := s.store.ListSubscriptions(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	result := make(ListSubscriptions200JSONResponse, 0, len(subs))
	for _, sub := range subs {
		result = append(result, toAPISubscription(sub))
	}
	return result, nil
}

func (s *Server) CreateSubscription(ctx context.Context, request CreateSubscriptionRequestObject) (CreateSubscriptionResponseObject, error) {
	sub := &store.Subscription{
		TenantID: request.TenantId,
		Type:     string(request.Body.Type),
		Target:   request.Body.Target,
		Enabled:  true,
	}
	if request.Body.MeetingId != nil {
		sub.MeetingID = request.Body.MeetingId
	}
	if request.Body.MsgSuffix != nil {
		sub.MsgSuffix = *request.Body.MsgSuffix
	}
	if request.Body.IncludeLink != nil {
		sub.IncludeLink = *request.Body.IncludeLink
	}

	if err := s.store.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	return CreateSubscription201JSONResponse(toAPISubscription(sub)), nil
}

func (s *Server) UpdateSubscription(ctx context.Context, request UpdateSubscriptionRequestObject) (UpdateSubscriptionResponseObject, error) {
	sub, err := s.store.GetSubscription(ctx, request.SubscriptionId)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return UpdateSubscription404JSONResponse{NotFoundJSONResponse{Error: "subscription not found"}}, nil
	}

	if request.Body.Enabled != nil {
		sub.Enabled = *request.Body.Enabled
	}
	if request.Body.Target != nil {
		sub.Target = *request.Body.Target
	}
	if request.Body.MsgSuffix != nil {
		sub.MsgSuffix = *request.Body.MsgSuffix
	}
	if request.Body.IncludeLink != nil {
		sub.IncludeLink = *request.Body.IncludeLink
	}

	if err := s.store.UpdateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	return UpdateSubscription200JSONResponse(toAPISubscription(sub)), nil
}

func (s *Server) DeleteSubscription(ctx context.Context, request DeleteSubscriptionRequestObject) (DeleteSubscriptionResponseObject, error) {
	sub, err := s.store.GetSubscription(ctx, request.SubscriptionId)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return DeleteSubscription404JSONResponse{NotFoundJSONResponse{Error: "subscription not found"}}, nil
	}
	if err := s.store.DeleteSubscription(ctx, request.SubscriptionId); err != nil {
		return nil, err
	}
	return DeleteSubscription204Response{}, nil
}

// --- Filters ---

func (s *Server) ListFilters(ctx context.Context, request ListFiltersRequestObject) (ListFiltersResponseObject, error) {
	filters, err := s.store.ListFilters(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	result := make(ListFilters200JSONResponse, 0, len(filters))
	for _, f := range filters {
		result = append(result, MeetingFilter{
			Id:       f.ID,
			TenantId: f.TenantID,
			Pattern:  f.Pattern,
		})
	}
	return result, nil
}

func (s *Server) CreateFilter(ctx context.Context, request CreateFilterRequestObject) (CreateFilterResponseObject, error) {
	f := &store.MeetingFilter{
		TenantID: request.TenantId,
		Pattern:  request.Body.Pattern,
	}
	if err := s.store.CreateFilter(ctx, f); err != nil {
		return nil, err
	}
	return CreateFilter201JSONResponse{
		Id:       f.ID,
		TenantId: f.TenantID,
		Pattern:  f.Pattern,
	}, nil
}

func (s *Server) DeleteFilter(ctx context.Context, request DeleteFilterRequestObject) (DeleteFilterResponseObject, error) {
	if err := s.store.DeleteFilter(ctx, request.FilterId); err != nil {
		return nil, err
	}
	return DeleteFilter204Response{}, nil
}

// --- IRC Config ---

func (s *Server) GetIRCConfig(ctx context.Context, request GetIRCConfigRequestObject) (GetIRCConfigResponseObject, error) {
	configs, err := s.store.GetIRCConfig(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	result := make(GetIRCConfig200JSONResponse, 0, len(configs))
	for _, c := range configs {
		tenantID := c.TenantID
		result = append(result, IRCConfig{
			TenantId: &tenantID,
			Server:   c.Server,
			Nick:     c.Nick,
			UseTls:   c.UseTLS,
		})
	}
	return result, nil
}

func (s *Server) PutIRCConfig(ctx context.Context, request PutIRCConfigRequestObject) (PutIRCConfigResponseObject, error) {
	useTLS := false
	if request.Body.UseTls != nil {
		useTLS = *request.Body.UseTls
	}

	c := &store.IRCConfig{
		TenantID: request.TenantId,
		Server:   request.Body.Server,
		Nick:     request.Body.Nick,
		Password: request.Body.Password,
		UseTLS:   useTLS,
	}
	if err := s.store.UpsertIRCConfig(ctx, c); err != nil {
		return nil, err
	}

	tenantID := request.TenantId
	return PutIRCConfig200JSONResponse{
		TenantId: &tenantID,
		Server:   c.Server,
		Nick:     c.Nick,
		UseTls:   c.UseTLS,
	}, nil
}

// --- Meetings ---

func (s *Server) ListMeetings(ctx context.Context, request ListMeetingsRequestObject) (ListMeetingsResponseObject, error) {
	meetings, err := s.store.ListActiveMeetings(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	result := make(ListMeetings200JSONResponse, 0, len(meetings))
	for _, m := range meetings {
		result = append(result, toAPIMeeting(m))
	}
	return result, nil
}

func (s *Server) ListParticipants(ctx context.Context, request ListParticipantsRequestObject) (ListParticipantsResponseObject, error) {
	meeting, err := s.store.GetMeeting(ctx, request.MeetingId)
	if err != nil {
		return nil, err
	}
	if meeting == nil {
		return ListParticipants404JSONResponse{NotFoundJSONResponse{Error: "meeting not found"}}, nil
	}

	participants, err := s.store.GetActiveParticipants(ctx, request.MeetingId)
	if err != nil {
		return nil, err
	}
	result := make(ListParticipants200JSONResponse, 0, len(participants))
	for _, p := range participants {
		result = append(result, toAPIParticipant(p))
	}
	return result, nil
}

// --- API Key Rotation ---

func (s *Server) RotateAPIKey(ctx context.Context, request RotateAPIKeyRequestObject) (RotateAPIKeyResponseObject, error) {
	newKey, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateTenantAPIKey(ctx, request.TenantId, newKey); err != nil {
		return nil, err
	}
	return RotateAPIKey200JSONResponse{ApiKey: newKey}, nil
}

// --- Zoom Credentials ---

func (s *Server) PutZoomCredentials(ctx context.Context, request PutZoomCredentialsRequestObject) (PutZoomCredentialsResponseObject, error) {
	z := &store.ZoomCredentials{
		TenantID:     request.TenantId,
		ClientID:     request.Body.ClientId,
		ClientSecret: request.Body.ClientSecret,
		AccountID:    request.Body.AccountId,
	}
	if err := s.store.UpsertZoomCredentials(ctx, z); err != nil {
		return nil, err
	}
	return PutZoomCredentials200Response{}, nil
}

// --- Zoom Webhook ---

// webhookCRCResponse returns CRC validation JSON for Zoom endpoint verification.
type webhookCRCResponse struct {
	body zoom.CRCResponse
}

func (r webhookCRCResponse) VisitPostWebhookZoomResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	return json.NewEncoder(w).Encode(r.body)
}

func (s *Server) PostWebhookZoom(ctx context.Context, request PostWebhookZoomRequestObject) (PostWebhookZoomResponseObject, error) {
	if request.Body == nil {
		return PostWebhookZoom200Response{}, nil
	}

	// Re-encode map body to JSON, then decode into typed payload
	data, err := json.Marshal(request.Body)
	if err != nil {
		return PostWebhookZoom200Response{}, nil
	}
	var payload zoom.WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return PostWebhookZoom200Response{}, nil
	}

	// Handle CRC validation
	if payload.Event == "endpoint.url_validation" {
		resp, err := zoom.ValidateCRC(payload, s.webhookSecret)
		if err != nil {
			return PostWebhookZoom200Response{}, nil
		}
		return webhookCRCResponse{body: *resp}, nil
	}

	// Process the webhook event
	if s.zoomHandler != nil {
		s.zoomHandler.ProcessWebhook(ctx, payload)
	}
	return PostWebhookZoom200Response{}, nil
}

// --- Type conversion helpers ---

func toAPITenant(t *store.Tenant) Tenant {
	tenant := Tenant{
		Id:          t.ID,
		ApiKey:      t.APIKey,
		InstalledAt: &t.InstalledAt,
	}
	if t.TeamName != "" {
		tenant.TeamName = &t.TeamName
	}
	if t.ZoomAccountID != "" {
		tenant.ZoomAccountId = &t.ZoomAccountID
	}
	return tenant
}

func toAPISubscription(sub *store.Subscription) Subscription {
	s := Subscription{
		Id:        sub.ID,
		TenantId:  sub.TenantID,
		Type:      SubscriptionType(sub.Type),
		Target:    sub.Target,
		Enabled:   sub.Enabled,
		MeetingId: sub.MeetingID,
		CreatedAt: &sub.CreatedAt,
	}
	if sub.MsgSuffix != "" {
		s.MsgSuffix = &sub.MsgSuffix
	}
	if sub.IncludeLink {
		s.IncludeLink = &sub.IncludeLink
	}
	return s
}

func toAPIMeeting(m *store.ActiveMeeting) ActiveMeeting {
	am := ActiveMeeting{
		MeetingId: m.MeetingID,
		TenantId:  m.TenantID,
		StartTime: &m.StartTime,
	}
	if m.Topic != "" {
		am.Topic = &m.Topic
	}
	if m.HostID != "" {
		am.HostId = &m.HostID
	}
	if m.JoinURL != "" {
		am.JoinUrl = &m.JoinURL
	}
	return am
}

func toAPIParticipant(p *store.Participant) Participant {
	part := Participant{
		Id:        p.ID,
		MeetingId: p.MeetingID,
		UserName:  p.UserName,
		JoinTime:  &p.JoinTime,
		LeaveTime: p.LeaveTime,
	}
	if p.Email != "" {
		part.Email = &p.Email
	}
	return part
}
