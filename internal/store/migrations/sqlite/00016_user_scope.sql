-- +goose Up
-- A user kept to a customer, project or environment and what is below it
-- (decision 0023); NULL for the whole hub.
ALTER TABLE users ADD COLUMN scope_id INTEGER REFERENCES scopes (id);

-- +goose Down
ALTER TABLE users DROP COLUMN scope_id;
