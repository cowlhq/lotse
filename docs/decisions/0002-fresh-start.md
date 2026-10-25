# 0002: lotse starts over, at 0.2.0

Date: 2026-10-25

## Context

lotse 0.1 grew to 36,000 lines of Go in a few days, around a model of
customers, projects and environments. That model is right for agencies and
wrong for the people most likely to run lotse: someone with a homelab, a
blog and their parents' router, who has no customers. It was wired into
about twenty places. Changing it in place would have dragged the old model
through every later commit.

## Decision

- lotse starts over in a new repository, `cowlhq/lotse`. The old one is
  renamed `cowlhq/lotse-v0` and archived; it stays online for reference.
- Code from lotse 0.1 is taken over where it still fits, rewritten to the
  new model rather than copied wholesale. Its decisions that still hold are
  carried over as new records, each saying where it came from.
- The first version watches checks only (websites, ports, certificates,
  cron jobs). The agent and server monitoring come later.
- Releases start at 0.2.0, so that nothing of 0.1 takes them for older
  versions. Until 1.0 a release counts up the last number (0.2.0, 0.2.1,
  …); the middle one goes up only for commits marked as breaking (`feat!:`
  or a `BREAKING CHANGE:` footer). release-please does this with
  `bump-minor-pre-major` and `bump-patch-for-minor-pre-major`.
- No one runs lotse 0.1 that we know of, so there is no upgrade path from
  it.

## Consequences

A feature is designed in a prototype until it is settled, and then arrives
in the repository once, so that the history shows how lotse is built
rather than how it was found.
