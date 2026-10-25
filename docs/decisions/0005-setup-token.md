# 0005: Setup needs a one-time token from the log

Date: 2026-10-25

Taken from lotse 0.1, decision 0005.

## Decision

A hub without an owner starts in setup mode and writes a setup URL with a
one-time token to its log (`docker compose logs lotse`). Setup does nothing
without the token. The token is gone once setup is done.

## Why

A hub is often reachable from the internet before anyone has set it up.
Whoever got there first would own it.
