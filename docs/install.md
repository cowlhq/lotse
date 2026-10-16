# Installing lotse

lotse is one binary in three roles: the **hub** (web interface, API,
storage, alerts), the **agent** on every machine it watches, and the
**probe**, which checks from outside. This page installs all three.

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

**Backups**: with SQLite, everything is in the volume `lotse_data`.
`lotse backup` takes a consistent copy while the hub runs:

```sh
docker compose -f /opt/lotse/compose.yml exec hub lotse backup /var/lib/lotse/backup-$(date +%F).db
```

The copy lands in the volume beside the database; copy it out from there.
It does not write over a file that is there. To restore, stop the hub and
put the copy in place of `lotse.db`. With Postgres, back it up as you back up
Postgres. Keep `secret.key` from the volume either way, since it unseals
what the hub keeps sealed.

**Retention**: readings are kept as they came for 2 days, as 5-minute
averages for 30 days and as hourly ones for a year; events and alerts for as
long as the hourly ones. Change it in the hub's settings (`retention.raw`,
`retention.5m`, `retention.1h`).

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

Besides the readings, the agent asks systemd which units failed and which
services run, over D-Bus as `systemctl` would (a container agent through
`/host/run/dbus/system_bus_socket`), and notes when updates wait for a
restart, on Debian and Ubuntu (`/run/reboot-required`) and NixOS (a kernel
other than the one booted). A failed unit warns, unless `host.ignore_units`
names it (some fail on every VM, such as
`systemd-networkd-wait-online.service`); services that must run go in the
host's setting `host.services`, such as `[nginx.service]`.

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

Checks (HTTP, TLS, TCP, DNS, and a domain's expiry) run on the hub's own
probe. A probe in another
place checks from outside as well, so that the hub's network or a down
machine does not decide alone: a check is down when most of the probes that
run it see it fail. **Settings → Probes → Add probe** shows the command
for a name and a location. On a Linux machine with systemd:

```
curl -fsSL https://lotse.sh/install | sudo sh -s -- probe --hub https://lotse.example.com --token lotse_enr_… --name fra --location Frankfurt
```

This installs the binary as for the agent and starts the service
`lotse-probe`, as user `lotse`; the credential is in `/var/lib/lotse`, next
to an agent's if the machine has one. Or as a container:

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

Probes are not updated from the interface. Run the install command again
(without `--token`, the credential stays), or pull the image and start the
container again. With the token, a probe enrolls again under its name and
keeps its checks and history, even after it was revoked.

To remove a probe installed with the script:

```
sudo systemctl disable --now lotse-probe
sudo rm /etc/systemd/system/lotse-probe.service /var/lib/lotse/probe.json
```

The binary, the user and `/var/lib/lotse` stay if an agent uses them;
otherwise remove them as for the agent.

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
