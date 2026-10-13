# 0004: SQLite and Postgres, chosen at setup

Date: 2026-10-13

## Decision

- The hub supports SQLite and Postgres from the start.
- SQLite runs in WAL mode with the pure-Go driver `modernc.org/sqlite`, so the
  binary stays static without CGO.
- The database is chosen in the setup wizard. SQLite is preselected and needs
  no input. Postgres takes a URL, which the wizard tests. The shipped compose
  file has an optional `postgres` profile; the wizard finds a reachable
  `postgres:5432` and fills it in. An external Postgres works the same way.
- `LOTSE_DATABASE_URL` skips that step, for provisioning and hosted use.
- After setup the database is fixed. Moving between them later is a CLI
  command (`lotse db migrate`), not part of the MVP.

## Consequences

- Every schema change is written for both databases, and the whole store test
  suite runs against both in CI.
- The SQL stays within what both understand (see the design, "SQLite and
  Postgres").
- With SQLite, a backup is a copy of one file. With Postgres, backups are the
  operator's job.
