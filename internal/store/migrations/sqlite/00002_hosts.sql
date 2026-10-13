-- +goose Up
-- Hosts are known by their machine ID (or hostname, if they have none).
-- Milestone 3 puts them into environments.
CREATE TABLE hosts (
    id             INTEGER PRIMARY KEY,
    machine_id     TEXT    NOT NULL UNIQUE,
    name           TEXT    NOT NULL,
    hostname       TEXT    NOT NULL,
    os             TEXT    NOT NULL DEFAULT '',
    platform       TEXT    NOT NULL DEFAULT '',
    kernel         TEXT    NOT NULL DEFAULT '',
    arch           TEXT    NOT NULL DEFAULT '',
    cpu_model      TEXT    NOT NULL DEFAULT '',
    cores          INTEGER NOT NULL DEFAULT 0,
    mem_total      INTEGER NOT NULL DEFAULT 0,
    boot_time      INTEGER NOT NULL DEFAULT 0,
    agent_version  TEXT    NOT NULL DEFAULT '',
    docker_version TEXT    NOT NULL DEFAULT '',
    first_seen     INTEGER NOT NULL,
    last_seen      INTEGER NOT NULL
);

CREATE TABLE filesystems (
    id        INTEGER PRIMARY KEY,
    host_id   INTEGER NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    mount     TEXT    NOT NULL,
    device    TEXT    NOT NULL DEFAULT '',
    fstype    TEXT    NOT NULL DEFAULT '',
    last_seen INTEGER NOT NULL,
    UNIQUE (host_id, mount)
);

-- A container keeps its row across redeploys: slug is compose project and
-- service (or the container name), docker_id the current container.
CREATE TABLE containers (
    id              INTEGER PRIMARY KEY,
    host_id         INTEGER NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    slug            TEXT    NOT NULL,
    docker_id       TEXT    NOT NULL DEFAULT '',
    name            TEXT    NOT NULL DEFAULT '',
    image           TEXT    NOT NULL DEFAULT '',
    compose_project TEXT    NOT NULL DEFAULT '',
    compose_service TEXT    NOT NULL DEFAULT '',
    state           TEXT    NOT NULL DEFAULT '',
    health          TEXT    NOT NULL DEFAULT '',
    exit_code       INTEGER NOT NULL DEFAULT 0,
    restarts        INTEGER NOT NULL DEFAULT 0,
    started_at      INTEGER NOT NULL DEFAULT 0,
    labels_json     TEXT    NOT NULL DEFAULT '{}',
    first_seen      INTEGER NOT NULL,
    last_seen       INTEGER NOT NULL,
    gone_at         INTEGER,
    UNIQUE (host_id, slug)
);

-- Samples: one row per reading, a column per value. NULL means the value
-- was not measured (rates need two readings).
CREATE TABLE host_samples (
    host_id       INTEGER NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    ts            INTEGER NOT NULL,
    cpu           REAL,
    iowait        REAL,
    steal         REAL,
    load1         REAL,
    load5         REAL,
    load15        REAL,
    mem_used      INTEGER,
    mem_available INTEGER,
    swap_used     INTEGER,
    swap_total    INTEGER,
    net_rx        REAL,
    net_tx        REAL,
    disk_read     REAL,
    disk_write    REAL,
    uptime        INTEGER,
    PRIMARY KEY (host_id, ts)
) WITHOUT ROWID;

CREATE TABLE fs_samples (
    filesystem_id INTEGER NOT NULL REFERENCES filesystems (id) ON DELETE CASCADE,
    ts            INTEGER NOT NULL,
    used          INTEGER NOT NULL,
    total         INTEGER NOT NULL,
    inodes_used   INTEGER,
    inodes_total  INTEGER,
    PRIMARY KEY (filesystem_id, ts)
) WITHOUT ROWID;

CREATE TABLE container_samples (
    container_id INTEGER NOT NULL REFERENCES containers (id) ON DELETE CASCADE,
    ts           INTEGER NOT NULL,
    cpu          REAL,
    mem          INTEGER,
    mem_limit    INTEGER,
    net_rx       REAL,
    net_tx       REAL,
    PRIMARY KEY (container_id, ts)
) WITHOUT ROWID;

-- What happened to an object: reboots, container starts and deaths, agents
-- coming and going.
CREATE TABLE events (
    id           INTEGER PRIMARY KEY,
    ts           INTEGER NOT NULL,
    subject_kind TEXT    NOT NULL,
    subject_id   INTEGER NOT NULL,
    kind         TEXT    NOT NULL,
    data_json    TEXT    NOT NULL DEFAULT '{}'
);
CREATE INDEX events_subject ON events (subject_kind, subject_id, ts);
CREATE INDEX events_ts ON events (ts);

-- +goose Down
DROP TABLE events;
DROP TABLE container_samples;
DROP TABLE fs_samples;
DROP TABLE host_samples;
DROP TABLE containers;
DROP TABLE filesystems;
DROP TABLE hosts;
