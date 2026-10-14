-- +goose Up
-- Docker labels (design 9.3): lotse.env puts a container into another
-- environment of its host's customer; labels_applied holds the lotse.*
-- labels last applied, so that what a removed label set is undone.
ALTER TABLE containers ADD COLUMN environment_id BIGINT REFERENCES scopes (id) ON DELETE SET NULL;
ALTER TABLE containers ADD COLUMN labels_applied TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE containers DROP COLUMN labels_applied;
ALTER TABLE containers DROP COLUMN environment_id;
