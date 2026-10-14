-- +goose Up
-- Readings kept longer (design 5.3, milestone 9): for every 5 minutes and
-- every hour, the average of each value under its own name, its maximum
-- as <name>_max, and how many readings went into it. rollups notes how far
-- each table is made, so that each step is made once.
CREATE TABLE rollups (
    name TEXT    PRIMARY KEY,
    upto BIGINT NOT NULL
);

CREATE TABLE host_samples_5m (
    host_id BIGINT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    cpu DOUBLE PRECISION,
    cpu_max DOUBLE PRECISION,
    iowait DOUBLE PRECISION,
    iowait_max DOUBLE PRECISION,
    steal DOUBLE PRECISION,
    steal_max DOUBLE PRECISION,
    load1 DOUBLE PRECISION,
    load1_max DOUBLE PRECISION,
    load5 DOUBLE PRECISION,
    load5_max DOUBLE PRECISION,
    load15 DOUBLE PRECISION,
    load15_max DOUBLE PRECISION,
    mem_used DOUBLE PRECISION,
    mem_used_max DOUBLE PRECISION,
    mem_available DOUBLE PRECISION,
    mem_available_max DOUBLE PRECISION,
    swap_used DOUBLE PRECISION,
    swap_used_max DOUBLE PRECISION,
    swap_total DOUBLE PRECISION,
    swap_total_max DOUBLE PRECISION,
    net_rx DOUBLE PRECISION,
    net_rx_max DOUBLE PRECISION,
    net_tx DOUBLE PRECISION,
    net_tx_max DOUBLE PRECISION,
    disk_read DOUBLE PRECISION,
    disk_read_max DOUBLE PRECISION,
    disk_write DOUBLE PRECISION,
    disk_write_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (host_id, ts)
);

CREATE TABLE host_samples_1h (
    host_id BIGINT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    cpu DOUBLE PRECISION,
    cpu_max DOUBLE PRECISION,
    iowait DOUBLE PRECISION,
    iowait_max DOUBLE PRECISION,
    steal DOUBLE PRECISION,
    steal_max DOUBLE PRECISION,
    load1 DOUBLE PRECISION,
    load1_max DOUBLE PRECISION,
    load5 DOUBLE PRECISION,
    load5_max DOUBLE PRECISION,
    load15 DOUBLE PRECISION,
    load15_max DOUBLE PRECISION,
    mem_used DOUBLE PRECISION,
    mem_used_max DOUBLE PRECISION,
    mem_available DOUBLE PRECISION,
    mem_available_max DOUBLE PRECISION,
    swap_used DOUBLE PRECISION,
    swap_used_max DOUBLE PRECISION,
    swap_total DOUBLE PRECISION,
    swap_total_max DOUBLE PRECISION,
    net_rx DOUBLE PRECISION,
    net_rx_max DOUBLE PRECISION,
    net_tx DOUBLE PRECISION,
    net_tx_max DOUBLE PRECISION,
    disk_read DOUBLE PRECISION,
    disk_read_max DOUBLE PRECISION,
    disk_write DOUBLE PRECISION,
    disk_write_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (host_id, ts)
);

CREATE TABLE fs_samples_5m (
    filesystem_id BIGINT NOT NULL REFERENCES filesystems (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    used DOUBLE PRECISION,
    used_max DOUBLE PRECISION,
    total DOUBLE PRECISION,
    total_max DOUBLE PRECISION,
    inodes_used DOUBLE PRECISION,
    inodes_used_max DOUBLE PRECISION,
    inodes_total DOUBLE PRECISION,
    inodes_total_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (filesystem_id, ts)
);

CREATE TABLE fs_samples_1h (
    filesystem_id BIGINT NOT NULL REFERENCES filesystems (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    used DOUBLE PRECISION,
    used_max DOUBLE PRECISION,
    total DOUBLE PRECISION,
    total_max DOUBLE PRECISION,
    inodes_used DOUBLE PRECISION,
    inodes_used_max DOUBLE PRECISION,
    inodes_total DOUBLE PRECISION,
    inodes_total_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (filesystem_id, ts)
);

CREATE TABLE container_samples_5m (
    container_id BIGINT NOT NULL REFERENCES containers (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    cpu DOUBLE PRECISION,
    cpu_max DOUBLE PRECISION,
    mem DOUBLE PRECISION,
    mem_max DOUBLE PRECISION,
    mem_limit DOUBLE PRECISION,
    mem_limit_max DOUBLE PRECISION,
    net_rx DOUBLE PRECISION,
    net_rx_max DOUBLE PRECISION,
    net_tx DOUBLE PRECISION,
    net_tx_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (container_id, ts)
);

CREATE TABLE container_samples_1h (
    container_id BIGINT NOT NULL REFERENCES containers (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    cpu DOUBLE PRECISION,
    cpu_max DOUBLE PRECISION,
    mem DOUBLE PRECISION,
    mem_max DOUBLE PRECISION,
    mem_limit DOUBLE PRECISION,
    mem_limit_max DOUBLE PRECISION,
    net_rx DOUBLE PRECISION,
    net_rx_max DOUBLE PRECISION,
    net_tx DOUBLE PRECISION,
    net_tx_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (container_id, ts)
);

CREATE TABLE check_results_5m (
    check_id BIGINT NOT NULL REFERENCES checks (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    latency_ms DOUBLE PRECISION,
    latency_ms_max DOUBLE PRECISION,
    ok DOUBLE PRECISION,
    ok_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (check_id, ts)
);

CREATE TABLE check_results_1h (
    check_id BIGINT NOT NULL REFERENCES checks (id) ON DELETE CASCADE,
    ts BIGINT NOT NULL,
    latency_ms DOUBLE PRECISION,
    latency_ms_max DOUBLE PRECISION,
    ok DOUBLE PRECISION,
    ok_max DOUBLE PRECISION,
    n BIGINT NOT NULL,
    PRIMARY KEY (check_id, ts)
);

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
