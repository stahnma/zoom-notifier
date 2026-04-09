package store

import (
	"context"
	"time"
)

type Store interface {
	// Tenants
	CreateTenant(ctx context.Context, t *Tenant) error
	GetTenant(ctx context.Context, id string) (*Tenant, error)
	GetTenantByZoomAccount(ctx context.Context, zoomAccountID string) ([]*Tenant, error)
	ListTenants(ctx context.Context) ([]*Tenant, error)
	DeleteTenant(ctx context.Context, id string) error
	UpdateTenant(ctx context.Context, t *Tenant) error
	UpdateTenantAPIKey(ctx context.Context, id string, newKey string) error

	// Tenant Admins
	AddAdmin(ctx context.Context, tenantID string, slackUserID string) error
	RemoveAdmin(ctx context.Context, tenantID string, slackUserID string) error
	ListAdmins(ctx context.Context, tenantID string) ([]*TenantAdmin, error)
	IsAdmin(ctx context.Context, tenantID string, slackUserID string) (bool, error)

	// Subscriptions
	CreateSubscription(ctx context.Context, s *Subscription) error
	GetSubscription(ctx context.Context, id int64) (*Subscription, error)
	UpdateSubscription(ctx context.Context, s *Subscription) error
	DeleteSubscription(ctx context.Context, id int64) error
	ListSubscriptions(ctx context.Context, tenantID string) ([]*Subscription, error)
	GetSubscriptionsForMeeting(ctx context.Context, tenantID string, meetingID string) ([]*Subscription, error)

	// Meeting Filters
	CreateFilter(ctx context.Context, f *MeetingFilter) error
	DeleteFilter(ctx context.Context, id int64) error
	ListFilters(ctx context.Context, tenantID string) ([]*MeetingFilter, error)
	MatchesFilter(ctx context.Context, tenantID string, topic string) (bool, error)

	// Active Meetings
	UpsertMeeting(ctx context.Context, m *ActiveMeeting) error
	GetMeeting(ctx context.Context, meetingID string) (*ActiveMeeting, error)
	ListActiveMeetings(ctx context.Context, tenantID string) ([]*ActiveMeeting, error)
	DeleteMeeting(ctx context.Context, meetingID string) error

	// Participants
	AddParticipant(ctx context.Context, p *Participant) error
	SetParticipantLeft(ctx context.Context, meetingID string, userName string, leaveTime *time.Time) error
	GetActiveParticipants(ctx context.Context, meetingID string) ([]*Participant, error)
	DeleteParticipantsForMeeting(ctx context.Context, meetingID string) error

	// IRC Config
	UpsertIRCConfig(ctx context.Context, c *IRCConfig) error
	GetIRCConfig(ctx context.Context, tenantID string) ([]*IRCConfig, error)

	// Zoom Credentials
	UpsertZoomCredentials(ctx context.Context, z *ZoomCredentials) error
	GetZoomCredentials(ctx context.Context, tenantID string) (*ZoomCredentials, error)

	// Lifecycle
	Close() error
}
