# 0011: Alerts by email, ntfy and webhook

Date: 2026-10-25

Taken from lotse 0.1, decision 0017, trimmed to checks.

## Decision

- **Channels**: email, ntfy and webhook.
  - Email goes through the hub's SMTP server, with STARTTLS unless told
    `tls` (port 465) or `none`. With `none`, a password is only sent to a
    server on the same machine.
  - ntfy posts to a topic on ntfy.sh or on the owner's own server,
    optionally with a token.
  - A webhook gets a documented JSON body. With a signing key,
    `X-Lotse-Signature` is `sha256=` and the HMAC-SHA256 of
    `X-Lotse-Timestamp`, a dot and the body.
- **Who is told** is a setting, and inherits along hub, space, sub-space
  and check (decision 0003).
- **A check is down after two failed runs in a row**, and is told again
  when it is back. That works the same for a check every 30 seconds and
  one every hour.
- **Gathered for 30 seconds per channel.** The first alert for a channel
  waits 30 seconds for others; then all go out as one message.
- **Secrets** of channels and the mail server are sealed in the database.
  Errors never contain a webhook's address.
- **Notifications are rows** in the database, so a restart loses none that
  were waiting. Sending is tried 6 times, waiting 30 seconds to 10 minutes
  in between.

## Why ntfy and not more

Webhooks reach almost anything, but need a bridge for a phone. ntfy is one
POST, free, self-hostable and popular with people who run a homelab. Other
chat and push services can follow when someone asks.
