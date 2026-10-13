-- +goose Up
-- Probes run checks (design 8.3): the hub's own, number 1, and external
-- ones in other places, which enroll with the probe token (an enrollment
-- token of the global scope).
CREATE TABLE probes (
    id         INTEGER PRIMARY KEY,
    slug       TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    location   TEXT    NOT NULL DEFAULT '',
    builtin    INTEGER NOT NULL DEFAULT 0,
    token_hash TEXT    UNIQUE,
    version    TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    last_seen  INTEGER,
    revoked_at INTEGER
);
INSERT INTO probes (id, slug, name, builtin, created_at) VALUES (1, 'hub', 'Hub', 1, 0);

-- A check of something from outside: http, tls, tcp, or a heartbeat that
-- a job sends. Its key is its environment's path and its slug.
CREATE TABLE checks (
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
CREATE INDEX checks_environment ON checks (environment_id);

CREATE TABLE check_results (
    check_id       INTEGER NOT NULL REFERENCES checks (id) ON DELETE CASCADE,
    probe_id       INTEGER NOT NULL REFERENCES probes (id) ON DELETE CASCADE,
    ts             INTEGER NOT NULL,
    ok             INTEGER NOT NULL,
    latency_ms     REAL,
    status_code    INTEGER,
    cert_not_after INTEGER,
    error          TEXT,
    PRIMARY KEY (check_id, probe_id, ts)
) WITHOUT ROWID;

-- +goose Down
DROP TABLE check_results;
DROP TABLE checks;
DROP TABLE probes;
