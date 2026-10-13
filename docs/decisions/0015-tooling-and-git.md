# 0015: mise, release-please, GoReleaser; rebase-merged PRs

Date: 2026-10-13

## Decision

- Like cowl: mise pins the tools and holds the tasks (`mise run dev`,
  `mise run check` runs everything CI runs). release-please keeps a release
  pull request with the changelog; merging it tags the version. On that tag,
  GoReleaser builds binaries, images, checksums and signatures.
- `main` is protected and needs green CI. Work arrives in small pull requests,
  several per milestone, and is rebase-merged, so `main` has a linear history
  of the original commits.
- Every commit is a conventional commit, builds, and passes the tests. No
  trailers and no mention of tools in commit messages.
- Releases are SemVer tags.
