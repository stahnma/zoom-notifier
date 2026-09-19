package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migsqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/secrets"
	"github.com/stahnma/zoom-notifier/internal/store"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Compile-time check that SQLiteStore implements store.Store.
var _ store.Store = (*SQLiteStore)(nil)

type SQLiteStore struct {
	db *sql.DB
	// cipher encrypts secret columns at rest. nil means plaintext storage.
	cipher *secrets.Cipher
}

// Option configures a SQLiteStore.
type Option func(*SQLiteStore) error

// WithEncryptionKey enables AES-256-GCM encryption of secret columns
// (bot tokens, API keys, Zoom client secrets, IRC passwords) using a
// hex-encoded 32-byte key. An empty key leaves encryption disabled.
func WithEncryptionKey(hexKey string) Option {
	return func(s *SQLiteStore) error {
		c, err := secrets.New(hexKey)
		if err != nil {
			return err
		}
		s.cipher = c
		return nil
	}
}

func New(dbPath string, opts ...Option) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	s := &SQLiteStore{db: db}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			if closeErr := db.Close(); closeErr != nil {
				log.WithError(closeErr).Warn("failed to close database after option error")
			}
			return nil, err
		}
	}
	return s, nil
}

// EncryptionEnabled reports whether secret columns are encrypted at rest.
func (s *SQLiteStore) EncryptionEnabled() bool {
	return s.cipher.Enabled()
}

func (s *SQLiteStore) Migrate() error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create source driver: %w", err)
	}

	dbDriver, err := migsqlite.WithInstance(s.db, &migsqlite.Config{
		NoTxWrap: false,
	})
	if err != nil {
		return fmt.Errorf("create db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// closeRows closes a sql.Rows and logs if the close fails, which can indicate
// a leaked database connection.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.WithError(err).Warn("failed to close database rows")
	}
}

// secretColumn identifies a column holding a secret that must be encrypted.
type secretColumn struct {
	table, column, keyColumn string
}

// secretColumns lists every column that stores a secret at rest. Keep this in
// sync with the cipher calls in the store methods.
var secretColumns = []secretColumn{
	{"tenants", "bot_token", "id"},
	{"tenants", "api_key", "id"},
	{"zoom_credentials", "client_secret", "tenant_id"},
	{"irc_configs", "password", "rowid"},
}

// EncryptLegacySecrets encrypts any secret values still stored in plaintext.
// It is idempotent and safe to run at every startup; rows that already carry
// the encryption prefix are skipped. Returns the number of rows updated.
func (s *SQLiteStore) EncryptLegacySecrets(ctx context.Context) (int, error) {
	if !s.cipher.Enabled() {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	// Rollback is a no-op after a successful Commit.
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.WithError(err).Warn("failed to roll back secret migration")
		}
	}()

	updated := 0
	for _, col := range secretColumns {
		n, err := s.encryptColumn(ctx, tx, col)
		if err != nil {
			return 0, err
		}
		updated += n
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit secret migration: %w", err)
	}
	return updated, nil
}

func (s *SQLiteStore) encryptColumn(ctx context.Context, tx *sql.Tx, col secretColumn) (int, error) {
	// Table/column names come from the fixed secretColumns list, not user input.
	query := fmt.Sprintf(
		`SELECT %s, %s FROM %s WHERE %s IS NOT NULL AND %s != '' AND %s NOT LIKE ?`,
		col.keyColumn, col.column, col.table, col.column, col.column, col.column,
	)
	rows, err := tx.QueryContext(ctx, query, secrets.Prefix+"%")
	if err != nil {
		return 0, fmt.Errorf("scan %s.%s for plaintext secrets: %w", col.table, col.column, err)
	}

	type pending struct {
		key   any
		value string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.key, &p.value); err != nil {
			closeRows(rows)
			return 0, fmt.Errorf("scan %s.%s row: %w", col.table, col.column, err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		closeRows(rows)
		return 0, fmt.Errorf("iterate %s.%s rows: %w", col.table, col.column, err)
	}
	closeRows(rows)

	update := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ?`, col.table, col.column, col.keyColumn)
	for _, p := range todo {
		sealed, err := s.cipher.Seal(p.value)
		if err != nil {
			return 0, fmt.Errorf("encrypt %s.%s: %w", col.table, col.column, err)
		}
		if _, err := tx.ExecContext(ctx, update, sealed, p.key); err != nil {
			return 0, fmt.Errorf("update %s.%s: %w", col.table, col.column, err)
		}
	}
	return len(todo), nil
}
