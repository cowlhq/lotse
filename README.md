# lotse

A lean, self-hosted monitoring tool for machines across several customers and
projects: host and Docker metrics from an agent, HTTP, TLS, TCP and heartbeat
checks from the outside, alerting, and one place to look at it all.

lotse runs on its own. [cowl](https://cowl.sh) can read from it, but does not
have to.

Work in progress. See [docs/design.md](docs/design.md), and
[docs/install.md](docs/install.md) for installing the hub and agents.

## Try it

```
docker compose up --build
```

starts a hub on http://localhost:8080 and three demo containers: a healthy
one, an unhealthy one and one that crashes every two minutes. Then:

1. Open the setup link from `docker compose logs hub`, choose SQLite, and
   create the admin.
2. Create an environment ("New environment": customer, project, environment).
3. "Add agent" on it shows, under "This compose stack", the line that starts
   the agent of `compose.yml` on this machine:
   `LOTSE_TOKEN=lotse_enr_… docker compose --profile agent up -d`.

Port, listen address and the hub's address come from `LOTSE_PORT`,
`LOTSE_BIND` and `LOTSE_URL`, best kept in a `.env` file next to
`compose.yml` (see there), so every `docker compose` command uses them. With
`--profile postgres` and `LOTSE_DATABASE_URL` set (see `compose.yml`), the
hub runs on Postgres.

For development, `mise run dev` runs the hub and the web interface with live
reload, and `mise run check` runs what CI runs.

## License

lotse is licensed under the [GNU AGPL-3.0](LICENSE). The Go client in
[`client/`](client/) is licensed under [Apache-2.0](client/LICENSE), so that
any program can use it.
