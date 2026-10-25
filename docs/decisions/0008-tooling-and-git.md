# 0008: mise, release-please, GoReleaser; rebase-merged pull requests

Date: 2026-10-25

Taken from lotse 0.1, decision 0015.

## Decision

- mise pins the tools and holds the tasks (`mise run dev`; `mise run
  check` runs everything CI runs). release-please keeps a release pull
  request with the changelog; merging it tags the version. On that tag,
  GoReleaser builds binaries, images and checksums.
- `main` is protected and needs green CI. Work arrives in small pull
  requests and is rebase-merged, so that `main` has a linear history of the
  original commits.
- Every commit is a conventional commit, builds, and passes the tests. No
  trailers and no mention of tools in commit messages.
- Releases are SemVer tags, counted as decision 0002 says.
