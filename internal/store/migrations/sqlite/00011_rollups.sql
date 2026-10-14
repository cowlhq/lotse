-- +goose Up
-- Readings kept longer (design 5.3, milestone 9): for every 5 minutes and
-- every hour, the average of each value under its own name, its maximum
-- as <name>_max, and how many readings went into it. rollups notes how far
-- each table is made, so that each step is made once.
CREATE TABLE rollups (
    name TEXT    PRIMARY KEY,
    upto INTEGER NOT NULL
);

CREATE TABLE host_samples_5m (
    host_id INTEGER NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    cpu REAL,
    cpu_max REAL,
    iowait REAL,
    iowait_max REAL,
    steal REAL,
    steal_max REAL,
    load1 REAL,
    load1_max REAL,
    load5 REAL,
    load5_max REAL,
    load15 REAL,
    load15_max REAL,
    mem_used REAL,
    mem_used_max REAL,
    mem_available REAL,
    mem_available_max REAL,
    swap_used REAL,
    swap_used_max REAL,
    swap_total REAL,
    swap_total_max REAL,
    net_rx REAL,
    net_rx_max REAL,
    net_tx REAL,
    net_tx_max REAL,
    disk_read REAL,
    disk_read_max REAL,
    disk_write REAL,
    disk_write_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (host_id, ts)
) WITHOUT ROWID;

CREATE TABLE host_samples_1h (
    host_id INTEGER NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    cpu REAL,
    cpu_max REAL,
    iowait REAL,
    iowait_max REAL,
    steal REAL,
    steal_max REAL,
    load1 REAL,
    load1_max REAL,
    load5 REAL,
    load5_max REAL,
    load15 REAL,
    load15_max REAL,
    mem_used REAL,
    mem_used_max REAL,
    mem_available REAL,
    mem_available_max REAL,
    swap_used REAL,
    swap_used_max REAL,
    swap_total REAL,
    swap_total_max REAL,
    net_rx REAL,
    net_rx_max REAL,
    net_tx REAL,
    net_tx_max REAL,
    disk_read REAL,
    disk_read_max REAL,
    disk_write REAL,
    disk_write_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (host_id, ts)
) WITHOUT ROWID;

CREATE TABLE fs_samples_5m (
    filesystem_id INTEGER NOT NULL REFERENCES filesystems (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    used REAL,
    used_max REAL,
    total REAL,
    total_max REAL,
    inodes_used REAL,
    inodes_used_max REAL,
    inodes_total REAL,
    inodes_total_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (filesystem_id, ts)
) WITHOUT ROWID;

CREATE TABLE fs_samples_1h (
    filesystem_id INTEGER NOT NULL REFERENCES filesystems (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    used REAL,
    used_max REAL,
    total REAL,
    total_max REAL,
    inodes_used REAL,
    inodes_used_max REAL,
    inodes_total REAL,
    inodes_total_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (filesystem_id, ts)
) WITHOUT ROWID;

CREATE TABLE container_samples_5m (
    container_id INTEGER NOT NULL REFERENCES containers (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    cpu REAL,
    cpu_max REAL,
    mem REAL,
    mem_max REAL,
    mem_limit REAL,
    mem_limit_max REAL,
    net_rx REAL,
    net_rx_max REAL,
    net_tx REAL,
    net_tx_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (container_id, ts)
) WITHOUT ROWID;

CREATE TABLE container_samples_1h (
    container_id INTEGER NOT NULL REFERENCES containers (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    cpu REAL,
    cpu_max REAL,
    mem REAL,
    mem_max REAL,
    mem_limit REAL,
    mem_limit_max REAL,
    net_rx REAL,
    net_rx_max REAL,
    net_tx REAL,
    net_tx_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (container_id, ts)
) WITHOUT ROWID;

CREATE TABLE check_results_5m (
    check_id INTEGER NOT NULL REFERENCES checks (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    latency_ms REAL,
    latency_ms_max REAL,
    ok REAL,
    ok_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (check_id, ts)
) WITHOUT ROWID;

CREATE TABLE check_results_1h (
    check_id INTEGER NOT NULL REFERENCES checks (id) ON DELETE CASCADE,
    ts INTEGER NOT NULL,
    latency_ms REAL,
    latency_ms_max REAL,
    ok REAL,
    ok_max REAL,
    n INTEGER NOT NULL,
    PRIMARY KEY (check_id, ts)
) WITHOUT ROWID;

-- +goose Down
DROP TABLE check_results_1h;
DROP TABLE check_results_5m;
DROP TABLE container_samples_1h;
DROP TABLE container_samples_5m;
DROP TABLE fs_samples_1h;
DROP TABLE fs_samples_5m;
DROP TABLE host_samples_1h;
DROP TABLE host_samples_5m;
DROP TABLE rollups;
