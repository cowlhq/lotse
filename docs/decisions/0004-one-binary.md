# 0004: One binary with SQLite and the interface embedded

Date: 2026-10-25

Taken from lotse 0.1, decisions 0002 and 0004; without Postgres.

## Decision

- lotse is one Go binary. `lotse` runs the hub: the web server, the API,
  the checks and the alerts. Later commands (an agent, a backup) join the
  same binary.
- The hub keeps its data in SQLite, in WAL mode, with the pure-Go driver
  `modernc.org/sqlite`, so that the binary stays static without CGO. There
  is one first migration; later migrations only add to it.
- The interface is a single-page app: Vite, React, TypeScript, TanStack
  Router and Query, Tailwind. Its components are lotse's own, built to the
  prototype, with base-ui primitives where behaviour is hard (menus,
  dialogs, sheets). Its build is embedded with `go:embed`; no Node process
  runs in production. The build output is not committed.
- The API is JSON over HTTP, described in `api/openapi.yaml`, from which
  the Go handlers' types and the TypeScript client are generated. The
  browser gets live changes over server-sent events.
- Code is arranged by feature (`internal/check`, `internal/space`,
  `internal/alert`, `internal/people`), each with its storage, handlers and
  tests, rather than by kind of code.

## Why

- SQLite needs nothing set up and is plenty for thousands of checks.
  Postgres can come back when someone needs it; supporting both from the
  start doubled every storage test in lotse 0.1.
- Server-sent events are enough for a page that only listens; the WebSocket
  of lotse 0.1 existed for agents, which come later.
