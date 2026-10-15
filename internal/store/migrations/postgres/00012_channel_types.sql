-- +goose Up
-- More types of channel: slack, discord, telegram, ntfy, pushover.
ALTER TABLE channels DROP CONSTRAINT channels_type_check;
ALTER TABLE channels ADD CONSTRAINT channels_type_check
    CHECK (type IN ('email', 'googlechat', 'webhook', 'slack', 'discord', 'telegram', 'ntfy', 'pushover'));

-- +goose Down
-- Channels of the new types go; their notifications keep their history.
DELETE FROM channels WHERE type NOT IN ('email', 'googlechat', 'webhook');
ALTER TABLE channels DROP CONSTRAINT channels_type_check;
ALTER TABLE channels ADD CONSTRAINT channels_type_check CHECK (type IN ('email', 'googlechat', 'webhook'));
