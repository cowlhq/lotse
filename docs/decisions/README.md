# Decisions

One file per decision, numbered in the order they were made. A decision is
changed by a new record that says which one it replaces, never by editing the
old one.

| # | Decision | Date |
|---|---|---|
| [0001](0001-license.md) | AGPL-3.0, the Go client Apache-2.0 | 2026-10-13 |
| [0002](0002-frontend.md) | React + Vite SPA, embedded in the binary | 2026-10-13 |
| [0003](0003-websocket.md) | WebSocket for agents, probes and the browser | 2026-10-13 |
| [0004](0004-database.md) | SQLite and Postgres, chosen at setup | 2026-10-13 |
| [0005](0005-setup-token.md) | Setup needs a one-time token from the log | 2026-10-13 |
| [0006](0006-tls.md) | Plain HTTP behind a reverse proxy, ACME optional | 2026-10-13 |
| [0007](0007-agent-credentials.md) | Agents authenticate with a bearer token | 2026-10-13 |
| [0008](0008-metric-intervals.md) | 15 s hosts, 30 s containers, 3 s live | 2026-10-13 |
| [0009](0009-updates.md) | Over-the-air updates for hub and agents | 2026-10-13 |
| [0010](0010-release-signing.md) | minisign for binaries, cosign for images | 2026-10-13 |
| [0011](0011-hub-install.md) | The hub runs with docker compose, installed by script | 2026-10-13 |
| [0012](0012-lotse-sh.md) | `lotse.sh` as module path, served by GitHub Pages | 2026-10-13 |
| [0013](0013-platforms.md) | Linux amd64 and arm64 only | 2026-10-13 |
| [0014](0014-language.md) | English everywhere, UI ready for translation | 2026-10-13 |
| [0015](0015-tooling-and-git.md) | mise, release-please, GoReleaser; rebase-merged PRs | 2026-10-13 |
| [0016](0016-versions.md) | Until 1.0, releases count up the patch version | 2026-10-13 |
| [0017](0017-alerting.md) | Alerts: a mail server per hub or channel, gathered, acknowledged | 2026-10-14 |
