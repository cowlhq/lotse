-- +goose NO TRANSACTION
-- +goose Up
-- More types of check: dns and domain. As with channels (12), SQLite makes
-- the table anew, with foreign keys off, so that dropping the old one does
-- not delete the checks' results.
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE checks_new (
    id                INTEGER PRIMARY KEY,
    environment_id    INTEGER NOT NULL REFERENCES scopes (id),
    container_id      INTEGER REFERENCES containers (id) ON DELETE CASCADE,
    slug              TEXT    NOT NULL,
    path              TEXT    NOT NULL UNIQUE,
    name              TEXT    NOT NULL,
    type              TEXT    NOT NULL CHECK (type IN ('http', 'tls', 'tcp', 'dns', 'domain', 'heartbeat')),
    spec_json         TEXT    NOT NULL,
    ping_token_hash   TEXT    UNIQUE,
    ping_token_sealed TEXT,
    state             TEXT    NOT NULL DEFAULT 'unknown',
    state_since       INTEGER,
    last_ping         INTEGER,
    created_at        INTEGER NOT NULL
);
INSERT INTO checks_new (id, environment_id, container_id, slug, path, name, type, spec_json, ping_token_hash, ping_token_sealed, state, state_since, last_ping, created_at)
    SELECT id, environment_id, container_id, slug, path, name, type, spec_json, ping_token_hash, ping_token_sealed, state, state_since, last_ping, created_at FROM checks;
DROP TABLE checks;
ALTER TABLE checks_new RENAME TO checks;
CREATE INDEX checks_environment ON checks (environment_id);
COMMIT;
PRAGMA foreign_keys = ON;

-- +goose Down
-- Checks of the new types go, with their results.
DELETE FROM checks WHERE type IN ('dns', 'domain');
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE checks_old (
    id                INTEGER PRIMARY KEY,
    environment_id    INTEGER NOT NULL REFERENCES scopes (id),
    container_id      INTEGER REFERENCES containers (id) ON DELETE CASCADE,
    slug              TEXT    NOT NULL,
    path              TEXT    NOT NULL UNIQUE,
    name              TEXT    NOT NULL,
    type              TEXT    NOT NULL CHECK (type IN ('http', 'tls', 'tcp', 'heartbeat')),
    spec_json         TEXT    NOT NULL,
    ping_token_hash   TEXT    UNIQUE,
    ping_token_sealed TEXT,
    state             TEXT    NOT NULL DEFAULT 'unknown',
    state_since       INTEGER,
    last_ping         INTEGER,
    created_at        INTEGER NOT NULL
);
INSERT INTO checks_old (id, environment_id, container_id, slug, path, name, type, spec_json, ping_token_hash, ping_token_sealed, state, state_since, last_ping, created_at)
    SELECT id, environment_id, container_id, slug, path, name, type, spec_json, ping_token_hash, ping_token_sealed, state, state_since, last_ping, created_at FROM checks;
DROP TABLE checks;
ALTER TABLE checks_old RENAME TO checks;
CREATE INDEX checks_environment ON checks (environment_id);
COMMIT;
PRAGMA foreign_keys = ON;
