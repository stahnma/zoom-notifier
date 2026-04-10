package sqlite

import (
	"context"
	"fmt"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) UpsertIRCConfig(ctx context.Context, c *store.IRCConfig) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO irc_configs (tenant_id, server, nick, password, use_tls)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(tenant_id, server) DO UPDATE SET
		   nick = excluded.nick,
		   password = excluded.password,
		   use_tls = excluded.use_tls`,
		c.TenantID, c.Server, c.Nick, c.Password, c.UseTLS,
	)
	if err != nil {
		return fmt.Errorf("upsert irc config: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetIRCConfig(ctx context.Context, tenantID string) ([]*store.IRCConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tenant_id, server, nick, password, use_tls
		 FROM irc_configs WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("get irc config: %w", err)
	}
	defer rows.Close()

	var configs []*store.IRCConfig
	for rows.Next() {
		c := &store.IRCConfig{}
		if err := rows.Scan(&c.TenantID, &c.Server, &c.Nick, &c.Password, &c.UseTLS); err != nil {
			return nil, fmt.Errorf("scan irc config: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}
