-- +goose Up
-- The tree: one global scope, customers in it, projects in customers,
-- environments in projects. path is the stable key (acme/shop/prod); the
-- name is what people read and may change.
CREATE TABLE scopes (
    id         INTEGER PRIMARY KEY,
    parent_id  INTEGER REFERENCES scopes (id),
    kind       TEXT    NOT NULL CHECK (kind IN ('global', 'customer', 'project', 'environment')),
    slug       TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    path       TEXT    NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);
INSERT INTO scopes (id, parent_id, kind, slug, name, path, created_at) VALUES (1, NULL, 'global', '', '', '', 0);

-- A host lives in an environment; enrollment puts it there.
ALTER TABLE hosts ADD COLUMN environment_id INTEGER REFERENCES scopes (id) ON DELETE SET NULL;

-- What changed, who changed it and how: one row per changed field.
-- batch_id groups what one save, one apply or one deploy's labels changed.
CREATE TABLE changes (
    id           INTEGER PRIMARY KEY,
    at           INTEGER NOT NULL,
    batch_id     TEXT    NOT NULL,
    actor_kind   TEXT    NOT NULL,
    actor_id     INTEGER,
    actor_name   TEXT    NOT NULL,
    via          TEXT    NOT NULL,
    subject_kind TEXT    NOT NULL,
    subject_id   INTEGER NOT NULL,
    subject_path TEXT    NOT NULL,
    field        TEXT    NOT NULL,
    old_json     TEXT,
    new_json     TEXT
);
CREATE INDEX changes_subject ON changes (subject_kind, subject_id, at);

-- +goose Down
DROP TABLE changes;
ALTER TABLE hosts DROP COLUMN environment_id;
DROP TABLE scopes;
