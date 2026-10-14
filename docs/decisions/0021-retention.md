# 0021: Readings are averaged before they are deleted

Date: 2026-10-14

## Decision

- Readings are kept in three resolutions: as they came, as 5-minute steps
  and as hourly steps, each with the average, the maximum and the number of
  readings. The defaults are 2 days, 30 days and a year; they are set for
  the whole hub only (`retention.raw`, `retention.5m`, `retention.1h`).
- The hub makes the steps every 10 minutes, of readings older than 20
  minutes, since an agent sends what it kept while the hub was away up to 15
  minutes late. A step that is made is not made again: readings that come
  later than that are in the raw table only.
- Nothing raw is deleted that is not averaged yet. Deleting goes object by
  object, so that no statement holds SQLite for long.
- A series is read from the finest resolution that still holds the range's
  beginning, judged by what is there, not by the settings; what is newer
  than its steps comes from the finer ones, up to now. Charts offer ranges
  up to a year.
- Events and alerts are kept as long as the hourly steps; the change
  history is kept.
- `lotse backup FILE` copies a SQLite hub's database while it runs, with
  `VACUUM INTO`, without changing its schema. Postgres is backed up with
  its own tools.

## Consequences

A year of a host is about 8,800 hourly rows instead of two million
readings. Maxima stay
visible in the steps, so a short peak is not averaged away in the data,
though the charts show averages.
