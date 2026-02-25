# Zoom Notifier v2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rewrite zoom-notifier as a multi-tenant, API-driven service with native Slack app, per-tenant IRC, persistent meeting state, and spec-first REST API.

**Architecture:** Single Go binary (Approach A monolith). Chi HTTP server receives Zoom webhooks and serves REST API. Slack Socket Mode bot handles slash commands. SQLite stores all tenant/meeting state. IRC relay is a notification sink per-tenant.

**Tech Stack:** Go 1.25, Chi (router), oapi-codegen (OpenAPI codegen), modernc.org/sqlite (pure-Go SQLite), golang-migrate (migrations), Viper (config), slack-go/slack (Slack SDK), go-ircevent (IRC), logrus (logging)

**Design doc:** `docs/plans/2026-02-24-architecture-redesign-design.md`

**Module path:** `github.com/stahnma/mandatoryFun/zoom-notifier`

---

## Phase 1: Foundation

### Task 1: Project Scaffolding

Create the new directory structure alongside existing code. We'll remove old files after the new structure is wired up.

**Files:**
- Create: `cmd/zoom-notifier/main.go`
- Create: `internal/config/config.go`
- Create: `internal/store/store.go`
- Create: `internal/zoom/handler.go`
- Create: `internal/slack/bot.go`
- Create: `internal/irc/relay.go`
- Create: `internal/api/server.go`
- Create: `api/openapi.yaml` (placeholder)
- Create: `config.dev.toml`

**Step 1: Create directory structure**

```bash
mkdir -p cmd/zoom-notifier
mkdir -p internal/{config,store,zoom,slack,irc,api}
mkdir -p api
mkdir -p examples/{slack,api}
```

**Step 2: Create minimal `cmd/zoom-notifier/main.go`**

```go
package main

import (
	"fmt"
	"os"

	"flag"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Version: %s\nCommit: %s\nBuild Date: %s\n", version, commit, buildDate)
		os.Exit(0)
	}

	fmt.Println("zoom-notifier v2 starting...")
}
```

**Step 3: Create `config.dev.toml`**

```toml
[server]
port = 8888
host = "localhost"

[database]
path = "./zoom-notifier.db"

[log]
level = "debug"
```

**Step 4: Update Makefile to build from `cmd/zoom-notifier/`**

Update the build target to point at `./cmd/zoom-notifier/` instead of `.`. Update all cross-compilation targets similarly.

**Step 5: Verify it builds and runs**

Run: `make build && ./zoom-notifier --version`
Expected: Version output with "dev" / "none" / "unknown"

**Step 6: Commit**

```bash
git add cmd/ internal/ api/ config.dev.toml Makefile
git commit -m "feat: scaffold v2 project structure"
```

---

### Task 2: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8888 {
		t.Errorf("expected default port 8888, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("expected default host localhost, got %s", cfg.Server.Host)
	}
	if cfg.Database.Path != "./zoom-notifier.db" {
		t.Errorf("expected default db path, got %s", cfg.Database.Path)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Log.Level)
	}
}

func TestLoadFromTOMLFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "config.toml")
	err := os.WriteFile(tomlPath, []byte(`
[server]
port = 9999
host = "0.0.0.0"

[log]
level = "debug"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tomlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Log.Level)
	}
}

func TestLoadFromEnvVars(t *testing.T) {
	t.Setenv("ZOOM_SECRET", "testsecret")
	t.Setenv("ZOOMNOTIFIER_ADMIN_KEY", "testadminkey")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Zoom.WebhookSecret != "testsecret" {
		t.Errorf("expected zoom secret testsecret, got %s", cfg.Zoom.WebhookSecret)
	}
	if cfg.Admin.APIKey != "testadminkey" {
		t.Errorf("expected admin key testadminkey, got %s", cfg.Admin.APIKey)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `Load` function not defined

**Step 3: Write the implementation**

```go
// internal/config/config.go
package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Zoom     ZoomConfig
	Slack    SlackConfig
	Admin    AdminConfig
	Log      LogConfig
}

type ServerConfig struct {
	Port int
	Host string
}

type DatabaseConfig struct {
	Path          string
	EncryptionKey string
}

type ZoomConfig struct {
	WebhookSecret string
}

type SlackConfig struct {
	ClientID      string
	ClientSecret  string
	AppToken      string
	SigningSecret  string
}

type AdminConfig struct {
	APIKey string
}

type LogConfig struct {
	Level string
}

func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("toml")

	// Defaults
	v.SetDefault("server.port", 8888)
	v.SetDefault("server.host", "localhost")
	v.SetDefault("database.path", "./zoom-notifier.db")
	v.SetDefault("log.level", "info")

	// Environment variable bindings
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.BindEnv("zoom.webhook_secret", "ZOOM_SECRET")
	v.BindEnv("database.encryption_key", "ZOOMNOTIFIER_DB_KEY")
	v.BindEnv("slack.client_id", "SLACK_CLIENT_ID")
	v.BindEnv("slack.client_secret", "SLACK_CLIENT_SECRET")
	v.BindEnv("slack.app_token", "SLACK_APP_TOKEN")
	v.BindEnv("slack.signing_secret", "SLACK_SIGNING_SECRET")
	v.BindEnv("admin.api_key", "ZOOMNOTIFIER_ADMIN_KEY")

	// Load config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/zoom-notifier")
		v.AddConfigPath("/etc/zoom-notifier")
	}

	// Read config file (ignore "not found" errors — env/defaults are fine)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && configPath != "" {
			return nil, err
		}
	}

	cfg := &Config{}
	cfg.Server.Port = v.GetInt("server.port")
	cfg.Server.Host = v.GetString("server.host")
	cfg.Database.Path = v.GetString("database.path")
	cfg.Database.EncryptionKey = v.GetString("database.encryption_key")
	cfg.Zoom.WebhookSecret = v.GetString("zoom.webhook_secret")
	cfg.Slack.ClientID = v.GetString("slack.client_id")
	cfg.Slack.ClientSecret = v.GetString("slack.client_secret")
	cfg.Slack.AppToken = v.GetString("slack.app_token")
	cfg.Slack.SigningSecret = v.GetString("slack.signing_secret")
	cfg.Admin.APIKey = v.GetString("admin.api_key")
	cfg.Log.Level = v.GetString("log.level")

	return cfg, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package with TOML, env, and defaults support"
```

---

### Task 3: Store Interface and Types

Define the Store interface and domain types. No SQLite implementation yet — just the contract.

**Files:**
- Create: `internal/store/types.go`
- Create: `internal/store/store.go`

**Step 1: Write the types**

```go
// internal/store/types.go
package store

import "time"

type Tenant struct {
	ID            string
	TeamName      string
	BotToken      *string // NULL for IRC-only tenants
	APIKey        string
	InstalledAt   time.Time
	ZoomAccountID string
}

type TenantAdmin struct {
	TenantID    string
	SlackUserID string
}

type Subscription struct {
	ID          int64
	TenantID    string
	Type        string // "slack" or "irc"
	MeetingID   *string // NULL = all meetings
	Target      string
	MsgSuffix   string
	IncludeLink bool
	Enabled     bool
	CreatedAt   time.Time
}

type MeetingFilter struct {
	ID       int64
	TenantID string
	Pattern  string
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
```

**Step 2: Write the Store interface**

```go
// internal/store/store.go
package store

import "context"

type Store interface {
	// Tenants
	CreateTenant(ctx context.Context, t *Tenant) error
	GetTenant(ctx context.Context, id string) (*Tenant, error)
	GetTenantByZoomAccount(ctx context.Context, zoomAccountID string) ([]*Tenant, error)
	ListTenants(ctx context.Context) ([]*Tenant, error)
	DeleteTenant(ctx context.Context, id string) error
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
```

**Step 3: Verify it compiles**

Run: `go build ./internal/store/`
Expected: Success (no errors)

**Step 4: Commit**

```bash
git add internal/store/
git commit -m "feat: define Store interface and domain types"
```

---

### Task 4: SQLite Store — Schema and Migrations

**Files:**
- Create: `internal/store/sqlite/sqlite.go`
- Create: `internal/store/sqlite/sqlite_test.go`
- Create: `migrations/000001_initial_schema.up.sql`
- Create: `migrations/000001_initial_schema.down.sql`

**Step 1: Create migration files**

```sql
-- migrations/000001_initial_schema.up.sql
CREATE TABLE IF NOT EXISTS tenants (
    id              TEXT PRIMARY KEY,
    team_name       TEXT,
    bot_token       TEXT,
    api_key         TEXT NOT NULL,
    installed_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    zoom_account_id TEXT
);

CREATE TABLE IF NOT EXISTS tenant_admins (
    tenant_id     TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slack_user_id TEXT NOT NULL,
    PRIMARY KEY (tenant_id, slack_user_id)
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type         TEXT NOT NULL CHECK(type IN ('slack', 'irc')),
    meeting_id   TEXT,
    target       TEXT NOT NULL,
    msg_suffix   TEXT DEFAULT 'the zoom meeting.',
    include_link BOOLEAN DEFAULT 1,
    enabled      BOOLEAN DEFAULT 1,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS meeting_filters (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    pattern   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS active_meetings (
    meeting_id TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    topic      TEXT,
    host_id    TEXT,
    start_time DATETIME,
    join_url   TEXT
);

CREATE TABLE IF NOT EXISTS participants (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    meeting_id TEXT NOT NULL REFERENCES active_meetings(meeting_id) ON DELETE CASCADE,
    user_name  TEXT,
    email      TEXT,
    join_time  DATETIME,
    leave_time DATETIME
);

CREATE TABLE IF NOT EXISTS irc_configs (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    server    TEXT NOT NULL,
    nick      TEXT NOT NULL,
    password  TEXT NOT NULL,
    use_tls   BOOLEAN DEFAULT 1,
    PRIMARY KEY (tenant_id, server)
);

CREATE TABLE IF NOT EXISTS zoom_credentials (
    tenant_id     TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    client_id     TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    account_id    TEXT NOT NULL
);

CREATE INDEX idx_subscriptions_tenant ON subscriptions(tenant_id);
CREATE INDEX idx_subscriptions_meeting ON subscriptions(tenant_id, meeting_id);
CREATE INDEX idx_active_meetings_tenant ON active_meetings(tenant_id);
CREATE INDEX idx_participants_meeting ON participants(meeting_id);
```

```sql
-- migrations/000001_initial_schema.down.sql
DROP TABLE IF EXISTS zoom_credentials;
DROP TABLE IF EXISTS irc_configs;
DROP TABLE IF EXISTS participants;
DROP TABLE IF EXISTS active_meetings;
DROP TABLE IF EXISTS meeting_filters;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS tenant_admins;
DROP TABLE IF EXISTS tenants;
```

**Step 2: Write the SQLite store constructor and migration runner**

```go
// internal/store/sqlite/sqlite.go
package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
// Note: migrations are embedded from the top-level migrations/ directory.
// The go:embed directive is set in the package that owns the files.
// We'll address this in the implementation — see step notes.

type SQLiteStore struct {
	db *sql.DB
}

func New(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
```

Note on migrations embedding: `go:embed` only works with files in the same package directory or subdirectories. The cleanest approach is to put the migration SQL files under `internal/store/sqlite/migrations/` and embed them there.Adjust the directory structure:
- Move: `migrations/` -> `internal/store/sqlite/migrations/`

**Step 3: Write a test that opens a :memory: DB and runs migrations**

```go
// internal/store/sqlite/sqlite_test.go
package sqlite

import (
	"testing"
)

func TestNewInMemory(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()
}

func TestRunMigrations(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify tables exist by querying them
	tables := []string{"tenants", "tenant_admins", "subscriptions", "meeting_filters",
		"active_meetings", "participants", "irc_configs", "zoom_credentials"}
	for _, table := range tables {
		_, err := s.db.Exec("SELECT 1 FROM " + table + " LIMIT 1")
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}
```

**Step 4: Implement the Migrate method**

Add a `Migrate()` method to `SQLiteStore` that uses golang-migrate with the embedded SQL files.

**Step 5: Run tests**

Run: `go test ./internal/store/sqlite/ -v`
Expected: PASS (2 tests)

**Step 6: Commit**

```bash
git add internal/store/sqlite/ migrations/
git commit -m "feat: add SQLite store with schema migrations"
```

---

### Task 5: SQLite Store — Tenant CRUD

**Files:**
- Create: `internal/store/sqlite/tenants.go`
- Create: `internal/store/sqlite/tenants_test.go`

**Step 1: Write the failing tests**

```go
// internal/store/sqlite/tenants_test.go
package sqlite

import (
	"context"
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func setupTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetTenant(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	tenant := &store.Tenant{
		ID:            "T_SLACK_123",
		TeamName:      "Test Team",
		APIKey:        "key-123",
		ZoomAccountID: "zoom-acct-1",
	}
	if err := s.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	got, err := s.GetTenant(ctx, "T_SLACK_123")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.TeamName != "Test Team" {
		t.Errorf("expected team name 'Test Team', got '%s'", got.TeamName)
	}
	if got.APIKey != "key-123" {
		t.Errorf("expected api key 'key-123', got '%s'", got.APIKey)
	}
}

func TestGetTenantByZoomAccount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k1", ZoomAccountID: "zoom-1"})
	s.CreateTenant(ctx, &store.Tenant{ID: "T2", APIKey: "k2", ZoomAccountID: "zoom-1"})
	s.CreateTenant(ctx, &store.Tenant{ID: "T3", APIKey: "k3", ZoomAccountID: "zoom-2"})

	tenants, err := s.GetTenantByZoomAccount(ctx, "zoom-1")
	if err != nil {
		t.Fatalf("get by zoom account: %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tenants))
	}
}

func TestDeleteTenant(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k1"})
	if err := s.DeleteTenant(ctx, "T1"); err != nil {
		t.Fatalf("delete tenant: %v", err)
	}
	got, err := s.GetTenant(ctx, "T1")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestListTenants(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k1"})
	s.CreateTenant(ctx, &store.Tenant{ID: "T2", APIKey: "k2"})

	tenants, err := s.ListTenants(ctx)
	if err != nil {
		t.Fatalf("list tenants: %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tenants))
	}
}

func TestUpdateTenantAPIKey(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "old-key"})
	if err := s.UpdateTenantAPIKey(ctx, "T1", "new-key"); err != nil {
		t.Fatalf("update api key: %v", err)
	}
	got, _ := s.GetTenant(ctx, "T1")
	if got.APIKey != "new-key" {
		t.Errorf("expected new-key, got %s", got.APIKey)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/sqlite/ -v -run TestCreate`
Expected: FAIL — methods not defined

**Step 3: Implement tenant CRUD methods**

Implement `CreateTenant`, `GetTenant`, `GetTenantByZoomAccount`, `ListTenants`, `DeleteTenant`, `UpdateTenantAPIKey` on `SQLiteStore` using `database/sql` queries. Each method takes `context.Context` and uses `s.db.QueryRowContext` / `s.db.ExecContext`.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/sqlite/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/sqlite/tenants.go internal/store/sqlite/tenants_test.go
git commit -m "feat: implement tenant CRUD in SQLite store"
```

---

### Task 6: SQLite Store — Subscriptions

**Files:**
- Create: `internal/store/sqlite/subscriptions.go`
- Create: `internal/store/sqlite/subscriptions_test.go`

**Step 1: Write failing tests**

Test: `CreateSubscription`, `GetSubscription`, `ListSubscriptions`, `GetSubscriptionsForMeeting` (including wildcard NULL meeting_id matching), `UpdateSubscription`, `DeleteSubscription`.

Key test case: a subscription with `meeting_id = NULL` should match any meeting when calling `GetSubscriptionsForMeeting`.

**Step 2: Run to verify failure**

Run: `go test ./internal/store/sqlite/ -v -run TestSubscription`
Expected: FAIL

**Step 3: Implement subscription methods**

Key SQL for `GetSubscriptionsForMeeting`:
```sql
SELECT ... FROM subscriptions
WHERE tenant_id = ? AND enabled = 1
AND (meeting_id = ? OR meeting_id IS NULL)
```

**Step 4: Run tests to verify pass**

Run: `go test ./internal/store/sqlite/ -v -run TestSubscription`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/sqlite/subscriptions.go internal/store/sqlite/subscriptions_test.go
git commit -m "feat: implement subscription CRUD in SQLite store"
```

---

### Task 7: SQLite Store — Meeting State and Participants

**Files:**
- Create: `internal/store/sqlite/meetings.go`
- Create: `internal/store/sqlite/meetings_test.go`

**Step 1: Write failing tests**

Test the full meeting lifecycle:
1. `UpsertMeeting` — create a new active meeting
2. `AddParticipant` — participant joins
3. `GetActiveParticipants` — returns only participants with NULL leave_time
4. `SetParticipantLeft` — sets leave_time
5. `GetActiveParticipants` — now returns one fewer
6. `DeleteMeeting` — cascades to participants

Also test `ListActiveMeetings` for a tenant.

**Step 2: Run to verify failure**

Run: `go test ./internal/store/sqlite/ -v -run TestMeeting`
Expected: FAIL

**Step 3: Implement meeting and participant methods**

**Step 4: Run tests to verify pass**

Run: `go test ./internal/store/sqlite/ -v -run TestMeeting`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/sqlite/meetings.go internal/store/sqlite/meetings_test.go
git commit -m "feat: implement meeting state and participant tracking"
```

---

### Task 8: SQLite Store — Filters, Admins, IRC Config, Zoom Credentials

**Files:**
- Create: `internal/store/sqlite/filters.go`
- Create: `internal/store/sqlite/admins.go`
- Create: `internal/store/sqlite/irc.go`
- Create: `internal/store/sqlite/zoom_creds.go`
- Create: `internal/store/sqlite/filters_test.go`
- Create: `internal/store/sqlite/admins_test.go`
- Create: `internal/store/sqlite/irc_test.go`
- Create: `internal/store/sqlite/zoom_creds_test.go`

**Step 1: Write failing tests for each**

Key behaviors:
- `MatchesFilter`: if no filters exist for a tenant, return `true` (allow all). If filters exist, return `true` only if topic matches one.
- `IsAdmin`: simple boolean lookup.
- `UpsertIRCConfig`: insert or replace on (tenant_id, server) primary key.
- `UpsertZoomCredentials`: insert or replace on tenant_id primary key.

**Step 2: Run to verify failure**

Run: `go test ./internal/store/sqlite/ -v`
Expected: FAIL on new tests

**Step 3: Implement all four sets of methods**

**Step 4: Run tests to verify all pass**

Run: `go test ./internal/store/sqlite/ -v`
Expected: PASS (all tests)

**Step 5: Verify SQLiteStore satisfies the Store interface**

Add to `sqlite.go`:
```go
var _ store.Store = (*SQLiteStore)(nil)
```

Run: `go build ./internal/store/sqlite/`
Expected: Compiles without errors (proves interface satisfaction)

**Step 6: Commit**

```bash
git add internal/store/sqlite/
git commit -m "feat: complete Store interface implementation for SQLite"
```

---

## Phase 2: Zoom Webhook Pipeline

### Task 9: Zoom Webhook Types and CRC Validation

Port the existing CRC logic into the new zoom package with tests.

**Files:**
- Create: `internal/zoom/types.go`
- Create: `internal/zoom/crc.go`
- Create: `internal/zoom/crc_test.go`

**Step 1: Write the types**

```go
// internal/zoom/types.go
package zoom

import "time"

type WebhookPayload struct {
	Payload struct {
		PlainToken string `json:"plainToken"`
		AccountID  string `json:"account_id"`
		Object     struct {
			UUID        string `json:"uuid"`
			ID          string `json:"id"`
			Type        int    `json:"type"`
			Topic       string `json:"topic"`
			HostID      string `json:"host_id"`
			Duration    int    `json:"duration"`
			StartTime   time.Time `json:"start_time"`
			Timezone    string `json:"timezone"`
			Participant struct {
				UserID            string    `json:"user_id"`
				UserName          string    `json:"user_name"`
				Email             string    `json:"email"`
				JoinTime          time.Time `json:"join_time"`
				LeaveTime         time.Time `json:"leave_time"`
				LeaveReason       string    `json:"leave_reason"`
				ID                string    `json:"id"`
				ParticipantUserID string    `json:"participant_user_id"`
				ParticipantUUID   string    `json:"participant_uuid"`
				RegistrantID      string    `json:"registrant_id"`
			} `json:"participant"`
		} `json:"object"`
	} `json:"payload"`
	EventTs int64  `json:"event_ts"`
	Event   string `json:"event"`
}

type CRCResponse struct {
	PlainToken     string `json:"plainToken"`
	EncryptedToken string `json:"encryptedToken"`
}
```

**Step 2: Write the failing CRC test using existing sample data**

```go
// internal/zoom/crc_test.go
package zoom

import (
	"encoding/json"
	"os"
	"testing"
)

func TestValidateCRC(t *testing.T) {
	data, err := os.ReadFile("../../examples/zoom/endpoint_url_validation.json")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	var payload WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.Event != "endpoint.url_validation" {
		t.Fatalf("expected endpoint.url_validation event, got %s", payload.Event)
	}

	resp, err := ValidateCRC(payload, "test-secret")
	if err != nil {
		t.Fatalf("validate crc: %v", err)
	}
	if resp.PlainToken != payload.Payload.PlainToken {
		t.Errorf("plain token mismatch")
	}
	if resp.EncryptedToken == "" {
		t.Error("encrypted token should not be empty")
	}
}

func TestValidateCRC_NotValidationEvent(t *testing.T) {
	payload := WebhookPayload{}
	payload.Event = "meeting.participant_joined"

	_, err := ValidateCRC(payload, "secret")
	if err == nil {
		t.Error("expected error for non-validation event")
	}
}
```

**Step 3: Run to verify failure**

Run: `go test ./internal/zoom/ -v`
Expected: FAIL

**Step 4: Implement ValidateCRC**

```go
// internal/zoom/crc.go
package zoom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func ValidateCRC(payload WebhookPayload, secret string) (*CRCResponse, error) {
	if payload.Event != "endpoint.url_validation" {
		return nil, fmt.Errorf("not a CRC validation event: %s", payload.Event)
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload.Payload.PlainToken))
	return &CRCResponse{
		PlainToken:     payload.Payload.PlainToken,
		EncryptedToken: hex.EncodeToString(h.Sum(nil)),
	}, nil
}
```

**Step 5: Run tests to verify pass**

Run: `go test ./internal/zoom/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/zoom/
git commit -m "feat: add Zoom webhook types and CRC validation"
```

---

### Task 10: Zoom Webhook Handler

The Chi HTTP handler that receives webhooks, resolves tenants, and updates meeting state.

**Files:**
- Create: `internal/zoom/handler.go`
- Create: `internal/zoom/handler_test.go`

**Step 1: Write the failing test**

Test the handler using `httptest.NewRecorder` and the sample JSON payloads. Use a mock store (or the real SQLite `:memory:` store).

Key test cases:
1. CRC validation request returns 200 with encrypted token
2. `meeting.participant_joined` upserts participant in store
3. `meeting.participant_left` sets leave time in store
4. `meeting.started` creates active meeting
5. `meeting.ended` deletes active meeting
6. Unknown event returns 200 (ignored, no error)
7. Unknown zoom_account_id returns 200 (logged, no error — don't leak tenant info)

The handler should accept a `store.Store` and a `webhookSecret string` as dependencies (constructor injection).

It should also accept a "dispatcher" function/interface for sending notifications — but for now, pass a no-op dispatcher. We'll wire the real one in Phase 3.

**Step 2: Run to verify failure**

Run: `go test ./internal/zoom/ -v -run TestHandler`
Expected: FAIL

**Step 3: Implement the handler**

```go
// internal/zoom/handler.go
package zoom

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

// Dispatcher is called when a notification should be sent.
type Dispatcher func(tenantID string, sub *store.Subscription, msg string, payload WebhookPayload)

type Handler struct {
	store         store.Store
	webhookSecret string
	dispatch      Dispatcher
}

func NewHandler(s store.Store, webhookSecret string, dispatch Dispatcher) *Handler {
	return &Handler{store: s, webhookSecret: webhookSecret, dispatch: dispatch}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse payload, handle CRC, resolve tenant, update state, fan out
	// Implementation here — see design doc for full flow
}
```

The handler is a standard `http.Handler` — works directly with Chi.

**Step 4: Run tests to verify pass**

Run: `go test ./internal/zoom/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/zoom/handler.go internal/zoom/handler_test.go
git commit -m "feat: add Zoom webhook handler with tenant resolution and state tracking"
```

---

### Task 11: Zoom API Client (OAuth + Meeting Details)

Port and improve the existing `zoom_api.go` into the new package.

**Files:**
- Create: `internal/zoom/api_client.go`
- Create: `internal/zoom/api_client_test.go`

**Step 1: Write the failing test**

Use `httptest.NewServer` to mock Zoom's OAuth and Meeting endpoints.

```go
func TestGetAccessToken(t *testing.T) {
	// Mock OAuth server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "client-id", "client-secret", "account-id")
	token, err := client.GetAccessToken()
	// ...
}

func TestGetMeetingJoinLink(t *testing.T) {
	// Mock meeting API
	// ...
}
```

Key improvements over current code:
- Token caching (don't fetch a new token every call)
- Configurable base URL (for testing)
- Per-tenant credentials passed at construction time

**Step 2: Implement the API client**

**Step 3: Run tests to verify pass**

Run: `go test ./internal/zoom/ -v -run TestGet`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/zoom/api_client.go internal/zoom/api_client_test.go
git commit -m "feat: add Zoom API client with token caching"
```

---

## Phase 3: Notification Dispatch

### Task 12: Notification Dispatcher

Central fan-out logic: given a webhook event, find all matching subscriptions and dispatch to the right backend.

**Files:**
- Create: `internal/notify/dispatcher.go`
- Create: `internal/notify/dispatcher_test.go`

**Step 1: Write the failing test**

```go
func TestDispatchFanOut(t *testing.T) {
	// Setup: store with tenant, 2 slack subs, 1 IRC sub for same meeting
	// Action: call Dispatch with a participant_joined event
	// Assert: slack sender called twice, IRC sender called once
}

func TestDispatchRespectsFilters(t *testing.T) {
	// Setup: store with tenant and a filter for "Daily Standup"
	// Action: call Dispatch with topic "All Hands"
	// Assert: no senders called (filtered out)
}

func TestDispatchDisabledSubscription(t *testing.T) {
	// Setup: store with a disabled subscription
	// Assert: sender not called for disabled sub
}
```

**Step 2: Implement the dispatcher**

The dispatcher takes the store, a Slack sender interface, and an IRC sender interface. It:
1. Looks up tenants by zoom_account_id
2. Checks meeting filters
3. Gets subscriptions for the meeting
4. Formats message with suffix (and join link if include_link + zoom creds available)
5. Calls the appropriate sender for each subscription

```go
type SlackSender interface {
	Send(ctx context.Context, botToken string, channelID string, msg SlackMessage) error
}

type IRCSender interface {
	Send(ctx context.Context, config *store.IRCConfig, channel string, msg string) error
}

type Dispatcher struct {
	store     store.Store
	slack     SlackSender
	irc       IRCSender
}
```

**Step 3: Run tests**

Run: `go test ./internal/notify/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/notify/
git commit -m "feat: add notification dispatcher with fan-out logic"
```

---

### Task 13: Slack Notification Sender

**Files:**
- Create: `internal/slack/sender.go`
- Create: `internal/slack/sender_test.go`

**Step 1: Write failing test**

Mock `httptest.NewServer` to capture the Slack API call. Verify Block Kit payload structure.

Test cases:
1. Basic join message without link
2. Join message with meeting link (Block Kit button)
3. Leave message (simpler format)

**Step 2: Implement Slack sender using `slack-go/slack` library**

The sender uses the `slack.PostMessage` API (not incoming webhooks — we have bot tokens now). This gives us Block Kit support.

```go
type Sender struct {
	// No state needed — bot token comes per-call from the tenant
}

func (s *Sender) Send(ctx context.Context, botToken string, channelID string, msg SlackMessage) error {
	api := slack.New(botToken)
	_, _, err := api.PostMessageContext(ctx, channelID, msg.ToBlockKitOptions()...)
	return err
}
```

**Step 3: Run tests**

Run: `go test ./internal/slack/ -v -run TestSender`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/slack/sender.go internal/slack/sender_test.go
git commit -m "feat: add Slack notification sender with Block Kit support"
```

---

### Task 14: IRC Relay

Port existing IRC logic to the new package.

**Files:**
- Create: `internal/irc/relay.go`
- Create: `internal/irc/relay_test.go`

**Step 1: Write failing test**

IRC is hard to unit test due to the network connection. Test what we can:
- Message formatting
- Config validation (missing server, nick, etc.)

For the actual send, we test at the integration level or use a mock IRC server.

**Step 2: Implement IRC relay**

Port from existing `irc.go`. Key difference: credentials come from `store.IRCConfig` per-tenant, not from Viper globals.

```go
type Relay struct{}

func (r *Relay) Send(ctx context.Context, config *store.IRCConfig, channel string, msg string) error {
	// Connect, auth, join, send, disconnect — same pattern as existing code
}
```

**Step 3: Run tests**

Run: `go test ./internal/irc/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/irc/
git commit -m "feat: add per-tenant IRC relay"
```

---

## Phase 4: REST API

### Task 15: OpenAPI Spec

Write the full OpenAPI 3.0 spec. This is the source of truth for the REST API.

**Files:**
- Create: `api/openapi.yaml`

**Step 1: Write the spec**

Include all endpoints from the design doc:
- `GET /healthz`
- `POST /webhook/zoom`
- Tenant CRUD (`/api/v1/tenants`)
- Subscriptions CRUD (`/api/v1/tenants/{id}/subscriptions`)
- IRC config (`/api/v1/tenants/{id}/irc`)
- Meeting filters (`/api/v1/tenants/{id}/filters`)
- Meeting state (`/api/v1/tenants/{id}/meetings`)
- Admin management (`/api/v1/tenants/{id}/admins`)
- API key rotation (`/api/v1/tenants/{id}/rotate-key`)
- Zoom credentials (`/api/v1/tenants/{id}/zoom`)

Define security schemes:
- `AdminKey` — for tenant creation
- `TenantKey` — for tenant-scoped operations

Define all request/response schemas referencing the store types.

**Step 2: Validate the spec**

Run: `npx @redocly/cli lint api/openapi.yaml`
(Or use any OpenAPI linter. If Node.js is not available, use an online validator.)

**Step 3: Commit**

```bash
git add api/openapi.yaml
git commit -m "feat: add OpenAPI 3.0 spec for REST API"
```

---

### Task 16: Generate Server Code with oapi-codegen

**Files:**
- Create: `internal/api/generated.go` (generated)
- Create: `internal/api/oapi-codegen.yaml` (config file)

**Step 1: Install oapi-codegen**

```bash
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
```

**Step 2: Create codegen config**

```yaml
# internal/api/oapi-codegen.yaml
package: api
output: generated.go
generate:
  chi-server: true
  models: true
  strict-server: true
```

**Step 3: Generate code**

```bash
oapi-codegen --config internal/api/oapi-codegen.yaml api/openapi.yaml
```

**Step 4: Add a `generate` Makefile target**

```makefile
generate:
	oapi-codegen --config internal/api/oapi-codegen.yaml api/openapi.yaml
```

**Step 5: Verify it compiles**

Run: `go build ./internal/api/`
Expected: Success

**Step 6: Commit**

```bash
git add internal/api/generated.go internal/api/oapi-codegen.yaml Makefile
git commit -m "feat: generate Chi server interfaces from OpenAPI spec"
```

---

### Task 17: API Server Implementation

Implement the generated server interface.

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/middleware.go`
- Create: `internal/api/server_test.go`

**Step 1: Write failing integration tests**

Use the generated test client (oapi-codegen can generate a client too) or use `httptest` directly.

Test the core flow:
1. Health check returns 200
2. Create tenant with admin key → 201
3. Create tenant without admin key → 401
4. Get tenant with tenant API key → 200
5. Get tenant with wrong API key → 403
6. Create subscription → 201
7. List subscriptions → returns the one we created
8. CRUD for filters, IRC config, admins

**Step 2: Implement the server**

```go
// internal/api/server.go
package api

type Server struct {
	store    store.Store
	adminKey string
}

// Implement every method from the generated StrictServerInterface
func (s *Server) GetHealthz(ctx context.Context, request GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{Status: "healthy", Version: version}, nil
}

func (s *Server) PostApiV1Tenants(ctx context.Context, request PostApiV1TenantsRequestObject) (PostApiV1TenantsResponseObject, error) {
	// Create tenant in store, return ID + API key
}

// ... etc for all endpoints
```

**Step 3: Implement auth middleware**

```go
// internal/api/middleware.go
package api

// AdminKeyMiddleware checks Authorization header against deployment admin key
// TenantKeyMiddleware checks Authorization header against tenant's api_key in store
```

**Step 4: Run tests**

Run: `go test ./internal/api/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/server.go internal/api/middleware.go internal/api/server_test.go
git commit -m "feat: implement REST API handlers with auth middleware"
```

---

## Phase 5: Slack App

### Task 18: Slack OAuth Install Flow

**Files:**
- Create: `internal/slack/oauth.go`
- Create: `internal/slack/oauth_test.go`

**Step 1: Write failing test**

Mock Slack's OAuth endpoint. Test:
1. `/slack/install` redirects to Slack with correct client_id and scopes
2. `/slack/callback` with valid code creates tenant in store
3. `/slack/callback` with invalid code returns error

**Step 2: Implement OAuth handlers**

These are standard `http.Handler` functions mounted on the Chi router. They use the `slack-go/slack` library's OAuth methods.

On successful install:
1. Exchange code for bot token
2. Create tenant with workspace ID as tenant ID
3. Add installing user as first admin
4. Generate API key
5. Post welcome message to installing user (ephemeral)

**Step 3: Run tests**

Run: `go test ./internal/slack/ -v -run TestOAuth`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/slack/oauth.go internal/slack/oauth_test.go
git commit -m "feat: add Slack OAuth install flow"
```

---

### Task 19: Slack Socket Mode Bot and Slash Commands

**Files:**
- Create: `internal/slack/bot.go`
- Create: `internal/slack/commands.go`
- Create: `internal/slack/commands_test.go`

**Step 1: Write failing tests for command parsing and permission checks**

Test the command handler logic (not Socket Mode itself — that's an integration concern):
1. Parse `/zoom-notifier status` → calls status handler
2. Parse `/zoom-notifier subscribe #channel` → calls subscribe handler
3. Non-admin calling subscribe → returns permission error
4. Unknown subcommand → returns help text

**Step 2: Implement the command router**

```go
// internal/slack/commands.go
package slack

type CommandHandler struct {
	store store.Store
}

func (h *CommandHandler) Handle(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	parts := strings.Fields(cmd.Text)
	if len(parts) == 0 {
		return h.help(ctx, cmd)
	}
	switch parts[0] {
	case "status":
		return h.status(ctx, cmd)
	case "whois":
		return h.whois(ctx, cmd, parts[1:])
	case "subscribe":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.subscribe(ctx, cmd, parts[1:])
		})
	// ... etc for all commands from design doc
	}
}
```

**Step 3: Implement the Socket Mode bot**

```go
// internal/slack/bot.go
package slack

// Bot wraps the Socket Mode client and routes events to CommandHandler
type Bot struct {
	appToken string
	handler  *CommandHandler
}

func (b *Bot) Start(ctx context.Context) error {
	// Connect via Socket Mode, listen for slash commands, route to handler
}
```

**Step 4: Run tests**

Run: `go test ./internal/slack/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/slack/bot.go internal/slack/commands.go internal/slack/commands_test.go
git commit -m "feat: add Slack slash commands and Socket Mode bot"
```

---

## Phase 6: Wiring, Deployment, Cleanup

### Task 20: Wire Everything Together in main.go

**Files:**
- Modify: `cmd/zoom-notifier/main.go`

**Step 1: Implement the full startup flow**

```go
func main() {
	// 1. Parse flags (--version, --config, --migrate)
	// 2. Load config via config.Load(configPath)
	// 3. Setup logging (logrus level from config)
	// 4. Open SQLite store, run migrations
	// 5. If --migrate flag, exit after migrations
	// 6. Create notification dispatcher (slack sender + irc relay)
	// 7. Create Zoom webhook handler
	// 8. Create API server
	// 9. Build Chi router:
	//    - POST /webhook/zoom -> zoom handler
	//    - GET /slack/install -> slack oauth
	//    - GET /slack/callback -> slack oauth callback
	//    - Mount /api/v1 -> generated API handler
	//    - GET /healthz -> health check
	// 10. Start Slack Socket Mode bot (goroutine)
	// 11. Start HTTP server
	// 12. Graceful shutdown on SIGINT/SIGTERM
}
```

**Step 2: Verify it builds**

Run: `make build`
Expected: Success

**Step 3: Smoke test locally**

```bash
ZOOM_SECRET=devsecret ZOOMNOTIFIER_ADMIN_KEY=devkey ./zoom-notifier --config config.dev.toml
```

Expected: Server starts, logs show listening on localhost:8888. Ctrl-C to stop.

```bash
curl http://localhost:8888/healthz
```

Expected: `{"status":"healthy","version":"dev"}`

**Step 4: Commit**

```bash
git add cmd/zoom-notifier/main.go
git commit -m "feat: wire all components together in main.go"
```

---

### Task 21: Sample Payloads

**Files:**
- Create: `examples/slack/slash_command.json`
- Create: `examples/slack/oauth_callback.json`
- Create: `examples/slack/interactive_message.json`
- Create: `examples/slack/socket_mode_event.json`
- Create: `examples/api/create_tenant_request.json`
- Create: `examples/api/create_tenant_response.json`
- Create: `examples/api/create_subscription_request.json`
- Create: `examples/api/irc_config_request.json`

**Step 1: Create sample Slack payloads**

Reference Slack's documentation for the exact format of slash command payloads, OAuth callbacks, interactive message payloads, and Socket Mode envelopes. Create realistic examples with placeholder values.

**Step 2: Create sample API payloads**

Match the OpenAPI spec schemas exactly.

**Step 3: Update `examples/tests.sh`**

Update to use the new `/webhook/zoom` endpoint path and add API smoke tests using `curl`.

**Step 4: Commit**

```bash
git add examples/
git commit -m "feat: add sample Slack and API payloads"
```

---

### Task 22: Update Makefile and CI

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`

**Step 1: Update Makefile**

Add targets:
```makefile
generate:
	oapi-codegen --config internal/api/oapi-codegen.yaml api/openapi.yaml

test:
	go test ./...

test-verbose:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

dev: build
	./zoom-notifier --config config.dev.toml
```

Ensure `build` target points to `./cmd/zoom-notifier/`.

**Step 2: Update CI workflow**

Add `go test ./...` step. Keep existing build, vet, lint steps.

**Step 3: Verify CI passes locally**

```bash
make build && go vet ./... && go test ./...
```

**Step 4: Commit**

```bash
git add Makefile .github/workflows/ci.yml
git commit -m "ci: update Makefile and CI for v2 project structure"
```

---

### Task 23: GitHub Actions Deployment Workflow

**Files:**
- Create: `.github/workflows/deploy.yml`

**Step 1: Write the deployment workflow**

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    needs: [build-and-test]  # reference CI job
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Build
        run: make linux-amd64

      - name: Deploy
        env:
          DEPLOY_HOST: ${{ secrets.DEPLOY_HOST }}
          DEPLOY_USER: ${{ secrets.DEPLOY_USER }}
          SSH_KEY: ${{ secrets.DEPLOY_SSH_KEY }}
        run: |
          mkdir -p ~/.ssh
          echo "$SSH_KEY" > ~/.ssh/deploy_key
          chmod 600 ~/.ssh/deploy_key
          scp -o StrictHostKeyChecking=no -i ~/.ssh/deploy_key \
            bin/zoom-notifier.linux-amd64 \
            ${DEPLOY_USER}@${DEPLOY_HOST}:/tmp/zoom-notifier
          ssh -o StrictHostKeyChecking=no -i ~/.ssh/deploy_key \
            ${DEPLOY_USER}@${DEPLOY_HOST} \
            'sudo install -p -m0755 /tmp/zoom-notifier /usr/local/bin/zoom-notifier && sudo systemctl restart zoom-notifier'

      - name: Health Check
        run: |
          sleep 5
          curl -f http://${DEPLOY_HOST}:8888/healthz
```

**Step 2: Update systemd service file**

Update `contrib/zoomwh.service` to the new config format (TOML, new binary flags).

**Step 3: Commit**

```bash
git add .github/workflows/deploy.yml contrib/
git commit -m "ci: add GitHub Actions deployment workflow"
```

---

### Task 24: Remove Old Code

**Files:**
- Delete: `main.go`
- Delete: `slack.go`
- Delete: `irc.go`
- Delete: `zoom_api.go`
- Delete: `openapi.yml` (replaced by `api/openapi.yaml`)

**Step 1: Verify all functionality is ported**

Check each old file against the new internal packages:
- `main.go` → `cmd/zoom-notifier/main.go` + `internal/config/`
- `slack.go` → `internal/slack/sender.go` + `internal/notify/dispatcher.go`
- `irc.go` → `internal/irc/relay.go`
- `zoom_api.go` → `internal/zoom/api_client.go`

**Step 2: Verify tests pass**

Run: `go test ./...`
Expected: PASS

**Step 3: Verify it builds and runs**

Run: `make build && ./zoom-notifier --version`
Expected: Version output

**Step 4: Delete old files**

```bash
git rm main.go slack.go irc.go zoom_api.go openapi.yml
```

**Step 5: Commit**

```bash
git commit -m "chore: remove legacy single-package code"
```

---

### Task 25: Update CLAUDE.md and Documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

**Step 1: Update CLAUDE.md**

Reflect the new project structure, build commands (including `make generate`, `make test`, `make dev`), architecture, and configuration.

**Step 2: Update README.md**

Update to describe the v2 architecture, installation, configuration (TOML), multi-tenancy, Slack app setup, IRC setup, and API usage.

**Step 3: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: update CLAUDE.md and README for v2 architecture"
```

---

## Task Dependency Graph

```
Phase 1 (Foundation):
  Task 1 (scaffold) → Task 2 (config) → Task 3 (store interface)
  Task 3 → Task 4 (SQLite migrations) → Task 5 (tenant CRUD)
  Task 5 → Task 6 (subscriptions) → Task 7 (meetings) → Task 8 (filters/admins/irc/zoom)

Phase 2 (Zoom Pipeline):
  Task 8 + Task 2 → Task 9 (CRC) → Task 10 (webhook handler) → Task 11 (API client)

Phase 3 (Notifications):
  Task 10 + Task 8 → Task 12 (dispatcher) → Task 13 (Slack sender) → Task 14 (IRC relay)

Phase 4 (REST API):
  Task 8 → Task 15 (OpenAPI spec) → Task 16 (codegen) → Task 17 (API handlers)

Phase 5 (Slack App):
  Task 13 + Task 17 → Task 18 (OAuth) → Task 19 (slash commands)

Phase 6 (Wiring):
  Task 14 + Task 17 + Task 19 → Task 20 (main.go wiring) → Task 21 (samples)
  Task 20 → Task 22 (Makefile/CI) → Task 23 (deploy) → Task 24 (cleanup) → Task 25 (docs)
```

## Key Commands Reference

```bash
# Build
make build                    # builds cmd/zoom-notifier/
make generate                 # regenerate from OpenAPI spec

# Test
go test ./...                 # all tests
go test ./internal/store/sqlite/ -v  # store tests only
go test -run TestName ./...   # single test

# Run locally
make dev                      # build + run with config.dev.toml
./zoom-notifier --config config.dev.toml

# Lint
go vet ./...
golangci-lint run

# Smoke test
curl http://localhost:8888/healthz
curl -X POST http://localhost:8888/webhook/zoom -d @examples/zoom/participant_joined.json -H 'Content-Type: application/json'
```
