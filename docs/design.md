# lotse — design

How lotse is meant to work and why. The settled points are also in
[decisions](decisions/); where the two disagree, the decision holds.

## What lotse is

lotse watches websites, ports, certificates and cron jobs, and tells you when
something is wrong. One small program you run yourself, one page that says
whether all is well.

Its measure is the simplicity of Beszel and Uptime Kuma: installed in a
minute, understood without reading anything, quiet while all is well.

## Who it is for

- **Most people run it for themselves** (about four in five): a homelab, a
  blog, the parents' router, the sports club's website. They have a handful
  of checks and no customers. For them lotse must never show levels,
  customers or projects they did not ask for.
- **Some run it for others**: a freelancer or a small agency with a few
  dozen customers. They need to group by customer, send a customer's alerts
  to that customer, and let the customer look in.

Both use the same model. The second group switches on what the first never
sees.

## Scope of the first version

In:

- **Checks** of four kinds:
  - *Website* (HTTP/S): status, response time, optionally a word in the page.
  - *Port* (TCP): opens or not, how fast.
  - *Certificate*: days until it expires, whether it is valid for the name.
  - *Cron* (heartbeat): a URL the job calls; silence past its grace is an
    alert.
- **Spaces**, optional, with one optional level of sub-spaces.
- **Alerts** by email, ntfy and webhook, with "tell me when" per check.
- **People**: an owner, and people invited to the whole hub or to one or
  more spaces.
- **History**: a 30-day strip per check, response times, what happened.

Out, for now (each comes later as its own feature):

- The agent and server monitoring (CPU, disks, containers).
- External probes (checks run from the hub only).
- DNS and domain-expiry checks.
- Further chat and push channels (Slack, Telegram …). Webhooks cover them
  until then.
- Configuration as code, API tokens, the public read API.
- Status pages, maintenance windows, acknowledging.
- Over-the-air updates.

## The model

```
Hub
├── Check                      (not in a space)
├── Space
│   ├── Check
│   └── Sub-space
│       └── Check
└── Space
    └── Check
```

- **Hub**: one installation. It holds the settings everything else
  inherits: who is told, how often a check runs, what counts as slow.
- **Space**: a group of checks with something in common: a place (Home,
  Parents), a club, a customer. A space may say who is told about its
  checks and who may look at it. Spaces are optional: a hub without any
  shows a plain list of checks, and nothing about spaces until one is made.
- **Sub-space**: a space inside a space, one level deep and no deeper
  (Home › Network, Bäckerei Sommer › Shop). It is a quieter grouping: on
  its space's page it is a heading over its checks, not a card of its own.
  Deleting it moves its checks to its space.
- **Check**: belongs to the hub, a space or a sub-space. Moving it is a
  select in its settings.

**Settings inherit** along hub → space → sub-space → check. A level either
uses its parent's setting ("Like Home") or has its own. There is no other
mechanism: no labels, no environments, no rules attached to tags.

**Why not customer → project → environment** (decision 0003), as lotse 0.1 had: it was right
for agencies and wrong for everyone else, and it was hard-wired into about
twenty places. Spaces carry the same meaning when someone wants it ("Space"
= customer, "Sub-space" = project) and stay out of sight when not.

Tags may come later, as a filter only, never as structure.

## The interface

Designed in a clickable prototype first (decision 0009); the points that
matter:

- **Look**: Fira Sans and Fira Mono, 15 px, one indigo for actions, light by
  default and a designed dark mode. Status is a coloured dot with a soft
  halo every 5 seconds; a shape per state so colour is not needed to read
  it. Uptime as even blocks, one per day.
- **Top bar**: logo, breadcrumb, search (⌘K), alerts, user.
- **Breadcrumb**: every step is a menu of what sits beside it, with the
  current item in bold. On a page that has children, a last step
  "Overview ▾" offers what can be chosen next: the sub-spaces with their
  counts, then the checks with their kind. The same rule at every level:
  `Overview ▾` on the home page, `Home ▾ / Overview ▾`,
  `Home ▾ / Media ▾ / Overview ▾`, `Home ▾ / Media ▾ / Nextcloud ▾`.
  On a phone only the last step (and the one before "Overview") shows.
- **Home**: one sentence on how things stand, then what needs attention,
  then the spaces with a 30-day strip and the checks not in a space. Without
  spaces: one list of checks. Empty: a welcome with one button.
- **Space page**: its own checks first, then a quiet heading per sub-space
  with its checks.
- **Check page**: where it is, its name, a line of facts, a banner if
  something is wrong, the 30-day strip, a chart (response time, days left,
  or pings), and what happened.
- **Settings at the side**: a check's and a space's settings open in a sheet
  from the right, so the page stays in view.
- **Adding a check is one field**: paste `https://…` and it is a website,
  `host:port` a port, a bare domain a website. Cron and certificate are one
  click away. The space select only shows when spaces exist.
- **Words, not keys**: "Tell me when it is down", not `check.down`.

## Alerts

Decision 0011:

- Channels: email (through the hub's SMTP server), ntfy (a topic on
  ntfy.sh or the owner's own server, optionally with a token) and webhook
  (documented JSON, optional HMAC signature).
- A check alerts after two failed runs in a row, and again when it is back.
- Alerts are gathered for 30 seconds per channel and sent as one message.
- Notifications are rows in the database; a restart loses none. Sending is
  retried with backoff.
- Who is told is a setting that inherits like any other.

## People

- The first person, created at setup with a one-time token from the log
  (decision 0005), owns the hub (decision 0010).
- Others are invited by email link. A person has one or more accesses; an
  access is the whole hub or one space (with its sub-spaces), each with its
  own role: *admin* (may change things there) or *viewer* (only looks). So
  someone can be admin of "Sports club" and viewer of "Parents".
- Someone without access to the whole hub sees only their spaces: the home
  page, the breadcrumb, search and alerts show nothing else, not even that
  other spaces exist. Checks not in a space belong to the hub and are only
  seen with access to the whole hub.
- Sign-in with password; passkeys later.

## The foundation

- **One Go binary**: hub, check runner, web server and the embedded UI.
  `lotse hub` runs the hub; later `lotse agent` joins it.
- **SQLite or Postgres** (decision 0014): SQLite by default, in WAL mode
  with `modernc.org/sqlite`, no CGO; Postgres when `LOTSE_DATABASE_URL`
  names one. Every migration and every storage test exists for both.
- **Web**: React, Vite, TypeScript, TanStack Router and Query, Tailwind.
  Components are our own, built to the prototype; base-ui primitives where
  behaviour is hard (menus, dialogs, sheets). Texts in translation files.
- **API**: JSON over HTTP, described in `api/openapi.yaml`, code generated
  for Go and TypeScript. Live updates to the browser over server-sent
  events.
- **Code by feature**: `internal/check`, `internal/space`, `internal/alert`,
  `internal/people`, each with its store, its handlers and its tests,
  rather than one layer per kind of code.
- **Readings** are kept as they came for 2 days, then as 5-minute and hourly
  averages (decision 0012).
- **Install**: a container image and a compose file; plain HTTP behind the
  owner's reverse proxy, ACME optional (decision 0006). Linux amd64 and
  arm64 (decision 0013).
- **Tooling**: mise pins tools and holds the tasks (`mise run dev`,
  `mise run check`); CI runs `mise run check`; release-please and
  GoReleaser; minisign-signed checksums.

## Working on it

- English in code, comments, commits and docs.
- Conventional commits, no trailers.
- Small pull requests, rebase-merged; every commit builds and passes.
- A feature is designed in the prototype first and becomes a pull request
  once it is settled, so the history shows each feature arriving once.
- Decisions live in `docs/decisions/`; one changes only by a new record.
- Versions start at 0.2.0, so that agents and hubs of lotse 0.1 do not take
  the new releases for older ones.

## Order of work

1. Skeleton: repository, mise, CI, the binary serving an empty page, the
   first migration, setup with the one-time token, sign-in.
2. The shell: top bar, breadcrumb, home page, empty state, light and dark.
3. Website checks: add, run, list, check page with history.
4. Alerts by email, ntfy and webhook.
5. Port, certificate and cron checks.
6. Spaces and sub-spaces, with inherited settings.
7. People and invitations.
8. Release 0.2.0.

## What comes from lotse 0.1

The old repository stays online, archived, as `cowlhq/lotse-v0`; the new
one takes the name `cowlhq/lotse` (decision 0002). Taken over, rewritten
where needed and not copied wholesale:

- The check runners for HTTP, TLS and TCP (`internal/probe`).
- Email and webhook sending, batching and retries (`internal/notify`,
  `internal/alert`).
- Setup token, retention, release signing and the install script.
- Decision records that still hold, carried over as new records that say
  where they came from. Those about the website, release signing, updates
  and check secrets come back with their features.

Not taken over: the customer, project and environment scopes, labels,
enrollment, the agent and its protocol, configuration as code.
Its access model is kept, with spaces in place of scopes (decision 0010).
