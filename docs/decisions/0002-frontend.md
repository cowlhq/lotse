# 0002: React + Vite SPA, embedded in the binary

Date: 2026-10-13

## Decision

The UI is a static single-page app: Vite, React, TypeScript, TanStack Router and
Query, Tailwind and shadcn/ui, charts with uPlot. Its build is embedded in the
`lotse` binary with `go:embed`. No Node process runs in production.

The build output is not committed. A binary built with `go install` therefore
has no UI: it works fully as agent, probe and CLI, and as hub it shows a page
that points to the release binaries and the image.

## Why not Next.js

Next.js earns its weight with server rendering, server components and API
routes, which all need a Node server next to the hub. Its static export keeps
none of that and handles dynamic routes such as `/hosts/:id` poorly. A second
process would break "one binary" and complicate updates (0009).

## Consequences

- Node and pnpm are needed to build, not to run.
- Every update of the binary updates the UI with it.
