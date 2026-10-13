-- +goose Up
-- An environment's enrollment token (decision 0007): the hash finds it,
-- the sealed copy lets the UI show the install command again.
CREATE TABLE enrollment_tokens (
    id             INTEGER PRIMARY KEY,
    environment_id INTEGER NOT NULL REFERENCES scopes (id) ON DELETE CASCADE,
    token_hash     TEXT    NOT NULL UNIQUE,
    token_sealed   TEXT    NOT NULL,
    created_by     TEXT    NOT NULL,
    created_at     INTEGER NOT NULL,
    revoked_at     INTEGER
);
CREATE INDEX enrollment_tokens_environment ON enrollment_tokens (environment_id);

-- An agent's own credential, from its enrollment on.
CREATE TABLE agents (
    id           INTEGER PRIMARY KEY,
    host_id      INTEGER NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    token_hash   TEXT    NOT NULL UNIQUE,
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER,
    revoked_at   INTEGER
);
CREATE INDEX agents_host ON agents (host_id);

-- +goose Down
DROP TABLE agents;
DROP TABLE enrollment_tokens;
