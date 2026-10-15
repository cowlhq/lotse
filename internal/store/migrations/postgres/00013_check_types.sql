-- +goose Up
-- More types of check: dns and domain.
ALTER TABLE checks DROP CONSTRAINT checks_type_check;
ALTER TABLE checks ADD CONSTRAINT checks_type_check CHECK (type IN ('http', 'tls', 'tcp', 'dns', 'domain', 'heartbeat'));

-- +goose Down
-- Checks of the new types go, with their results.
DELETE FROM checks WHERE type IN ('dns', 'domain');
ALTER TABLE checks DROP CONSTRAINT checks_type_check;
ALTER TABLE checks ADD CONSTRAINT checks_type_check CHECK (type IN ('http', 'tls', 'tcp', 'heartbeat'));
