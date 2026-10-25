# 0014: Postgres beside SQLite

Date: 2026-10-25

Replaces the point of decision 0004 that kept the hub to SQLite; the rest
of 0004 holds.

## Decision

- The hub keeps its data in SQLite or in Postgres, both from the start.
- SQLite is the default and needs nothing: the hub makes `lotse.db` in its
  data directory. With `LOTSE_DATABASE_URL` set to a `postgres://` URL, the
  hub uses that database instead. There is no choice to make in setup.
- SQL is written by hand, to what both databases understand, with `?`
  placeholders that become `$1, $2, …` for Postgres. Times are Unix
  seconds; JSON is stored as text and not queried inside.
- Every migration exists for both, with the same number and name. Every
  test that touches the database runs against both; locally Postgres comes
  from `mise run dev:db`, and without it those runs are skipped. In CI they
  must pass.

## Why

Some who run lotse already run Postgres, back it up and watch it, and
would rather not keep a second database beside it. Adding Postgres later
would mean finding every query that only SQLite understands; keeping both
from the first query costs little.

## Consequences

Each storage test runs twice. A feature that wants something only one
database has, such as SQLite's `VACUUM INTO` for a backup, offers it for
that database only and says so.
