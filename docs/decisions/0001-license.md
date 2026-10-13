# 0001: AGPL-3.0, the Go client Apache-2.0

Date: 2026-10-13

## Context

lotse is open source. Its author may offer it as a hosted service later and
does not want others to run a competing service on modified code without
sharing the changes. cowl and other programs must be able to use the Go client
without taking on the AGPL.

## Decision

- Everything is licensed under AGPL-3.0, except `client/`, which is
  Apache-2.0 and carries its own `LICENSE`.
- External contributions need a signed CLA, so the author can keep offering
  lotse under other terms (for example a hosted service with closed
  additions). The CLA is set up before the first external pull request.
- Code from MIT and Apache-2.0 projects may be used (it is compatible), keeping
  its copyright, license and, for Apache-2.0, a NOTICE entry and a note of what
  changed. No code from GPL projects.

## Consequences

- Some companies avoid AGPL software. Running lotse unmodified is unaffected;
  only offering a modified lotse over a network requires publishing the changes.
- The Functional Source License would protect a hosted offer more strongly, but
  is not open source. Apache-2.0 alone would not protect it at all.
