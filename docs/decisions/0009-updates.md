# 0009: Over-the-air updates for hub and agents

Date: 2026-10-13

## Decision

- The hub (in compose), agents installed as a systemd service, and agents
  running as a container can be updated over the air. External probes cannot;
  they are updated by hand.
- The image (or the installed binary) is only a starter. It runs the newest
  verified binary from the data directory. After an update the process exits,
  and the starter runs the new one. If the new one does not come up, the
  starter goes back to the previous one.
- Updates are triggered by hand in the UI (one agent, an environment, or all)
  or automatically: "auto-update" is a setting that is inherited down the
  hierarchy and respects its update window. The hub itself is only updated by
  hand.

## Security

This is the one exception to "agents run no commands": the hub can ask for
"update to version X", nothing else. The agent downloads X itself from the
official release location, checks its signature against a key built into the
binary (0010), and refuses older versions. A compromised hub can therefore
only install official releases.

## Consequences

- No container needs write access to the Docker socket.
- The image tag and the running version can differ. The UI always shows the
  running version.
