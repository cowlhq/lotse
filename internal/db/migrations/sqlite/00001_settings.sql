-- +goose Up
-- The hub's own settings and state, one value per key, as JSON.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
