# 0020: The read API is its own, stable part of the API

Date: 2026-10-14

## Decision

- The read API is the part of `api/openapi.yaml` under `/api/v1`, tagged
  `read`: status, scopes, hosts, a host, checks, alerts and their history,
  series of hosts, containers and checks, and export. It is kept stable:
  fields are added, not changed or removed, until a `/api/v2`.
- The rest of the API is the web interface's own and changes with it.
- It takes an API token of either kind (decision 0019) and answers only
  about what is in the token's scope; something outside it is 404, as if it
  were not there. A heartbeat's ping address, its secret, is never in it.
- The Go client, `lotse.sh/client` (Apache-2.0, decision 0001), is made
  from the operations tagged `read` by oapi-codegen, with a small hand-made
  `Hub` on top: `client.New(hub, token)`, then `Status`, `Hosts`, `Checks`,
  `Alerts`, series and `Export`.
- The hub serves its API's description at `/api/openapi.yaml`, without a
  login.

## Consequences

The UI can change its API freely. Programs that read lotse, such as cowl,
use `/api/v1` or the client and keep working across releases.
