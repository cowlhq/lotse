# 0019: Configuration as code

Date: 2026-10-14

## Decision

- **API tokens** (`lotse_tok_…`) are made in Settings → API tokens, for
  everything or for one customer, project or environment and what is in
  it. A token is for applying and exporting, or only for reading (the read
  API, milestone 8). The hub keeps a hash. A token limited to ACME cannot
  change, export or prune anything outside ACME, nor the global settings.
- **Secrets in lotse.yaml** are never values:
  - `{env: NAME}` and `{file: /run/secrets/name}` are references the hub
    resolves when it sends, as in the UI (decision 0017).
  - `{from_env: NAME}` is read by `lotse apply` on the machine it runs on,
    such as a CI job, and sent to the hub, which keeps it sealed.
  - Export writes references as they are and a sealed secret as
    `{secret: CHANNEL}`, which apply leaves as it is.
- **What a file holds**: customers, projects, environments, their settings
  and channels; checks; hosts and their containers' settings; and
  maintenance that comes back (`sun 02:00-04:00`). One-off maintenance
  windows stay in the UI: they are moments, not configuration.
- **Docker labels** may set a container's environment, name, `ignore`, its
  own checks, and its own alert rules (`lotse.rules.container.…`), and
  nothing else: no routing, channels, secrets or other objects.
- Apply happens in **one transaction**: all of it, or none. A dry run takes
  the same way and rolls back, so it shows exactly what applying would do.

## Consequences

A pipeline can hold one customer's configuration and its own token, and
cannot reach another customer's. A value set in the UI and later applied
from a file is changed by whoever applies last; the dry run says who
changed it before.
