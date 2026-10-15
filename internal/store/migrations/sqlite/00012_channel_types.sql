-- +goose NO TRANSACTION
-- +goose Up
-- More types of channel: slack, discord, telegram, ntfy, pushover. SQLite
-- cannot change a CHECK, so the table is made anew. Foreign keys are off
-- meanwhile, so that dropping the old table does not unlink the
-- notifications sent to its channels.
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE channels_new (
    id               INTEGER PRIMARY KEY,
    scope_id         INTEGER NOT NULL REFERENCES scopes (id) ON DELETE CASCADE,
    slug             TEXT    NOT NULL,
    name             TEXT    NOT NULL,
    type             TEXT    NOT NULL CHECK (type IN ('email', 'googlechat', 'webhook', 'slack', 'discord', 'telegram', 'ntfy', 'pushover')),
    spec_json        TEXT    NOT NULL,
    secret_sealed    TEXT,
    secret_refs_json TEXT    NOT NULL DEFAULT '{}',
    created_at       INTEGER NOT NULL,
    UNIQUE (scope_id, slug)
);
INSERT INTO channels_new (id, scope_id, slug, name, type, spec_json, secret_sealed, secret_refs_json, created_at)
    SELECT id, scope_id, slug, name, type, spec_json, secret_sealed, secret_refs_json, created_at FROM channels;
DROP TABLE channels;
ALTER TABLE channels_new RENAME TO channels;
COMMIT;
PRAGMA foreign_keys = ON;

-- +goose Down
-- Channels of the new types go; their notifications keep their history.
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE channels_old (
    id               INTEGER PRIMARY KEY,
    scope_id         INTEGER NOT NULL REFERENCES scopes (id) ON DELETE CASCADE,
    slug             TEXT    NOT NULL,
    name             TEXT    NOT NULL,
    type             TEXT    NOT NULL CHECK (type IN ('email', 'googlechat', 'webhook')),
    spec_json        TEXT    NOT NULL,
    secret_sealed    TEXT,
    secret_refs_json TEXT    NOT NULL DEFAULT '{}',
    created_at       INTEGER NOT NULL,
    UNIQUE (scope_id, slug)
);
INSERT INTO channels_old (id, scope_id, slug, name, type, spec_json, secret_sealed, secret_refs_json, created_at)
    SELECT id, scope_id, slug, name, type, spec_json, secret_sealed, secret_refs_json, created_at FROM channels
    WHERE type IN ('email', 'googlechat', 'webhook');
UPDATE notifications SET channel_id = NULL WHERE channel_id NOT IN (SELECT id FROM channels_old);
DROP TABLE channels;
ALTER TABLE channels_old RENAME TO channels;
COMMIT;
PRAGMA foreign_keys = ON;
