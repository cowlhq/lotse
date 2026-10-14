# The read API

Other programs read a hub through `/api/v1`, with an API token made in
**Settings → API tokens** (either kind). A token sees everything, or one
customer, project or environment; anything outside its scope answers 404.
The hub describes its API at `/api/openapi.yaml`; the operations tagged
`read` are the read API, and stay stable (decision 0020).

```
curl -H "Authorization: Bearer $LOTSE_TOKEN" https://lotse.example.com/api/v1/status
{"level":"crit","hosts":12,"hosts_online":12,"checks":30,"checks_up":29,"critical":1,"warnings":0}
```

| | |
|---|---|
| `GET /api/v1/status` | how everything stands, in figures |
| `GET /api/v1/scopes` | customers, projects, environments |
| `GET /api/v1/hosts` | hosts with their newest reading |
| `GET /api/v1/hosts/{id}` | a host with filesystems and containers |
| `GET /api/v1/hosts/{id}/series?from=&to=&points=` | a host's readings over time |
| `GET /api/v1/containers/{id}/series` | a container's readings over time |
| `GET /api/v1/checks` | checks with their state |
| `GET /api/v1/checks/{id}/series` | a check's response time and success |
| `GET /api/v1/alerts` | what is not OK now |
| `GET /api/v1/alerts/history?from=&to=` | transitions, newest first |
| `GET /api/v1/export?scope=acme` | a scope as lotse.yaml |

Times are Unix seconds, sizes bytes, rates bytes per second, shares percent.

## The Go client

`lotse.sh/client` is Apache-2.0, so any program can use it:

```go
import "lotse.sh/client"

hub, err := client.New("https://lotse.example.com", os.Getenv("LOTSE_TOKEN"))
status, err := hub.Status(ctx)
hosts, err := hub.Hosts(ctx)
cpu, err := hub.HostSeries(ctx, hosts[0].Host.Id, client.Range{From: time.Now().Add(-time.Hour)})
```

An answer other than the one asked for is a `*client.APIError` with the HTTP
status: 401 for a token that does not work, 404 for something not there or
not in the token's scope.
