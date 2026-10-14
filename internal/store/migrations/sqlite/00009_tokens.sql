-- +goose Up
-- API tokens (decision 0019): for lotse apply and export, and the read API.
-- A token works on its scope and everything below it: the global scope, a
-- customer, a project or an environment. Kept as a hash only.
CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY,
    name         TEXT    NOT NULL,
    kind         TEXT    NOT NULL CHECK (kind IN ('read', 'apply')),
    scope_id     INTEGER NOT NULL REFERENCES scopes (id) ON DELETE CASCADE,
    token_hash   TEXT    NOT NULL UNIQUE,
    created_by   TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER,
    revoked_at   INTEGER
);

-- +goose Down
DROP TABLE api_tokens;
