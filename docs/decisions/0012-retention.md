# 0012: Readings are averaged before they are deleted

Date: 2026-10-25

Taken from lotse 0.1, decision 0021, without agents and Postgres.

## Decision

- A check's readings are kept in three resolutions: as they came, as
  5-minute steps and as hourly steps, each with the average, the maximum
  and the number of readings and failures. The defaults are 2 days, 30
  days and a year, set for the whole hub only.
- The hub makes the steps every 10 minutes. Nothing raw is deleted that is
  not averaged yet, and deleting goes check by check, so that no statement
  holds SQLite for long.
- A series is read from the finest resolution that still holds the range's
  beginning; what is newer than its steps comes from the finer ones.
- Events are kept as long as the hourly steps.

## Consequences

A year of a check every minute is about 8,800 hourly rows instead of half
a million readings. Maxima stay in the steps, so a short spike is not
averaged away.
