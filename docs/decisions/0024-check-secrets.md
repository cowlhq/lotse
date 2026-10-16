# 0024: Checks have secrets, as channels do

Date: 2026-10-16

## Decision

- An http check may send secrets: a header's value, such as a token in
  `Authorization`, and the password of `basic_auth`. They are kept as a
  channel's are (decision 0019): sealed as given, or a reference to the
  hub's environment or secret files, which the hub resolves.
- The spec holds no secret. The API and exports say which secrets a check
  has and where they come from, never their values; only admins see even
  that. In a lotse.yaml a secret stands where it goes, such as
  `headers: {Authorization: {from_env: API_TOKEN}}`, and an export writes
  `{secret: KEY}` for one the hub keeps sealed.
- The hub resolves a check's secrets when it hands the check to its probes,
  and sends them as headers, `basic_auth` as an `Authorization` header. A
  probe runs with what it is given, and keeps nothing.
- A secret the hub cannot resolve, such as an environment variable that is
  not set, does not keep the check quiet: the hub's own probe reports it
  as the check's error, every run, and the check goes down.
- The rules of decision 0023 hold for checks as for channels: only admins
  and tokens of the whole hub refer to the hub's own secrets, and a check
  that sends one is pointed elsewhere only by them. Labels give no secrets.

## Why

- Health endpoints often want a token. Without secrets, it would be in the
  spec, the API, every export and the Git repository the export goes to.
- A probe has to send the token to the site, so it has to have it. Whoever
  runs a probe can read what it sends; probes are the hub's admins' own
  machines, enrolled by them.
- Headers on the hub's side keep the probes as they are: one that knows
  headers knows secrets.

## Consequences

Secrets travel to every probe that runs the check, over the probes'
WebSocket. `check.probes` keeps a check, and its secrets, to some.
