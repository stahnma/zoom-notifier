package store

import "time"

// Subscription type constants.
const (
	SubscriptionTypeSlack = "slack"
	SubscriptionTypeIRC   = "irc"
)

type Tenant struct {
	ID                 string
	TeamName           string
	BotToken           *string // NULL for IRC-only tenants
	APIKey             string
	InstalledAt        time.Time
	ZoomAccountID      string
	DefaultMsgSuffix   string
	DefaultIncludeLink bool
}

type TenantAdmin struct {
	TenantID    string
	SlackUserID string
}

type Subscription struct {
	ID        int64
	TenantID  string
	Type      string  // "slack" or "irc"
	MeetingID *string // NULL = all meetings
	Target    string
	Enabled   bool
	CreatedAt time.Time
}

type MeetingFilter struct {
	ID          int64
	TenantID    string
	Pattern     string
	MsgSuffix   *string // nullable override
	IncludeLink *bool   // nullable override
}

// ActiveMeeting is per Zoom account: every tenant linked to the account sees it.
type ActiveMeeting struct {
	ZoomAccountID string
	MeetingID     string
	Topic         string
	HostID        string
	StartTime     time.Time
	JoinURL       string
}

type Participant struct {
	ID            int64
	ZoomAccountID string
	MeetingID     string
	UserName      string
	Email         string
	JoinTime      time.Time
	LeaveTime     *time.Time // NULL = still in meeting
}

type IRCConfig struct {
	TenantID    string
	Server      string
	Nick        string
	Password    string // encrypted at rest
	UseTLS      bool
	InsecureTLS bool // skip TLS certificate verification (operator opt-in)
}

// ZoomAccount is a Zoom account (one Server-to-Server OAuth app) that delivers
// webhooks. Many tenants may link to one account via Tenant.ZoomAccountID.
type ZoomAccount struct {
	AccountID     string
	WebhookSecret string // encrypted at rest; empty = fall back to global zoom.webhook_secret
	ClientID      string
	ClientSecret  string // encrypted at rest
	CreatedAt     time.Time
}

// HasAPICredentials reports whether the account can call the Zoom REST API.
func (a *ZoomAccount) HasAPICredentials() bool {
	return a.ClientID != "" && a.ClientSecret != ""
}
