# Installing lotse

lotse is one binary in three roles: the **hub** (web interface, API,
storage, alerts), the **agent** on every machine it watches, and the
**probe**, which checks from outside. This page installs the first two.

## The hub

On a Linux machine with Docker, or one that may get it:

```
curl -fsSL https://lotse.sh/install | sudo sh -s -- hub
```

It writes `/opt/lotse/compose.yml` (the same as [deploy/compose.yml](../deploy/compose.yml)),
starts the hub on port 8080 (`--port` changes that) and prints the setup
link. The link holds a one-time token; it is also in
`docker compose -f /opt/lotse/compose.yml logs hub`. Setup asks for:

1. **The database.** SQLite needs nothing and keeps everything in one file in
   the hub's volume. Postgres takes a URL. Set `LOTSE_DATABASE_URL` to skip
   this step.
2. **The admin**, and the address under which people and agents reach the
   hub.

Running the command again pulls the newest image and restarts the hub; its
data stays in the volume `lotse_data`.

**Behind a reverse proxy** (recommended, decision 0006): bind the port to
`127.0.0.1:8080` in `compose.yml`, let Traefik or Caddy terminate TLS and
forward to it, and set the hub's address in setup to the `https://` one.

**Backups**: with SQLite, everything is in the volume `lotse_data`: stop the
hub for a moment and copy it. `lotse backup` will take consistent copies
while it runs, from milestone 9 on. With Postgres, back it up as you back up
Postgres; keep `secret.key` from the volume either way, since it unseals
what the hub keeps sealed.

## Agents

Every environment has an **Add agent** button. It shows the command with the
environment's enrollment token in it, for one of three ways:

**systemd**

```
curl -fsSL https://lotse.sh/install | sudo sh -s -- agent --hub https://lotse.example.com --token lotse_enr_…
```

This installs `/usr/local/bin/lotse`, creates the user `lotse` (in group
`docker` if Docker is there), enrolls once, and starts the service
`lotse-agent`. The service may write only to `/var/lib/lotse`, where the
agent keeps its own credential (`agent.json`); the enrollment token is not
kept anywhere. Running the command again updates the binary; it does not
enroll again.

**docker run** and **compose**: the same agent as a container. It needs the
machine's `/` mounted read-only at `/host` and the Docker socket; the command
in the UI has both.

### A Docker socket proxy

The agent only reads from Docker: the version, the list of containers, each
container's details and usage, and the event stream. But a Docker socket,
even mounted read-only, lets whoever holds it use the whole Docker API, and
that is root on the machine. To give the agent only what it reads, put
[docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy)
in between:

```yaml
services:
  docker-proxy:
    image: tecnativa/docker-socket-proxy
    restart: unless-stopped
    environment:
      CONTAINERS: 1   # list and inspect containers, read their stats
      EVENTS: 1
      VERSION: 1
      # everything else, and every POST, stays refused
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro

  lotse-agent:
    image: ghcr.io/cowlhq/lotse
    command: ["start", "agent"]
    restart: unless-stopped
    environment:
      LOTSE_HUB: https://lotse.example.com
      LOTSE_TOKEN: lotse_enr_…
      LOTSE_HOST_ROOT: /host
      DOCKER_HOST: tcp://docker-proxy:2375
    volumes:
      - /:/host:ro,rslave
      - lotse-agent:/var/lib/lotse

volumes:
  lotse-agent:
```

The agent no longer needs to run as root then.

### Revoking an agent

**Revoke agent** on a host disconnects it and revokes its credential. It
does not come back by itself. To bring it back, run the install command
again on the machine, with a current token: it notices the revoked
credential and enrolls again, as the same host. For a container agent, run
`lotse agent enroll` in it with `LOTSE_TOKEN` set, or delete the volume
`lotse-agent` and start it again.

### Removing the agent

```
sudo systemctl disable --now lotse-agent
sudo rm /etc/systemd/system/lotse-agent.service /usr/local/bin/lotse
sudo rm -r /var/lib/lotse && sudo userdel lotse
```

## Probes

Checks (HTTP, TLS, TCP) run on the hub's own probe. A probe in another
place checks from outside as well, so that the hub's network or a down
machine does not decide alone: a check is down when most of the probes that
run it see it fail. **Settings → Probes → Add probe** shows the command:

```
docker run -d --name lotse-probe --restart unless-stopped \
  -e LOTSE_HUB=https://lotse.example.com \
  -e LOTSE_TOKEN=lotse_enr_… \
  -e LOTSE_PROBE_NAME=fra -e LOTSE_PROBE_LOCATION=Frankfurt \
  -v lotse-probe:/var/lib/lotse \
  ghcr.io/cowlhq/lotse probe
```

A probe needs no ports and only reaches out to the hub. Which probes run a
check is the setting `check.probes`: `all`, or a list of names.

## Heartbeats

For jobs such as backups: a heartbeat check has a URL the job calls when it
ran, and with `/fail` added when it failed. If no call comes within the
period and the grace, the check goes down.

```
backup.sh && curl -fsS https://lotse.example.com/ping/<token> || curl -fsS https://lotse.example.com/ping/<token>/fail
```

The URL is the secret; the check's page shows it to admins.

## Checking what you download

Each release has `checksums.txt` with the SHA-256 of every archive, and
`checksums.txt.minisig`, its signature (decision 0010). The install script
always compares the checksum. To check a download by hand:

```
sha256sum -c checksums.txt --ignore-missing
minisign -Vm checksums.txt -P RWQGkOnHGbqiZ+oFSmuH/4VRHQB0FRdFZ3SzMcDyNqYc5apeh5h4SfQ6
```

The key is also at https://lotse.sh/minisign.pub, in the install script,
which checks the signature whenever `minisign` is installed, and (from the
update milestone on) in the binary, which checks every update with it.

Images are signed with cosign, keyless, by the release workflow:

```
cosign verify ghcr.io/cowlhq/lotse:<version> \
  --certificate-identity-regexp '^https://github.com/cowlhq/lotse/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```
