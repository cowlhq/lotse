-- +goose Up
-- Where notifications go (design 8.4): defined on a scope, usable on it and
-- below it by key. Secrets, such as a webhook address, are either sealed
-- (secret_sealed, a JSON object of the secret fields) or references the hub
-- resolves when it sends (secret_refs_json: {"url": {"env": "NAME"}}).
CREATE TABLE channels (
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

-- The mail server that email channels send through, unless they name
-- their own. There is one, or none.
CREATE TABLE mail_server (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    spec_json        TEXT    NOT NULL,
    secret_sealed    TEXT,
    secret_refs_json TEXT    NOT NULL DEFAULT '{}',
    updated_at       INTEGER NOT NULL
);

-- A maintenance window on an object or a scope: once, from starts_at to
-- ends_at, or again and again by schedule ("sun 02:00-04:00").
CREATE TABLE maintenance (
    id           INTEGER PRIMARY KEY,
    subject_kind TEXT    NOT NULL,
    subject_id   INTEGER NOT NULL,
    starts_at    INTEGER,
    ends_at      INTEGER,
    schedule     TEXT,
    reason       TEXT    NOT NULL DEFAULT '',
    created_by   TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    CHECK ((starts_at IS NOT NULL AND ends_at IS NOT NULL) <> (schedule IS NOT NULL))
);
CREATE INDEX maintenance_subject ON maintenance (subject_kind, subject_id);

-- Where each rule stands on each object (design 8.1). alert_id is the
-- transition into the current state; previous is the state before UNKNOWN,
-- which the rule goes back to when data comes again; notified_state is the
-- state last sent, which is how alerts held back during maintenance go out
-- after it.
CREATE TABLE alert_states (
    rule             TEXT    NOT NULL,
    subject_kind     TEXT    NOT NULL,
    subject_id       INTEGER NOT NULL,
    instance         TEXT    NOT NULL DEFAULT '',
    state            TEXT    NOT NULL,
    since            INTEGER NOT NULL,
    pending          TEXT,
    pending_since    INTEGER,
    recovering_since INTEGER,
    previous         TEXT,
    value            REAL,
    alert_id         INTEGER,
    notified_state   TEXT    NOT NULL DEFAULT 'ok',
    notified_at      INTEGER,
    acked_by         TEXT,
    acked_at         INTEGER,
    PRIMARY KEY (rule, subject_kind, subject_id, instance)
);

-- Every transition, with what it was measured against and why.
CREATE TABLE alerts (
    id             INTEGER PRIMARY KEY,
    rule           TEXT    NOT NULL,
    subject_kind   TEXT    NOT NULL,
    subject_id     INTEGER NOT NULL,
    subject_path   TEXT    NOT NULL,
    instance       TEXT    NOT NULL DEFAULT '',
    from_state     TEXT    NOT NULL,
    to_state       TEXT    NOT NULL,
    at             INTEGER NOT NULL,
    value          REAL,
    threshold      REAL,
    reason         TEXT    NOT NULL DEFAULT '',
    rule_json      TEXT    NOT NULL,
    in_maintenance INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX alerts_subject ON alerts (subject_kind, subject_id, at);
CREATE INDEX alerts_at ON alerts (at);

-- What was sent where, or held back. Pending ones are sent together per
-- channel, and tried again until next_at runs out of attempts.
CREATE TABLE notifications (
    id         INTEGER PRIMARY KEY,
    alert_id   INTEGER NOT NULL REFERENCES alerts (id) ON DELETE CASCADE,
    channel_id INTEGER REFERENCES channels (id) ON DELETE SET NULL,
    channel    TEXT    NOT NULL,
    kind       TEXT    NOT NULL CHECK (kind IN ('change', 'repeat')),
    status     TEXT    NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'suppressed')),
    attempts   INTEGER NOT NULL DEFAULT 0,
    error      TEXT,
    created_at INTEGER NOT NULL,
    next_at    INTEGER,
    sent_at    INTEGER
);
CREATE INDEX notifications_alert ON notifications (alert_id);
CREATE INDEX notifications_pending ON notifications (status, next_at);

-- container.restarting counts a kind of event across all containers.
CREATE INDEX events_kind ON events (kind, ts);

-- +goose Down
DROP INDEX events_kind;
DROP TABLE notifications;
DROP TABLE alerts;
DROP TABLE alert_states;
DROP TABLE maintenance;
DROP TABLE mail_server;
DROP TABLE channels;
