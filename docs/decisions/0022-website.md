# 0022: lotse.sh presents lotse and carries its documentation

Date: 2026-10-16

Replaces [0012](0012-lotse-sh.md), which kept lotse.sh to the `go-import`
tag and the install script.

## Decision

- `lotse.sh` is lotse's website: a landing page that says what lotse is and
  shows it, and the documentation. It still serves the `go-import` tag on
  every page and the install script at `/install`; the module path stays
  `lotse.sh`.
- The landing page and the pages behind Go import paths are written by hand,
  in HTML and one stylesheet, in `site/`. No JavaScript, no framework. Fira
  Sans and Fira Mono are served from the site itself.
- The documentation stays Markdown in `docs/`, its one source, readable on
  GitHub as before. `internal/sitegen` (Go, with goldmark) renders it into
  pages beside `site/` when the site is published: the guides, the design,
  and every decision. Links between the files become links between the
  pages; links to anything else in the repository go to GitHub.
- The screenshots on the landing page come from a hub with made-up
  customers, machines and checks, never from real data.
- GitHub Pages publishes it, as before, whenever `site/`, `docs/` or the
  generator change on `main`.

## Why

- The interface is what sets lotse apart, and a placeholder showed none of
  it. People deciding whether to try lotse look for a page, not a README.
- Generating the documentation from the Markdown keeps one source for it,
  without a Node build or a static site generator to keep up to date: the
  generator is a small Go program, tested with the rest.
- Unchanged from 0012: the site lives with the code, under the same
  protection, so whoever can change the install script went through a pull
  request.
