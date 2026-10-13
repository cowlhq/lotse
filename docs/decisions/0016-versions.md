# 0016: Until 1.0, releases count up the patch version

Date: 2026-10-13

## Decision

Until lotse reaches 1.0 (the MVP), a release with new features or fixes
counts up the last number: 0.1.0, 0.1.1, 0.1.2, … The middle number goes up
only for commits marked as breaking (`feat!:` or a `BREAKING CHANGE:`
footer). This is how cowl counts.

release-please does this with `bump-minor-pre-major` and
`bump-patch-for-minor-pre-major`.

## Consequences

A change that breaks agents or the API has to be marked as breaking in its
commit, or the version will not show it.
