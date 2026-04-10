package store

import "time"

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

type ActiveMeeting struct {
	MeetingID string
	TenantID  string
	Topic     string
	HostID    string
	StartTime time.Time
	JoinURL   string
}

type Participant struct {
	ID        int64
	MeetingID string
	UserName  string
	Email     string
	JoinTime  time.Time
	LeaveTime *time.Time // NULL = still in meeting
}

type IRCConfig struct {
	TenantID string
	Server   string
	Nick     string
	Password string // encrypted at rest
	UseTLS   bool
}

type ZoomCredentials struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	AccountID    string
}
