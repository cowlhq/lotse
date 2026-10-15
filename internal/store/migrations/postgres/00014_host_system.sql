-- +goose Up
-- What the agent last said of its machine's services and restarts
-- (protocol.System), as JSON.
ALTER TABLE hosts ADD COLUMN system_json TEXT;

-- +goose Down
ALTER TABLE hosts DROP COLUMN system_json;
