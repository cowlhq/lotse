# Configuration as code

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
              - key: shop-dns
                dns: {name: shop.acme.de, type: A, expect: [203.0.113.10]}
              - key: domain
                domain: {domain: acme.com}
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

Or in the interface, as an admin: drop a `lotse.yaml` anywhere on it, or
choose one in **Settings → General** or a customer's **Settings**. It shows
what would change, and applies it with a click; what `{from_env: NAME}`
would read where `lotse apply` runs, it asks for. It never deletes, as
`--prune` would.

Other channels take their secrets the same way:

```yaml
channels:
  - key: ops-phone
    ntfy: {topic: {from_env: NTFY_TOPIC}}          # server: https://ntfy.example.com
  - key: ops-telegram
    telegram: {chat_id: "-1001234567890", bot_token: {env: TELEGRAM_BOT}}
  - key: ops-slack
    slack: {webhook: {from_env: SLACK_WEBHOOK}}    # discord: the same
  - key: oncall
    pushover: {user: {env: PUSHOVER_USER}, token: {env: PUSHOVER_APP}}
```

And so do checks, for a header or a login:

```yaml
checks:
  - key: api
    http:
      url: https://api.acme.de/health
      headers: {Authorization: {from_env: ACME_API_TOKEN}}
      basic_auth: {username: monitor, password: {file: acme-api-password}}
```

Secrets are never in the file. `{from_env: NAME}` is read where `lotse
apply` runs, such as a CI secret, and kept sealed by the hub; `{env: NAME}`
and `{file: /run/secrets/name}` are read by the hub itself when it sends. An
export writes a sealed secret as `{secret: <channel>}` or `{secret:
<check>}`, which apply leaves as it is. A host named in the file before its agent enrolled waits for it, with its
settings in place: the machine of that name that enrolls in that
environment becomes it, and no alert is raised while it waits. Hosts that
enrolled are never deleted; `--prune` deletes only waiting ones the file no
longer names.

## Docker labels

A container can say how it wants to be watched, with labels in its compose
file; the hub applies them on each deploy:

```yaml
services:
  app:
    labels:
      lotse.check.web.http.url: https://shop.acme.de/health   # a check of its own
      lotse.check.web.http.json.path: status                    # a field of a field
      lotse.check.web.http.json.equals: UP
      lotse.rules.container.restarting.enabled: "false"         # its own alert rules
      lotse.ignore: "true"                                      # or no alerts at all
      lotse.env: acme/shop/staging     # another environment of its host's customer
      lotse.name: shop-app             # its key, instead of project-service
```

Labels reach only the container itself: no channels, no routing, no
secrets, no other customer. What a removed label set is undone on the next
deploy; a label that cannot be applied is shown in the container's events.
