# 0018: The interface is as simple as Beszel's

Date: 2026-10-14

## Context

The first interface was a column of cards per page: it worked, but showed
how lotse is built rather than what is going on. A second one, close to
shadcn's defaults, looked like any generated dashboard. The interface is
meant to be what sets lotse apart, and its measure is the simplicity and
clarity of [Beszel](https://github.com/henrygd/beszel).

## Decision

- **Everything at once.** Home shows whether all is well, what needs
  attention, a table of every host with its load as bars, and a table of
  every check with its last day. A customer's and an environment's pages
  show the same, for theirs: on a customer's page the rows are grouped by
  project and environment.
- **A short sidebar.** Search (⌘K), home, alerts, the customers as one flat
  list, settings, the user. Projects and environments are not navigation;
  they are groups in the tables and pages of their own.
- **Pages of objects** have the same shape: where it is, how it stands, its
  name, a line of facts, its problems, and a few tabs, the first of which
  holds what matters (for a host: its graphs). What is seldom done, such
  as moving a host or deleting a check, is in a menu.
- **Alerts per object through a bell**: a switch per rule, a slider for its
  thresholds, buttons for how long it must hold. The inherited settings
  stay in the settings, for those who need them.
- **Words, not keys.** "Disk almost full", not `host.disk`; a rule's
  settings as "Critical at 95 %, from ACME".
- **Type and color.** Fira Sans and Fira Mono, 15 px for text. Status colors
  are saturated and mean only state, with a shape each (ring with a check,
  half-filled, filled with a mark, dashed), so that they do not rely on
  color. One indigo for the main action. Light and dark are both designed,
  and follow the system.

## Consequences

New features come with a place in this shape, or wait until they have one.
