# 0012: `lotse.sh` as module path, served by GitHub Pages

Date: 2026-10-13

## Decision

- The Go module path is `lotse.sh`, so the repository can move without
  breaking anyone's imports (`import "lotse.sh/client"`).
- `lotse.sh` serves the `go-import` meta tag on every path and the install
  script at `/install`. Nothing else runs there; the hub runs wherever its
  owner puts it.
- The site is the `site/` directory of this repository, published by GitHub
  Pages from a workflow. DNS stays at Cloudflare, with one CNAME record.

## Why

- Whoever controls `lotse.sh` controls every `curl | sh` install. GitHub Pages
  keeps the site in the same place, and under the same protection, as the code
  and the releases. Changes to the install script go through pull requests.
- No server to run, and no other account to secure.
- If `lotse.sh` is down, `go install` mostly still works through
  proxy.golang.org; only `/install` is gone.
