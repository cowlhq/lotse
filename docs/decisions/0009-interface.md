# 0009: The interface is as simple as Beszel's

Date: 2026-10-25

Taken from lotse 0.1, decision 0018, with the navigation of spaces.

## Context

lotse 0.1 tried a column of cards per page, then something close to
shadcn's defaults, then a sidebar of customers. The first showed how lotse
is built rather than what is going on, the second looked like any
generated dashboard, the third assumed customers. The interface is meant to
be what sets lotse apart, and its measure is the simplicity of
[Beszel](https://github.com/henrygd/beszel).

## Decision

- **A top bar, no sidebar**: the logo, the breadcrumb, search (⌘K),
  alerts and the user.
- **The breadcrumb is the navigation.** Every step is a menu of what sits
  beside it, the current item in bold. A page that has children ends in a
  step "Overview", which offers them: sub-spaces with their number of
  checks, then checks with their kind. So the home page shows `Overview ▾`,
  a space `Home ▾ / Overview ▾`, a sub-space `Home ▾ / Media ▾ /
  Overview ▾`, a check `Home ▾ / Media ▾ / Nextcloud ▾`. On a phone only
  the last step shows, and the one before "Overview".
- **Home says whether all is well** in one sentence, then what needs
  attention, then the spaces with their last 30 days and the checks in no
  space. Without spaces it is one list of checks.
- **A space's page** shows its own checks first, then a quiet heading per
  sub-space over its checks.
- **A check's page** shows where it is, its name, a line of facts, what is
  wrong if anything, its last 30 days, one chart and what happened.
- **Settings open at the side**, in a sheet, so that the page stays in
  view.
- **Adding a check is one field.** `https://…` is a website, `host:port` a
  port, a bare domain a website; cron and certificate are one click away.
- **Words, not keys**: "Tell me when it is down", not `check.down`.
- **Type and colour.** Fira Sans and Fira Mono, 15 px for text. Status
  colours are saturated, mean only state, and come with a shape each, so
  that they do not rely on colour; a status dot breathes once every 5
  seconds. One indigo for actions. Light by default, dark designed as well
  and following the system. The red of the logo stays in the logo, since
  red means down.

## Consequences

A new feature comes with a place in this shape, or waits until it has one.
