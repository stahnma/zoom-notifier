package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/setup"
	"github.com/stahnma/zoom-notifier/internal/store"
	"github.com/stahnma/zoom-notifier/internal/zoom"
)

// Server implements the generated StrictServerInterface.
type Server struct {
	store         store.Store
	version       string
	commit        string
	buildDate     string
	startTime     time.Time
	zoomHandler   *zoom.Handler
	webhookSecret string
}

var _ StrictServerInterface = (*Server)(nil)

func NewServer(s store.Store, version, commit, buildDate string, zoomHandler *zoom.Handler, webhookSecret string) *Server {
	return &Server{
		store:         s,
		version:       version,
		commit:        commit,
		buildDate:     buildDate,
		startTime:     time.Now(),
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
	return setup.GenerateAPIKey()
}

// --- Health ---

func (s *Server) GetHealthz(ctx context.Context, request GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	uptime := time.Since(s.startTime).Round(time.Second).String()
	commit := &s.commit
	buildDate := &s.buildDate

	// Check database
	dbStatus := "ok"
	tenants, err := s.store.ListTenants(ctx)
	tenantCount := 0
	if err != nil {
		dbStatus = "error: " + err.Error()
	} else {
		tenantCount = len(tenants)
	}

	return GetHealthz200JSONResponse{
		Status:    "ok",
		Version:   s.version,
		Commit:    commit,
		BuildDate: buildDate,
		Uptime:    &uptime,
		Database:  &dbStatus,
		Tenants:   &tenantCount,
	}, nil
}

// --- Tenants ---

func (s *Server) ListTenants(ctx context.Context, request ListTenantsRequestObject) (ListTenantsResponseObject, error) {
	log.Debug("listing tenants")
	tenants, err := s.store.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	log.WithField("count", len(tenants)).Debug("listed tenants")
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

	log.WithField("tenant_id", id).Info("tenant created via API")
	return CreateTenant201JSONResponse{
		Id:     id,
		ApiKey: apiKey,
	}, nil
}

func (s *Server) GetTenant(ctx context.Context, request GetTenantRequestObject) (GetTenantResponseObject, error) {
	log.WithField("tenant_id", request.TenantId).Debug("getting tenant")
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
	log.WithField("tenant_id", request.TenantId).Debug("deleting tenant")
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

// --- Tenant Defaults ---

func (s *Server) PutTenantDefaults(ctx context.Context, request PutTenantDefaultsRequestObject) (PutTenantDefaultsResponseObject, error) {
	log.WithField("tenant_id", request.TenantId).Info("updating tenant defaults via API")
	tenant, err := s.store.GetTenant(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return PutTenantDefaults404JSONResponse{NotFoundJSONResponse{Error: "tenant not found"}}, nil
	}

	suffix := tenant.DefaultMsgSuffix
	includeLink := tenant.DefaultIncludeLink
	if request.Body.DefaultMsgSuffix != nil {
		suffix = *request.Body.DefaultMsgSuffix
	}
	if request.Body.DefaultIncludeLink != nil {
		includeLink = *request.Body.DefaultIncludeLink
	}

	if err := s.store.UpdateTenantDefaults(ctx, request.TenantId, suffix, includeLink); err != nil {
		return nil, err
	}

	// Re-fetch to return updated tenant
	tenant, err = s.store.GetTenant(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	return PutTenantDefaults200JSONResponse(toAPITenant(tenant)), nil
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
	log.WithField("tenant_id", request.TenantId).Debug("listing subscriptions")
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
	if err := s.store.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"tenant_id": request.TenantId,
		"type":      sub.Type,
		"target":    sub.Target,
	}).Info("subscription created via API")
	return CreateSubscription201JSONResponse(toAPISubscription(sub)), nil
}

func (s *Server) UpdateSubscription(ctx context.Context, request UpdateSubscriptionRequestObject) (UpdateSubscriptionResponseObject, error) {
	log.WithField("subscription_id", request.SubscriptionId).Info("updating subscription via API")
	sub, err := s.store.GetSubscription(ctx, request.SubscriptionId)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.TenantID != request.TenantId {
		return UpdateSubscription404JSONResponse{NotFoundJSONResponse{Error: "subscription not found"}}, nil
	}

	if request.Body.Enabled != nil {
		sub.Enabled = *request.Body.Enabled
	}
	if request.Body.Target != nil {
		sub.Target = *request.Body.Target
	}
	if err := s.store.UpdateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	return UpdateSubscription200JSONResponse(toAPISubscription(sub)), nil
}

func (s *Server) DeleteSubscription(ctx context.Context, request DeleteSubscriptionRequestObject) (DeleteSubscriptionResponseObject, error) {
	log.WithField("subscription_id", request.SubscriptionId).Info("deleting subscription via API")
	sub, err := s.store.GetSubscription(ctx, request.SubscriptionId)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.TenantID != request.TenantId {
		return DeleteSubscription404JSONResponse{NotFoundJSONResponse{Error: "subscription not found"}}, nil
	}
	if err := s.store.DeleteSubscription(ctx, request.SubscriptionId); err != nil {
		return nil, err
	}
	return DeleteSubscription204Response{}, nil
}

// --- Filters ---

func (s *Server) ListFilters(ctx context.Context, request ListFiltersRequestObject) (ListFiltersResponseObject, error) {
	log.WithField("tenant_id", request.TenantId).Debug("listing filters")
	filters, err := s.store.ListFilters(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	result := make(ListFilters200JSONResponse, 0, len(filters))
	for _, f := range filters {
		result = append(result, toAPIFilter(f))
	}
	return result, nil
}

func (s *Server) CreateFilter(ctx context.Context, request CreateFilterRequestObject) (CreateFilterResponseObject, error) {
	f := &store.MeetingFilter{
		TenantID:    request.TenantId,
		Pattern:     request.Body.Pattern,
		MsgSuffix:   request.Body.MsgSuffix,
		IncludeLink: request.Body.IncludeLink,
	}
	if err := s.store.CreateFilter(ctx, f); err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"tenant_id": request.TenantId,
		"pattern":   f.Pattern,
	}).Info("filter created via API")
	return CreateFilter201JSONResponse(toAPIFilter(f)), nil
}

func (s *Server) UpdateFilter(ctx context.Context, request UpdateFilterRequestObject) (UpdateFilterResponseObject, error) {
	log.WithFields(log.Fields{
		"tenant_id": request.TenantId,
		"filter_id": request.FilterId,
	}).Info("updating filter via API")
	// Get existing filters to find this one
	filters, err := s.store.ListFilters(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	var existing *store.MeetingFilter
	for _, f := range filters {
		if f.ID == request.FilterId {
			existing = f
			break
		}
	}
	if existing == nil {
		return UpdateFilter404JSONResponse{NotFoundJSONResponse{Error: "filter not found"}}, nil
	}

	if request.Body.Pattern != nil {
		existing.Pattern = *request.Body.Pattern
	}
	// These are nullable: a non-nil pointer means the caller sent the field
	existing.MsgSuffix = request.Body.MsgSuffix
	existing.IncludeLink = request.Body.IncludeLink

	if err := s.store.UpdateFilter(ctx, existing); err != nil {
		return nil, err
	}
	return UpdateFilter200JSONResponse(toAPIFilter(existing)), nil
}

func (s *Server) DeleteFilter(ctx context.Context, request DeleteFilterRequestObject) (DeleteFilterResponseObject, error) {
	log.WithField("filter_id", request.FilterId).Info("deleting filter via API")
	filters, err := s.store.ListFilters(ctx, request.TenantId)
	if err != nil {
		return nil, err
	}
	var found bool
	for _, f := range filters {
		if f.ID == request.FilterId {
			found = true
			break
		}
	}
	if !found {
		return DeleteFilter404JSONResponse{NotFoundJSONResponse{Error: "filter not found"}}, nil
	}
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
		insecureTLS := c.InsecureTLS
		result = append(result, IRCConfig{
			TenantId:    &tenantID,
			Server:      c.Server,
			Nick:        c.Nick,
			UseTls:      c.UseTLS,
			InsecureTls: &insecureTLS,
		})
	}
	return result, nil
}

func (s *Server) PutIRCConfig(ctx context.Context, request PutIRCConfigRequestObject) (PutIRCConfigResponseObject, error) {
	useTLS := false
	if request.Body.UseTls != nil {
		useTLS = *request.Body.UseTls
	}
	insecureTLS := false
	if request.Body.InsecureTls != nil {
		insecureTLS = *request.Body.InsecureTls
	}

	c := &store.IRCConfig{
		TenantID:    request.TenantId,
		Server:      request.Body.Server,
		Nick:        request.Body.Nick,
		Password:    request.Body.Password,
		UseTLS:      useTLS,
		InsecureTLS: insecureTLS,
	}
	if err := s.store.UpsertIRCConfig(ctx, c); err != nil {
		return nil, err
	}

	tenantID := request.TenantId
	return PutIRCConfig200JSONResponse{
		TenantId:    &tenantID,
		Server:      c.Server,
		Nick:        c.Nick,
		UseTls:      c.UseTLS,
		InsecureTls: &insecureTLS,
	}, nil
}

// --- Meetings ---

func (s *Server) ListMeetings(ctx context.Context, request ListMeetingsRequestObject) (ListMeetingsResponseObject, error) {
	log.WithField("tenant_id", request.TenantId).Debug("listing meetings")
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
	log.WithField("meeting_id", request.MeetingId).Debug("listing participants")
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
	log.WithField("tenant_id", request.TenantId).Info("updating zoom credentials via API")
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
	log.Debug("received zoom webhook via API endpoint")
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
	if payload.Event == zoom.EventURLValidation {
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
		Id:                 t.ID,
		ApiKey:             t.APIKey,
		InstalledAt:        &t.InstalledAt,
		DefaultMsgSuffix:   &t.DefaultMsgSuffix,
		DefaultIncludeLink: &t.DefaultIncludeLink,
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
	return Subscription{
		Id:        sub.ID,
		TenantId:  sub.TenantID,
		Type:      SubscriptionType(sub.Type),
		Target:    sub.Target,
		Enabled:   sub.Enabled,
		MeetingId: sub.MeetingID,
		CreatedAt: &sub.CreatedAt,
	}
}

func toAPIFilter(f *store.MeetingFilter) MeetingFilter {
	return MeetingFilter{
		Id:          f.ID,
		TenantId:    f.TenantID,
		Pattern:     f.Pattern,
		MsgSuffix:   f.MsgSuffix,
		IncludeLink: f.IncludeLink,
	}
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
