# lotse

A lean, self-hosted monitoring tool for machines across several customers and
projects: host and Docker metrics from an agent, HTTP, TLS, TCP and heartbeat
checks from the outside, alerting, and one place to look at it all.

lotse runs on its own. [cowl](https://cowl.sh) can read from it, but does not
have to.

Work in progress. See [docs/design.md](docs/design.md).

## Try it

```
docker compose up --build
```

starts a hub on http://localhost:8080, an agent watching this machine and its
containers, and three demo containers: a healthy one, an unhealthy one and one
that crashes every two minutes. The setup link is in `docker compose logs hub`. `LOTSE_PORT=18080 docker compose up`
uses another port; `docker compose --profile postgres up` with
`LOTSE_DATABASE_URL` set (see `compose.yml`) runs on Postgres instead of
SQLite.

For development, `mise run dev` runs the hub and the web interface with live
reload, and `mise run check` runs what CI runs.

## License

lotse is licensed under the [GNU AGPL-3.0](LICENSE). The Go client in
[`client/`](client/) is licensed under [Apache-2.0](client/LICENSE), so that
any program can use it.
