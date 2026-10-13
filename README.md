# lotse

A lean, self-hosted monitoring tool for machines across several customers and
projects: host and Docker metrics from an agent, HTTP, TLS, TCP and heartbeat
checks from the outside, alerting, and one place to look at it all.

lotse runs on its own. [cowl](https://cowl.sh) can read from it, but does not
have to.

Work in progress. See [docs/design.md](docs/design.md).

## License

lotse is licensed under the [GNU AGPL-3.0](LICENSE). The Go client in
[`client/`](client/) is licensed under [Apache-2.0](client/LICENSE), so that
any program can use it.
