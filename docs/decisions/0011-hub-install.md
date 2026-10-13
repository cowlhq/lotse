# 0011: The hub runs with docker compose, installed by script

Date: 2026-10-13

## Decision

The hub runs as a docker compose stack, for its author on a Hetzner VM.
As with cowl, one line installs it:

    curl -fsSL https://lotse.sh/install | sudo sh -s -- hub

The script installs Docker if it is missing (after asking), writes
`/opt/lotse/compose.yml`, starts the hub and prints its address and the setup
URL (0005). Running it again updates the starter image; the data stays. The
compose file is also documented for anyone who prefers to write it by hand.
