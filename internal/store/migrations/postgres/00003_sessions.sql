-- +goose Up
-- A session is a login in a browser. The cookie holds a random token; the
-- hub keeps only its hash.
CREATE TABLE sessions (
    id_hash      TEXT    PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   BIGINT NOT NULL,
    expires_at   BIGINT NOT NULL,
    last_seen_at BIGINT NOT NULL
);
CREATE INDEX sessions_user ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
