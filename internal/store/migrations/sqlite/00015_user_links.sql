-- +goose Up
-- An invitation, or a new password, comes as a link that works once until
-- it expires (decision 0023). An invited user who has not chosen a password
-- yet has an empty password_hash.
ALTER TABLE users ADD COLUMN link_hash TEXT;
ALTER TABLE users ADD COLUMN link_expires_at INTEGER;
ALTER TABLE users ADD COLUMN last_login_at INTEGER;
CREATE UNIQUE INDEX users_link_hash ON users (link_hash);

-- +goose Down
DROP INDEX users_link_hash;
ALTER TABLE users DROP COLUMN last_login_at;
ALTER TABLE users DROP COLUMN link_expires_at;
ALTER TABLE users DROP COLUMN link_hash;
