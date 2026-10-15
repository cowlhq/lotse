package store

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// Making the channels table anew (migration 12) keeps the notifications
// linked to their channels, and the channels as they were.
func TestChannelTypesMigration(t *testing.T) {
	ctx := t.Context()
	s, err := open(filepath.Join(t.TempDir(), "lotse.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	files, _ := fs.Sub(migrations, "migrations/sqlite")
	provider, err := goose.NewProvider(goose.DialectSQLite3, s.writer, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 11); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO channels (id, scope_id, slug, name, type, spec_json, created_at) VALUES (5, 1, 'chat', 'Chat', 'googlechat', '{}', 1)`,
		`INSERT INTO alerts (id, rule, subject_kind, subject_id, subject_path, from_state, to_state, at, rule_json) VALUES (1, 'host.disk', 'host', 1, 'web1', 'ok', 'crit', 1, '{}')`,
		`INSERT INTO notifications (alert_id, channel_id, channel, kind, status, created_at) VALUES (1, 5, 'chat', 'change', 'sent', 1)`,
	} {
		if _, err := s.writer.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var channelID *int64
	s.writer.QueryRowContext(ctx, `SELECT channel_id FROM notifications`).Scan(&channelID)
	if channelID == nil || *channelID != 5 {
		t.Errorf("the notification's channel: %v", channelID)
	}
	var on int
	s.writer.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&on)
	if on != 1 {
		t.Error("foreign keys are off after the migration")
	}
	if _, err := s.writer.ExecContext(ctx, `INSERT INTO channels (scope_id, slug, name, type, spec_json, created_at) VALUES (1, 'phone', 'Phone', 'ntfy', '{}', 1)`); err != nil {
		t.Errorf("an ntfy channel: %v", err)
	}
	// And back.
	if _, err := provider.DownTo(ctx, 11); err != nil {
		t.Fatal(err)
	}
	var n int
	s.writer.QueryRowContext(ctx, `SELECT count(*) FROM channels`).Scan(&n)
	if n != 1 {
		t.Errorf("%d channels after going back", n)
	}
}
