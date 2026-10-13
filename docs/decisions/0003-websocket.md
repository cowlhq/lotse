# 0003: WebSocket for agents, probes and the browser

Date: 2026-10-13

## Decision

- Agents and probes open one outbound WebSocket over HTTPS to the hub and keep
  it open. They listen on no port. There is no SSH and no connection from the
  hub to an agent.
- The browser gets live data (states, graphs, alerts) over a WebSocket too: one
  connection per tab, with subscribe and unsubscribe messages as the user
  navigates.

## Why

- A dropped connection tells the hub at once that an agent is gone; a heartbeat
  catches connections that hang.
- The hub can switch an agent to live rate (0008) or ask it to update (0009)
  without the agent polling.
- Outbound HTTPS passes firewalls and reverse proxies. Netdata Cloud, Datadog
  and Elastic Fleet also connect from the agent outward.
- One library and one way of thinking for both directions.

## Consequences

- Reconnecting with backoff and jitter is normal operation, not an error.
- The browser socket authenticates with the session cookie and checks the
  `Origin` header.
