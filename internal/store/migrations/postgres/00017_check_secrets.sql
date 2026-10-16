-- +goose Up
-- A check's secrets, such as a header's token, as a channel keeps them:
-- sealed, or references the hub resolves when it hands the check to its
-- probes (decision 0024).
ALTER TABLE checks ADD COLUMN secret_sealed TEXT;
ALTER TABLE checks ADD COLUMN secret_refs_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE checks DROP COLUMN secret_refs_json;
ALTER TABLE checks DROP COLUMN secret_sealed;
