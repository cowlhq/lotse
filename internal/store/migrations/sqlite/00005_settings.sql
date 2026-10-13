-- +goose Up
-- A setting set on an object or a scope (design 5.2). Unset means
-- inherited from further up.
CREATE TABLE settings (
    subject_kind TEXT    NOT NULL,
    subject_id   INTEGER NOT NULL,
    key          TEXT    NOT NULL,
    value_json   TEXT    NOT NULL,
    updated_at   INTEGER NOT NULL,
    PRIMARY KEY (subject_kind, subject_id, key)
);

-- +goose Down
DROP TABLE settings;
