# 0008: 15 s hosts, 30 s containers, 3 s live

Date: 2026-10-13

## Decision

- Host metrics every 15 s, container metrics every 30 s (`docker stats` costs
  a call per container). Both are settings and inherit like any other.
- While someone looks at a host, its agent reports every 3 s. The hub tells it
  to; this changes a rate and runs nothing.
- While the hub is unreachable, the agent keeps about 15 minutes of samples in
  memory and sends them after reconnecting. It writes nothing to disk.
