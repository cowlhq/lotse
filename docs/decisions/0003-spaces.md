# 0003: Spaces, optional, one level of sub-spaces

Date: 2026-10-25

Replaces the customer, project and environment scopes of lotse 0.1.

## Decision

- A hub holds checks. A check may belong to a **space**: a group of checks
  with something in common, such as a place (Home, Parents), a club or a
  customer. Checks that belong to none are the hub's own.
- A space may hold **sub-spaces**, one level deep and no deeper. A
  sub-space holds checks, as a space does. Deleting a sub-space moves its
  checks to its space; deleting a space with checks or sub-spaces is
  refused until they are moved or deleted.
- Spaces are optional. A hub without any shows one list of checks and says
  nothing about spaces until one is made.
- **Settings inherit** along hub, space, sub-space, check. Each level uses
  its parent's setting or has its own. Who is told about a check, how often
  it runs and what counts as slow are such settings.
- There are no labels, environments or other ways of grouping. Tags may
  come later, as a filter only.

## Why

- Most people who run lotse have a handful of checks and no customers. They
  should never meet levels they did not ask for.
- Those who look after others need to group by customer, send a
  customer's alerts to the customer and let the customer look in. A space
  does all three, and a sub-space covers a customer's projects.
- One level of sub-spaces is enough for both; a tree of any depth would
  make every page, menu and setting harder to read.
