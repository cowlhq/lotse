# 0001: AGPL-3.0

Date: 2026-10-25

Taken from lotse 0.1, decision 0001.

## Context

lotse is open source. Its author may offer it as a hosted service later and
does not want others to run a competing service on modified code without
sharing the changes.

## Decision

- Everything is licensed under AGPL-3.0. A Go client for other programs,
  when there is one, gets its own directory and Apache-2.0, so that they can
  use it without taking on the AGPL.
- External contributions need a signed CLA, so the author can keep offering
  lotse under other terms (for example a hosted service with closed
  additions). The CLA is set up before the first external pull request.
- Code from MIT and Apache-2.0 projects may be used, keeping its copyright,
  its license and, for Apache-2.0, a NOTICE entry and a note of what
  changed. No code from GPL projects.

## Consequences

- Some companies avoid AGPL software. Running lotse unmodified is
  unaffected; only offering a modified lotse over a network requires
  publishing the changes.
- The Functional Source License would protect a hosted offer more strongly,
  but is not open source. Apache-2.0 alone would not protect it at all.
