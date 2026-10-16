# Checks and alerts

Agents report from inside a machine; checks look at it from outside. This
page is about the checks, and about how lotse tells you when something is
wrong. Installing the hub, agents and probes is in
[Installing lotse](install.md).

## Checks

Add a check in an environment with **Add check**, in a `lotse.yaml`
([Configuration as code](configuration.md)), or with a container's labels.

| Type | What it checks |
|---|---|
| `http` | A URL: its status after redirects (200–399 unless `status` says otherwise, such as `200-299`), optionally a `keyword` in the body, the time it takes, and over HTTPS the certificate. |
| `tls` | A TLS handshake and its certificate, for what is not HTTPS, such as SMTP on port 465. |
| `tcp` | That a port accepts connections. |
| `dns` | That a name resolves, and with `expect`, to what: an `A`, `AAAA`, `CNAME`, `MX`, `NS` or `TXT` record, from the system's resolver or a `server` you name. |
| `domain` | When a domain's registration expires, as its registry publishes it (RDAP). Some, such as DENIC for `.de`, publish no date. Only the hub runs these, a few times a day. |
| `heartbeat` | That a job calls in on time (below). |

A check runs every minute (`check.interval`) and waits 10 seconds for an
answer (`check.timeout`). It runs on the hub's own probe and on every
other probe, unless `check.probes` names some. It is down when most of
the probes that run it see it fail, twice in a row; so one network having
a bad minute does not wake anyone.

## Heartbeats

For jobs such as backups: a heartbeat check has a URL the job calls when it
ran, and with `/fail` added when it failed. If no call comes within the
period and the grace, the check goes down.

```
backup.sh && curl -fsS https://lotse.example.com/ping/<token> || curl -fsS https://lotse.example.com/ping/<token>/fail
```

The URL is the secret; the check's page shows it to admins. A few calls at
once go through; after that, one a second, and the rest get 429.

## Alerts

Rules are on from the start (see Settings → Alert rules): a silent agent, a
full disk, an unhealthy or restarting container, a check that is down, a
certificate that expires, a heartbeat that did not come. To be told:

1. **Settings → Mail server**, for email: host, port, sender, and the login.
   STARTTLS is the default; `TLS` for port 465.
2. **Add a channel**, in Settings for everything, or on a customer, project
   or environment for what is in it, and **Test** sends a test message:
   - **Email**: recipients.
   - **Slack**, **Discord**, **Google Chat**: the channel's or space's
     webhook.
   - **Telegram**: a bot's token from @BotFather, and the chat it sends to.
   - **ntfy**: a topic on ntfy.sh, or on a server of your own, to push to
     phones. The topic is a secret; pick one that is hard to guess.
   - **Pushover**: your user key and an application's token. Critical
     alerts sound even in quiet hours.
   - **Webhook**: any address, which gets the alerts as JSON.
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
