# 0017: How alerts are sent

Date: 2026-10-14

## Decision

- **Mail server: one for the hub, and one per channel if needed.** Settings
  → Mail server holds the SMTP server email channels send through. An email
  channel can name its own instead, as for a customer whose mail goes out
  through their own server.
- **Encrypted by default.** SMTP uses STARTTLS unless told `tls` (port 465)
  or `none`. With `none`, a password is only sent to a server on the same
  machine (Go's `smtp.PlainAuth`).
- **Gathered for 30 seconds per channel.** The first alert for a channel
  waits 30 seconds for others; then all go out as one message. A larger
  outage arrives as one message, at the price of half a minute.
- **Acknowledging** a firing alert notes who looks after it, and stops its
  repeats (`notify.repeat`). A change of state, such as WARN to CRIT, is
  sent all the same and ends the acknowledgement. Only admins acknowledge,
  and it is in the object's history.
- **Maintenance windows** are once (from, until) or by schedule
  (`sun 02:00-04:00`, `mon-fri 22:00-23:00`, `daily 04:00-06:00`), in the
  hub's time zone. During one, transitions are recorded and marked, not
  sent; what still fires when it ends is sent then. A window on a host covers
  its containers.
- **Webhooks** get a documented JSON body (see docs/install.md). With a
  signing key, `X-Lotse-Signature` is `sha256=` and the HMAC-SHA256 of
  `X-Lotse-Timestamp`, a dot and the body.
- **Secrets** of channels and the mail server are sealed, or references the
  hub resolves when it sends: an environment variable that is not one of
  lotse's own (`LOTSE_…`), or a file in `/run/secrets` (`LOTSE_SECRETS_DIR`).
  Otherwise an admin could send the hub's own key to a webhook of theirs.
  Errors never contain a webhook's address.
- **Containers inherit settings from their host**, then the host's
  environment and up, so that a rule turned off on a host is off for its
  containers.
- **`check.down` counts runs**: its value is how many runs in a row failed on
  most of the check's probes, CRIT at 2. That works the same for a check
  every 10 seconds and one every hour.

## Consequences

Notifications are rows in the database, so a restart of the hub loses none
that were waiting. A message names at most 50 alerts; more make more
messages. Sending is tried 6 times, waiting 30 s to 10 min in between; a
channel that failed 3 times in a row raises `channel.failing`, sent to the
global channels.
