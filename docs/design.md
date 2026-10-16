# lotse design

Approved 2026-10-13. The decisions in [decisions/](decisions/) say what was
chosen and why; this file describes how lotse is built on them, and is kept up
to date as it is built. What the review settled is in
[section 12](#12-settled-during-the-review).

## 1. In one paragraph

One Go binary, `lotse`, runs as **hub** (API, UI, storage, alerting, a built-in
probe), as **agent** on every machine (host and Docker metrics, read-only) or
as **probe** somewhere else (checks from the outside). Agents and probes open
one WebSocket to the hub; nothing connects to them. Everything lives in a tree
of customer → project → environment, and every setting is inherited down that
tree. Configuration comes in through the UI, `lotse apply` or Docker labels,
and ends up in the one database.

```
 browser ──── HTTPS + WebSocket ───┐
                                   ▼
 lotse apply / export ── HTTPS ──► hub ── SQLite or Postgres
                                   ▲  ▲
            outbound WebSocket     │  │  outbound WebSocket
                     ┌─────────────┘  └──────────────┐
            machine: lotse agent              elsewhere: lotse probe
            (/proc, Docker API read-only)     (HTTP, TLS, TCP checks)

 cron job / backup ── HTTPS GET /ping/<token> ──► hub   (heartbeats)
 hub ── HTTPS GET ──► healthchecks.io                   (dead man's switch)
```

## 2. Repository layout

```
cmd/lotse/              main: one binary, subcommands (section 3.3)
internal/
  agent/                collectors (host, Docker), connection, buffer
  probe/                check runners (http, tls, tcp); used by the hub too
  hub/                  HTTP server, setup wizard, sessions, wiring
    api/                REST handlers for the UI and the read API
    live/               browser WebSocket: topics and fan-out
    gateway/            agent and probe WebSocket endpoints, enrollment
  alert/                rule evaluation, state machine, no-data, maintenance
  notify/               channels: email, Google Chat, webhook; dead man's switch
  config/               settings schema, inheritance, effective values
  apply/                YAML model, diff, apply, export, labels → apply
  store/                database access, dialects, migrations/{sqlite,postgres}
  retention/            downsampling and cleanup
  protocol/             wire messages and protocol versions
  update/               releases, download, minisign, the starter (section 10)
  secret/               sealing of stored secrets
  ui/                   go:embed of the built SPA (dist/ is not committed)
client/                 public Go client for the read API (Apache-2.0)
api/openapi.yaml        the HTTP API, source for server types, client, UI types
web/                    the SPA (Vite); builds into internal/ui/dist
site/                   lotse.sh: the landing page, style.css, fonts/, install;
                        internal/sitegen adds the pages made from docs/
deploy/
  Dockerfile            one image for every mode
  compose.yml           what the install script writes to /opt/lotse
  lotse-agent.service   systemd unit
docs/                   design.md, decisions/, the user documentation
compose.yml             local development: hub, agent, Postgres profile, demo
mise.toml, .goreleaser.yaml, release-please-config.json, .github/workflows/
LICENSE (AGPL-3.0), client/LICENSE (Apache-2.0), NOTICE, README.md, AGENTS.md
```

`internal/ui` rather than `web/` holds the `go:embed`, because a Go package in
`web/` would be importable from outside. Without a built UI the hub serves a
page that points to the release binaries (0002).

## 3. Stack

### 3.1 Go (1.27)

| Purpose | Library | License |
|---|---|---|
| HTTP routing | `net/http` (`ServeMux` patterns) | BSD-3 |
| Logging | `log/slog` | BSD-3 |
| CLI | standard library `flag`, a small dispatcher as in cowl | — |
| WebSocket | `github.com/coder/websocket` (as in cowl) | ISC |
| SQLite | `modernc.org/sqlite` | BSD-3 |
| Postgres | `github.com/jackc/pgx/v5` via `database/sql` | MIT |
| Migrations | `github.com/pressly/goose/v3` (as in cowl) | MIT |
| Host metrics | `github.com/shirou/gopsutil/v4` | BSD-3 |
| Docker | `github.com/moby/moby/client` (as in cowl) | Apache-2.0 |
| YAML | `go.yaml.in/yaml/v3` | MIT, Apache-2.0 |
| Passwords | `golang.org/x/crypto/argon2` (argon2id) | BSD-3 |
| ACME | `golang.org/x/crypto/acme/autocert` | BSD-3 |
| Mail | `github.com/wneessen/go-mail` | MIT |
| Signatures | `aead.dev/minisign` | MIT |
| API code | `oapi-codegen` (server types, `client/`) | Apache-2.0 |

No ORM. SQL is written by hand in `internal/store`.

The API is written spec-first in `api/openapi.yaml`. oapi-codegen generates
the Go server interfaces and the public client from it, and
openapi-typescript generates the UI's types. The read API is the part of the
spec under `/api/v1` tagged `read`, kept stable for other programs; the
rest is the UI's own (decision 0020, docs/api.md).

### 3.2 Web

Vite, React 19, TypeScript, TanStack Router and Query, Tailwind 4, shadcn/ui,
uPlot for time series, i18next for texts, openapi-typescript for API types,
cmdk for search, sonner for toasts, Fira Sans and Fira Mono (SIL OFL).
pnpm, ESLint, `tsc --noEmit` in `check`. All MIT unless named. How the
interface looks and works is decision 0018.

### 3.3 The binary

```
lotse hub                     run the hub
lotse agent --hub URL [--token T]   run the agent (enrolls on first run)
lotse probe --hub URL [--token T]   run an external probe
lotse start <mode> [flags]    the starter: run the newest verified binary (section 10)
lotse apply -f FILE [--dry-run] [--prune --scope PATH]
lotse export [PATH]           print a scope as YAML
lotse backup FILE             online backup of a SQLite hub
lotse version
```

Hub settings come from the environment: `LOTSE_DATA_DIR` (`/var/lib/lotse`),
`LOTSE_LISTEN` (`:8080`), `LOTSE_URL` (public address, asked by the wizard if
unset), `LOTSE_DATABASE_URL`, `LOTSE_ACME_DOMAIN`. The setup wizard writes what
it learns to `hub.json` in the data directory. The key that seals stored
secrets is generated at setup into `secret.key`, or taken from
`LOTSE_SECRET_KEY`.

### 3.4 Tooling, CI, releases

- `mise.toml`: Go, Node 24, pnpm 10; tasks `setup`, `dev` (hub, agent, web dev
  server), `check` (gofmt, vet, `go test -race` against SQLite and Postgres,
  lint, tsc), `build`.
- `ci.yml`: `mise run check` on every pull request and on `main`, with a
  Postgres service container.
- `release.yml`: release-please; when it tags a release, the same run checks
  the tag and then builds:
  - with GoReleaser: binaries as tar.gz for linux amd64 and arm64,
    `checksums.txt` and `checksums.txt.minisig`
  - from `deploy/Dockerfile`, the same file `docker compose` builds:
    `ghcr.io/cowlhq/lotse:<version>` and `:latest`, multi-arch, signed with
    cosign
- `site.yml`: publishes `site/` to GitHub Pages when it changes on `main`.

## 4. Runtime

- **Hub.** One process: HTTP server, the gateway for agents and probes, the
  built-in probe, the alert loop, the notifier, retention, the release
  watcher. SQLite gets one writer connection plus a pool of readers. Samples
  are written in batches, one transaction per second.
- **Agent.** Gathers host metrics every 15 s and container metrics every 30 s.
  It listens to Docker events for state changes and lists all containers every
  5 minutes as a safety net. It keeps the last 15 minutes in memory while the
  hub is away. It runs as user `lotse` in group `docker`, or as a container
  with the socket mounted read-only. The docs show how to put a
  [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) in
  between, because even a read-only mounted socket allows the full API.
- **Probe.** Receives its checks from the hub, runs them, sends results. The
  hub's built-in probe is the same code, called in-process.
- **Grace after start.** For 2 minutes after the hub starts, "no data" alerts
  wait, so agents have time to reconnect.

## 5. Data model

### 5.1 Tree and keys

Four levels of scope: **global** (exactly one) → **customer** → **project** →
**environment**. Hosts, containers and checks belong to an environment.

Every object has a stable key: its kind and its path of slugs.

| Kind | Key | Example |
|---|---|---|
| customer | `customer:<c>` | `customer:acme` |
| project | `project:<c>/<p>` | `project:acme/shop` |
| environment | `environment:<c>/<p>/<e>` | `environment:acme/shop/prod` |
| host | `host:<c>/<p>/<e>/<host>` | `host:acme/shop/prod/web1` |
| container | `container:<c>/<p>/<e>/<host>/<container>` | `container:acme/shop/prod/web1/shop-app` |
| check | `check:<c>/<p>/<e>/<check>` | `check:acme/shop/prod/homepage` |
| check of a container | `check:<container path>/<check>` | `check:acme/shop/prod/web1/shop-app/web` |

The kind is part of the key, so a host and a check may share a name.

A **container's slug** is `<compose project>-<compose service>` (plus `-<n>`
for replicas beyond the first). Without compose it is the container name. The
slug therefore survives a redeploy that recreates the container with a new
Docker ID, and history, settings and checks stay attached. `lotse.name`
overrides it.

A container usually lives in its host's environment. The `lotse.env` label can
put it into another one (section 9.3). Its key then starts with that
environment and still contains the host's slug.

### 5.2 Settings and inheritance

Everything that can be inherited is a **setting**: a key from a schema defined
in `internal/config`, with a value set on any scope or object. The effective
value for an object is the one set closest to it. If nothing is set, the
built-in default applies. A container's settings come from its host first,
then the host's environment and up (decision 0017).

```
settings(subject_kind, subject_id, key, value_json, updated_at)
  unique (subject_kind, subject_id, key)
```

The schema (excerpt; it grows with the milestones):

| Key | Default | Applies to |
|---|---|---|
| `agent.interval.host` | `15s` | hosts |
| `agent.interval.containers` | `30s` | hosts |
| `agent.auto_update` | `false` | hosts |
| `agent.update_window` | `daily 04:00-06:00` | hosts |
| `check.interval` | `60s` | checks |
| `check.timeout` | `10s` | checks |
| `check.probes` | `all` | checks |
| `notify.channels` | `[]` | everything |
| `notify.repeat` | `0` (off) | everything |
| `rules.<rule>.enabled`, `.warn`, `.crit`, `.for`, `.recover` | per rule | per rule |
| `ignore` | `false` | containers |
| `retention.raw` / `.5m` / `.1h` | `2d` / `30d` / `365d` | global only |
| `deadman.url`, `deadman.interval` | empty / `1m` | global only |

A value that is a list replaces the inherited list; it is not merged.

The API answers "effective configuration" for every object as a list of keys
with: value, the scope or object it comes from, and the values it overrides
further up. The UI shows exactly that, with an "override here" and "reset to
inherited" action per key.

Settings attach to scopes *and* objects through `(subject_kind, subject_id)`.
The same pair is used by maintenance windows, alerts and the change history.
A plain foreign key per kind would be safer in the database but would multiply
every one of these tables; the store deletes the rows of an object in the same
transaction as the object itself.

### 5.3 Tables

Times are integer Unix seconds everywhere; ties are broken by `id`. JSON is
stored as text and never queried inside, which keeps the SQL the same for
both databases.

**Tree and objects**

```
scopes        id, parent_id, kind, slug, name, path (unique), created_at
hosts         id, environment_id, slug, name, path (unique), machine_id,
              os, arch, kernel, cpu_model, cores, mem_total, boot_time,
              agent_version, first_seen, last_seen, created_at
containers    id, host_id, environment_id, slug, path (unique), docker_id,
              name, image, state, health, restarts, started_at,
              labels_json, labels_hash, labels_applied_hash, last_seen, gone_at
filesystems   id, host_id, mount, device, fstype, last_seen
checks        id, environment_id, container_id (null unless from a container),
              slug, path (unique), type (http|tls|tcp|heartbeat),
              spec_json, ping_token_hash (heartbeats), created_at
probes        id, slug, name, location, builtin, token_hash, version,
              last_seen, revoked_at
channels      id, scope_id, slug, name, type (email|googlechat|webhook|
              slack|discord|telegram|ntfy|pushover),
              spec_json, secret_sealed, secret_refs_json, created_at
mail_server   spec_json, secret_sealed, secret_refs_json (one row)
maintenance   id, subject_kind, subject_id, starts_at, ends_at, schedule,
              reason, created_by
```

**Measurements**, one row per sample. They are wide, not one row per metric,
because that keeps SQLite small and fast for exactly these queries.

```
host_samples       host_id, ts, cpu, iowait, steal, load1, load5, load15,
                   mem_used, mem_available, swap_used, swap_total,
                   net_rx, net_tx, disk_read, disk_write, uptime
                   primary key (host_id, ts)
fs_samples         filesystem_id, ts, used, total, inodes_used, inodes_total
container_samples  container_id, ts, cpu, mem, mem_limit, net_rx, net_tx
check_results      check_id, probe_id, ts, ok, latency_ms, status_code,
                   cert_not_after, error
events             id, ts, subject_kind, subject_id, kind, data_json
                   (reboot, container start/die/oom/health, agent connect and
                   disconnect, updates, apply runs)
```

Beside each of the four sample tables are `_5m` and `_1h` tables (decision
0021): per object and step, the average of each column under its own name,
the maximum as `<column>_max`, and `n`, the number of readings. For checks,
the average of `ok` is the share of runs that passed. `rollups` holds how far
each of them is made (`name`, `upto`).

**Alerting**

```
alert_states   rule, subject_kind, subject_id, instance, state, since,
               pending, pending_since, recovering_since, previous, value,
               alert_id, notified_state, notified_at, acked_by, acked_at
               unique (rule, subject_kind, subject_id, instance)
alerts         id, rule, subject_kind, subject_id, subject_path, instance,
               from_state, to_state, at, value, threshold,
               rule_json (the effective rule with where each value came from),
               in_maintenance
notifications  id, alert_id, channel_id, channel, kind (change|repeat),
               status (pending|sent|failed|suppressed), attempts, error,
               created_at, next_at, sent_at
```

`instance` separates several alerts of one rule on one object, such as two
full filesystems on one host.

**Access**

```
users              id, email (unique), name, password_hash (argon2id),
                   role (admin|viewer), created_at, disabled_at
sessions           id_hash, user_id, created_at, expires_at, last_seen_at
api_tokens         id, name, kind (read|apply), token_hash, created_by,
                   created_at, expires_at, last_used_at, revoked_at
enrollment_tokens  id, environment_id, token_hash, created_by, created_at,
                   expires_at, revoked_at
agents             id, host_id, token_hash, created_at, last_seen_at,
                   version, protocol, revoked_at
```

Tokens are random and carry a prefix: `lotse_enr_`, `lotse_agt_`, `lotse_prb_`,
`lotse_api_`. GitHub's secret scanning and humans can then tell them apart. The
hub only ever stores a hash.

**History**

```
changes   id, at, batch_id, actor_kind (user|token|label|system),
          actor_id, actor_name, via (ui|apply|label|api|setup|system),
          subject_kind, subject_id, subject_path, field, old_json, new_json
```

One row per changed field. `batch_id` groups one UI save, one `apply` or one
label change. `subject_path` stays readable after the object is gone. For
secrets the row only says that they changed.

### 5.4 SQLite and Postgres

- The SQL is written with `?` placeholders. A small function turns them into
  `$1…` for Postgres.
- Both databases support `INSERT … ON CONFLICT … DO UPDATE` and `RETURNING`.
  Nothing beyond the common subset is used.
- Migrations live in `store/migrations/sqlite/` and `store/migrations/postgres/`
  with the same numbers. A test fails if the two sets differ.
- The same store tests run against both databases, locally (Postgres from
  `compose.yml`) and in CI.
- SQLite runs with WAL, `synchronous=NORMAL`, `foreign_keys=ON` and a
  `busy_timeout`. Retention deletes in small batches, so it never blocks
  writers for long.
- Migrations follow expand, then contract. The previous version must run on
  the new schema, so rolling back the binary (section 10) does not need a
  restore. Before each hub update, a SQLite hub also writes a backup with
  `VACUUM INTO`.

## 6. Agent and probe protocol

- Endpoints: `GET /api/agent/v1/connect` and `GET /api/probe/v1/connect`,
  upgraded to WebSocket. The client sends `Authorization: Bearer <credential>`
  and offers subprotocols `lotse.1` (later also `lotse.2`, …). The hub picks the
  highest one it speaks. It speaks the current version and the one before.
- Messages are JSON text frames with permessage-deflate, each with a `type`.
  Unknown types are logged and ignored in both directions, so new message
  types do not break old peers.
- Ping every 20 s. A connection without traffic for 60 s is dropped. The agent
  reconnects with exponential backoff (1 s to 60 s) plus jitter.

Agent → hub:

| Type | When | Content |
|---|---|---|
| `hello` | after connecting | version, protocol, os/arch, hostname, machine ID, kernel, CPU, cores, memory, boot time, Docker version, a rollback if one happened |
| `host` | every interval | one host sample, filesystems included |
| `containers` | on connect, on any change, every 5 min | full inventory: ID, name, image, compose project and service, state, health, restarts, started, labels |
| `container_event` | as Docker reports it | ID, action (start, die, oom, health_status, …), exit code |
| `container_stats` | every interval | CPU, memory, network per container |
| `system` | every minute | systemd's services, and units of other kinds that failed, asked over D-Bus; whether updates wait for a restart (`/run/reboot-required` on Debian and Ubuntu, a new kernel on NixOS) |

Samples sent from the buffer after a reconnect carry their original
timestamps. A machine whose clock is off would put its readings at the
wrong time: the hub compares when host readings arrive with their stamps,
and once the last five agree on an offset of more than 30 s, it moves what
that agent sends to its own time and shows the offset on the host.

Hub → agent:

| Type | Content |
|---|---|
| `welcome` | the host's key, intervals, whether live rate is on |
| `config` | new intervals, when the settings change |
| `live` | live rate on or off; it ends on its own after 60 s unless renewed |
| `update` | a version to update to (section 10) |

That is the complete list of what the hub can say. There is no message that
makes an agent run anything else.

Probes: the hub sends `checks` (the full list on connect, changes after that);
the probe sends `result` per run.

## 7. Enrollment and onboarding

1. **Start the hub.** `curl -fsSL https://lotse.sh/install | sudo sh -s -- hub`,
   or the compose file by hand. The log and the script print the setup URL.
2. **Setup wizard.** Setup token → database → admin account → public address
   (prefilled from the request) → optionally an email channel for alerts and
   the dead man's switch URL.
3. **Customer, project, environment.** One dialog with three fields; an
   existing customer or project can be picked instead of typing a new one.
4. **"Add agent"** on an environment shows a ready one-liner with the
   environment's enrollment token, for systemd or for Docker:

   ```
   curl -fsSL https://lotse.sh/install | sudo sh -s -- agent --hub https://… --token lotse_enr_…
   ```

   The install script detects the architecture, downloads the release from
   GitHub, checks the checksum (and the signature if `minisign` is present),
   installs the starter as `/usr/local/bin/lotse`, creates user `lotse` (in
   group `docker`), and writes and starts `lotse-agent.service`.
5. **First run.** The agent calls `POST /api/agent/v1/enroll` with the token,
   its hostname and `/etc/machine-id`, and receives its host key and its own
   credential. It stores them in `/var/lib/lotse/agent.json` (0600); the
   enrollment token is not kept. The host and its containers appear in the UI
   within seconds, and the default rules apply.

An enrollment token stays valid until it is revoked, so the same line works
for every machine in the environment. Enrolling again with the same machine ID
in the same environment, after a reinstall for example, takes over the existing
host: it gets a new credential, the old one is revoked, and the history keeps
both. External probes enroll the same way, with a probe token from the global
settings.

## 8. Alerting

### 8.1 States and rules

States: OK, WARN, CRIT, UNKNOWN. A rule has a condition on a value, `warn` and
`crit` thresholds, `for` (how long the condition must hold) and `recover` (how
long the value must stay back on the good side before the alert clears). The
good side is the threshold minus a margin, the hysteresis, which keeps a value
near the threshold from flapping.

```
OK ──condition true──► pending ──held for `for`──► WARN / CRIT ──► notify
WARN/CRIT ──beyond threshold − margin──► recovering ──held for `recover`──► OK ──► notify
no input data ──► UNKNOWN (not notified on its own; the "silent" rule covers it)
```

The rule loop runs every 10 s. Every transition is written to `alerts` with
the value, the threshold and the effective rule, including where each value
came from. The UI therefore answers "why does this fire" from the alert
itself, and marks transitions in the graphs.

### 8.2 Default rules

Active from the first minute, defined in code at the global level, and
overridable like any setting:

| Rule | Condition | Default |
|---|---|---|
| `host.silent` | no data from the agent | CRIT after 2 min |
| `host.disk` | filesystem usage | WARN 85 %, CRIT 95 %, for 5 min |
| `container.unhealthy` | health check `unhealthy` | CRIT after 2 min |
| `container.restarting` | restarts | CRIT at ≥ 3 in 10 min |
| `check.down` | runs in a row that failed on most of the check's probes | CRIT at 2 |
| `host.unit` | a systemd unit failed | WARN |
| `host.service` | a service named in `host.services` does not run | CRIT after 2 min |
| `host.reboot` | updates wait for a restart | WARN after 24 h |
| `check.cert` | certificate expires (any https check, any tls check) | WARN < 14 d, CRIT < 7 d |
| `check.domain` | domain registration expires | WARN < 30 d, CRIT < 7 d |
| `check.heartbeat` | no ping within period + grace | CRIT |
| `probe.silent` | an external probe is gone | WARN after 2 min, routed via global |

There are also rules for CPU, memory, swap, load and I/O wait. They ship
disabled and can be enabled at any level.

### 8.3 Checks and probes

- **HTTP(S):** status code (default 2xx/3xx), optional keyword, latency. For
  https the certificate's expiry is recorded with every run.
- **TLS:** handshake to `host:port`, expiry.
- **TCP:** connect to `host:port`.
- **DNS:** a name's A, AAAA, CNAME, MX, NS or TXT records, optionally
  through a given resolver; with `expect`, values that must be among them.
- **Domain:** a registration's expiry, from its registry over RDAP (the
  server found through IANA's list). Only the hub runs it, every 6 hours;
  `check.domain` warns at 30 days and is critical at 7. Registries that
  publish no date, such as DENIC for .de, give none, and nothing fires.
- **Heartbeat:** `GET|POST|HEAD /ping/<token>` reports success,
  `/ping/<token>/fail` reports failure. Settings: period and grace.

A check runs on the probes in `check.probes` (default: all) that run its
type: a probe says which types it runs when it connects (`X-Lotse-Checks`);
one too old to say runs HTTP, TLS and TCP. It counts as down
when the majority of the probes that are online see it failing. With a single
probe, that probe decides. If none of its probes is online, the check is
UNKNOWN and `probe.silent` fires.

### 8.4 Notifications

- Notified on WARN, CRIT, escalation from WARN to CRIT, and recovery. With
  `notify.repeat`, a still-firing alert is sent again at that interval,
  until someone acknowledges it (decision 0017).
- Channels are email, Slack, Discord, Google Chat, Telegram, ntfy, Pushover
  and webhooks. Push services get critical alerts with their highest
  priority short of an emergency. Each message stays within the service's
  limit of length; what does not fit is counted, with the hub's address.
- What comes for one channel within 30 seconds goes out as one message.
- The channels are `notify.channels`, effective on the alert's object. This is
  how alerts are routed: for example, set it on customer X and X's alerts go to
  X's Google Chat space. Channels are defined on a scope and are usable below
  it.
- During maintenance on an object or any scope above it, transitions are
  recorded and marked, but not sent. Alerts still firing when the window ends
  are sent then. Windows are once, or by schedule (`sun 02:00-04:00`); a
  window on a host covers its containers.
- Sending is retried with backoff. Failures are recorded and shown, and a
  channel that keeps failing raises an alert to the global channels.
- **Dead man's switch:** the alert loop requests `deadman.url` every
  `deadman.interval`, but only while it is running properly. If the hub stops
  or the loop hangs, the external service raises the alarm.

### 8.5 Secrets

The webhook URL of a channel or an SMTP password either comes in through the
UI and is stored sealed (AES-256-GCM, key from 3.3), or is a reference that
the hub resolves when sending: `{env: NAME}` or `{file: /run/secrets/name}`,
but neither a `LOTSE_` variable nor a file outside `/run/secrets`. Email
channels send through the hub's mail server (Settings), or one of their own.
Export writes references, never values. A secret entered in the UI is
exported as a placeholder, `{secret: <channel>}`, and `apply` leaves an
existing secret untouched when it sees that placeholder. Labels cannot set
channels at all.

## 9. Configuration: UI, apply, labels

### 9.1 One store

The database is the only truth. UI, `apply` and labels write to it in the same
way and record the same history (`via`). The last write wins. The UI does not
show where an object came from and protects nothing from editing.

### 9.2 `lotse apply` and `lotse export`

```yaml
# lotse.yaml
customers:
  - key: acme
    name: ACME GmbH
    channels:
      - key: acme-chat
        googlechat:
          webhook: {env: ACME_CHAT_WEBHOOK}
    settings:
      notify.channels: [acme-chat]
    projects:
      - key: shop
        environments:
          - key: prod
            settings:
              rules.host.disk.warn: 80
            checks:
              - key: homepage
                http: {url: "https://shop.acme.de", keyword: "Willkommen"}
              - key: nightly-backup
                heartbeat: {period: 24h, grace: 1h}
            hosts:
              - key: web1
                settings:
                  agent.auto_update: true
```

- `lotse apply -f lotse.yaml` sends the file to the hub (`POST /api/v1/apply`),
  authenticated with an API token of kind `apply`, for everything or for one
  scope (decision 0019). The hub matches objects by key, computes the
  change and applies it in one transaction. Secrets are references
  (`{env: …}`, `{file: …}`), read by `lotse apply` from its own environment
  (`{from_env: …}`), or kept as they are (`{secret: …}`).
- `--dry-run` prints the diff instead, and for each object who last changed it
  and how ("changed 2026-10-12 by andreas via ui").
- `--prune --scope acme/shop` also deletes what the file does not mention, but
  only inside that scope. Hosts and containers are never pruned, because they
  come from agents, not from files.
- A host that the file names but that has not enrolled yet is created as
  "waiting for agent". Its settings are already in place when the agent
  arrives, and `host.silent` waits until the host's first contact.
- `lotse export acme/shop` writes that scope in the same format, so a scope
  can go UI → Git → `apply` and back.

### 9.3 Docker labels

```yaml
labels:
  lotse.env: "acme/shop/prod"          # put this container into that environment
  lotse.name: "shop-app"               # its slug, instead of compose project-service
  lotse.ignore: "true"                 # do not monitor it
  lotse.check.web.http.url: "https://shop.acme.de/health"
  lotse.check.web.http.keyword: "ok"
  lotse.check.db.tcp.address: "db.internal:5432"
  lotse.check.backup.heartbeat.period: "24h"
```

- The agent reports labels with the container inventory. The hub applies them
  like an `apply` limited to that container, and only when their hash changes,
  that is, on a new deploy. A UI edit of a label-made check stands until the
  next deploy with changed labels.
- Labels can set the container's environment, its name, `ignore`, its own
  checks and its own alert rules (`lotse.rules.container.…`, decision
  0019). They cannot touch channels, routing, secrets, other objects, or
  create scopes. `lotse.env` only reaches environments of the host's own
  customer, so no container can move itself, or its alerts, into another
  customer's environment. If it names an environment that does not exist or
  belongs to another customer, the container stays in its host's environment
  and shows a warning.
- Checks that disappear from the labels are deleted, but only the container's
  own.
- The brief's example used `lotse.project`. Since the target is an environment
  (`customer/project/environment`), the label is called `lotse.env`.

## 10. Updates and the starter

The image and the installed `/usr/local/bin/lotse` do not run the service
directly. They run `lotse start <mode>`, **the starter**:

1. Look in `/var/lib/lotse/bin/` for a verified newer version. If there is one,
   run it as a child process; otherwise run its own code.
2. Pass signals through. When the child exits with code 75 ("update ready"),
   start the new current version.
3. If a new version exits within 60 s of starting, 3 times in a row, mark it
   bad, go back to the previous one, and report the rollback in the next
   `hello`.

An update, for an agent or for the hub:

1. The hub sends `update {version}` (or, for itself, the admin clicks
   "Update" in the UI).
2. The process refuses anything that is not newer than itself or that is
   marked bad.
3. It downloads `lotse_<version>_linux_<arch>.tar.gz`, `checksums.txt` and
   `checksums.txt.minisig`. The source is always the GitHub release; only a
   local flag on the machine (`--release-url`, for mirrors) can change it,
   never the hub.
4. It checks the signature against the keys built into the binary, then the
   checksum.
5. It unpacks to `/var/lib/lotse/bin/<version>/` and points `current` at it.
   The hub first writes its SQLite backup.
6. It exits with code 75. The starter does the rest.

The hub learns about new releases from the GitHub API every 6 hours. It shows
them per host, and in the hub's own settings. Automatic agent updates follow
`agent.auto_update` and `agent.update_window`. Within an environment they
start with one host and continue after it has been healthy for 10 minutes.

The starter itself only changes when the install script runs again or a newer
image is pulled. Because it always runs the newest verified version it can
find, pulling an older image never downgrades anything.

## 11. Milestones

The milestones from the brief, with the changes the decisions make necessary
(marked ★):

1. **Skeleton.** `go.mod` (`lotse.sh`), the binary with its modes, the starter
   as entrypoint from the first image on ★ (so that early installs can update
   later), an empty UI embedded, `/api/health`, the setup token in the log ★,
   store with migrations for both databases ★, CI, release pipeline (GoReleaser,
   release-please, minisign, cosign), `compose.yml`, `site/` with the meta tag,
   the agent connecting with `hello`.
2. **Agent → hub.** Host and container metrics stored, simple graphs, live rate
   over the browser WebSocket ★, raw data deleted after 48 h ★ (downsampling
   comes with 9).
3. **Tree and onboarding.** Customers, projects, environments, settings with
   inheritance and the effective view, the setup wizard ★, enrollment tokens,
   the "add agent" one-liner, the install script, the change history (from here
   on every UI change is recorded) ★.
4. **Updates ★.** Release watcher, `update` for agents and the hub, rollback,
   auto-update with update windows. It comes this early so that the shadow
   operation on real machines can be updated from the first release on;
   the repository goes public with that release.
5. **Probes and checks.** HTTP, TLS, TCP, heartbeat; the built-in probe and
   external probes.
6. **Alerting.** Rule engine, channels, routing, maintenance, default rules,
   dead man's switch.
7. **apply, export, labels.** With dry-run diff and scoped prune.
8. **Read API.** API tokens, OpenAPI published, `lotse.sh/client`.
9. **Retention and downsampling.** 48 h raw, 30 d of 5-minute values, 1 year of
   hourly values; series read from the finest that reaches back far enough
   ★, `lotse backup` ★.

After each milestone `docker compose up` runs everything locally: hub, an agent
that watches the Docker host, and a few demo containers with labels.

## 12. Settled during the review

These followed from the decisions and were confirmed on 2026-10-13:

- A container is identified by compose project and service, so it keeps its
  history and checks across redeploys (5.1).
- `lotse.env` replaces `lotse.project` and only reaches environments of the
  host's own customer (9.3).
- API tokens come in two kinds: `read` for the read API, and `apply`, which
  may also write configuration but not manage users or tokens (5.3, 9.2).
- Enrollment tokens stay valid until revoked (7).
- Enrolling again with the same machine ID takes over the existing host (7).
- A check is down when the majority of its online probes see it fail (8.3).
- A list in the settings replaces the inherited list (5.2).
- Updates are milestone 4, right after the tree and onboarding (11).
- The repository stays private through milestone 3 and goes public with the
  first release, before hub and agent are tried on real machines. Install
  downloads, updates, GitHub Pages and `go install` all need it public.
