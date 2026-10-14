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

## Alerts

Rules are on from the start (see Settings → Alert rules): a silent agent, a
full disk, an unhealthy or restarting container, a check that is down, a
certificate that expires, a heartbeat that did not come. To be told:

1. **Settings → Mail server**, for email: host, port, sender, and the login.
   STARTTLS is the default; `TLS` for port 465.
2. **Add a channel**, in Settings for everything, or on a customer, project
   or environment for what is in it: email (recipients), Google Chat (the
   space's webhook, under Apps & integrations → Webhooks) or a webhook.
   **Test** sends a test message.
3. Set **`notify.channels`** to the channels' keys, where alerts should go
   to them: globally for all, on a customer for theirs. The nearest setting
   wins, so customer X's alerts can go to X's space and nowhere else.

Alerts that come within 30 seconds of each other arrive as one message.
`notify.repeat` sends a firing alert again, until someone acknowledges it.
A **maintenance window**, on any object or scope, holds alerts back while it
lasts; what still fires afterwards is sent then.

**Secrets**, such as a webhook's address or the SMTP password, are kept
sealed when typed in. Or the hub reads them when it sends, from an
environment variable or a Docker secret, so they never reach its database:

```yaml
services:
  hub:
    environment:
      ACME_CHAT_WEBHOOK: ${ACME_CHAT_WEBHOOK}
    secrets: [smtp-password]
secrets:
  smtp-password:
    file: ./smtp-password
```

and choose **Variable** `ACME_CHAT_WEBHOOK`, or **File**
`/run/secrets/smtp-password`, in the form. Variables starting with `LOTSE_`
and files outside `/run/secrets` cannot be used.

**A webhook** receives a POST with JSON:

```json
{
  "hub": "https://lotse.example.com",
  "test": false,
  "alerts": [
    {
      "rule": "host.disk", "subject": "acme/shop/prod/web1", "subject_kind": "host",
      "instance": "/data", "from": "ok", "state": "crit", "since": 1791963808,
      "value": 96.2, "threshold": 95, "unit": "percent",
      "url": "https://lotse.example.com/alerts/41"
    }
  ]
}
```

With a signing key, `X-Lotse-Signature: sha256=<hex>` is the HMAC-SHA256,
with the key, of `X-Lotse-Timestamp`, a dot, and the body.

**The dead man's switch**: if the hub stops, nobody tells you. Set
`deadman.url` in Settings to a check at [healthchecks.io](https://healthchecks.io)
or the like: the hub calls it every minute while its alerting runs, and the
service raises the alarm when the calls stop.

## Configuration as code

Everything you set in the UI can live in a file as well, `lotse.yaml`, kept
in Git and applied from a pipeline:

```yaml
customers:
  - key: acme
    name: ACME GmbH
    channels:
      - key: acme-chat
        googlechat:
          webhook: {from_env: ACME_CHAT_WEBHOOK}
    settings:
      notify.channels: [acme-chat]
    projects:
      - key: shop
        environments:
          - key: prod
            settings:
              rules.host.disk.warn: 80
            maintenance:
              - schedule: sun 02:00-04:00
                reason: backups
            checks:
              - key: homepage
                http: {url: "https://shop.acme.de", keyword: Willkommen}
              - key: nightly-backup
                heartbeat: {period: 24h, grace: 1h}
            hosts:
              - key: web1
                settings:
                  agent.auto_update: true
```

Make a token in **Settings → API tokens**, for everything or for one
customer, and then:

```
export LOTSE_HUB=https://lotse.example.com LOTSE_TOKEN=lotse_tok_…
lotse export acme > lotse.yaml       # start from what the hub has
lotse apply -f lotse.yaml --dry-run  # what would change, and who changed it last
lotse apply -f lotse.yaml
lotse apply -f lotse.yaml --prune --scope acme   # also delete what the file leaves out
```

Secrets are never in the file. `{from_env: NAME}` is read where `lotse
apply` runs, such as a CI secret, and kept sealed by the hub; `{env: NAME}`
and `{file: /run/secrets/name}` are read by the hub itself when it sends. An
export writes a sealed secret as `{secret: <channel>}`, which apply leaves
as it is. A host named in the file before its agent enrolled waits for it, with its
settings in place: the machine of that name that enrolls in that
environment becomes it, and no alert is raised while it waits. Hosts that
enrolled are never deleted; `--prune` deletes only waiting ones the file no
longer names.

### Docker labels

A container can say how it wants to be watched, with labels in its compose
file; the hub applies them on each deploy:

```yaml
services:
  app:
    labels:
      lotse.check.web.http.url: https://shop.acme.de/health   # a check of its own
      lotse.check.web.http.keyword: ok
      lotse.rules.container.restarting.enabled: "false"         # its own alert rules
      lotse.ignore: "true"                                      # or no alerts at all
      lotse.env: acme/shop/staging     # another environment of its host's customer
      lotse.name: shop-app             # its key, instead of project-service
```

Labels reach only the container itself: no channels, no routing, no
secrets, no other customer. What a removed label set is undone on the next
deploy; a label that cannot be applied is shown in the container's events.

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
