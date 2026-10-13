# Changelog

## 0.1.0 (2026-10-13)


### Features

* **agent:** connect to the hub, say hello, and reconnect with backoff ([80eab5a](https://github.com/cowlhq/lotse/commit/80eab5a04d7f2be83160c4cebd7b8cdd8b716706))
* **agent:** read every 3 s while the hub says someone is looking ([bb8fc76](https://github.com/cowlhq/lotse/commit/bb8fc760accac502eadacf07e86e1c2fafb0cf3a))
* **agent:** read the machine and keep 15 minutes of readings while the hub is away ([2d07efa](https://github.com/cowlhq/lotse/commit/2d07efaf8387cd28985019ab7235230cde8887b7))
* **agent:** watch Docker for containers, their events and their usage ([3f2cefb](https://github.com/cowlhq/lotse/commit/3f2cefb68ca5f455a053cbb0ba4ad8ec33bfa72a))
* **api:** describe the hub's API in OpenAPI and generate its code from it ([58e0c7c](https://github.com/cowlhq/lotse/commit/58e0c7cb9cad08f9a73967e66a90b2b7aa703fe5))
* docker compose up runs a hub and an agent, optionally on Postgres ([8304a70](https://github.com/cowlhq/lotse/commit/8304a70fe8d6c188605c1c4355a9fc39bf59cec7))
* **hub:** accept agents over a WebSocket, for now with one shared key ([6f400b9](https://github.com/cowlhq/lotse/commit/6f400b99b1896bd2e9c5bb5ecf06d18390bd9f3a))
* **hub:** delete readings older than 48 hours ([292c390](https://github.com/cowlhq/lotse/commit/292c390923d2a85857d812fadd451678a717bff8))
* **hub:** save containers, their events and their usage ([d0eaadf](https://github.com/cowlhq/lotse/commit/d0eaadf54898a1e8d756c6c1b0916d91e2cfe76a))
* **hub:** save hosts, their readings, reboots and connections ([4e77e48](https://github.com/cowlhq/lotse/commit/4e77e48a7a0e06df2bef431744788e6c0114566f))
* **hub:** send browsers live readings and keep watched hosts' agents fast ([86d1a75](https://github.com/cowlhq/lotse/commit/86d1a75bbb2f36d7e9ff02191ec5a0172488aebd))
* **hub:** serve health, setup link checks and the embedded web interface ([1fdc05c](https://github.com/cowlhq/lotse/commit/1fdc05c594ace7c274ec369d2b1e4f57a6fb0b2e))
* **hub:** serve hosts, containers, their readings and events ([55b96c4](https://github.com/cowlhq/lotse/commit/55b96c462cba20bf679eed5d944b2cfef71a4eb5))
* lotse start runs the hub or the agent through the starter ([7c7eb2f](https://github.com/cowlhq/lotse/commit/7c7eb2fb1350404f4bddf728ee5d9dc8b213a048))
* one lotse binary that prints its version ([1a6cc8f](https://github.com/cowlhq/lotse/commit/1a6cc8f553aca8085cc3b502ee42888abef0aaec))
* **protocol:** containers, their events and their usage ([1388a4c](https://github.com/cowlhq/lotse/commit/1388a4ceb4340bf6f865f35a6705b4904f1fb56a))
* **protocol:** host readings and the agent's configuration ([8ec99cf](https://github.com/cowlhq/lotse/commit/8ec99cfc800e889b4b20fc3c40c9835a5265fb91))
* **protocol:** versioned messages between agents and the hub ([e5251f3](https://github.com/cowlhq/lotse/commit/e5251f3d793997a0a97632354dacfff84203c2b5))
* **site:** lotse.sh answers go get for the module path ([15b8a56](https://github.com/cowlhq/lotse/commit/15b8a5693d495f4eb424368059fa165194df83c3))
* **store:** hosts, containers, filesystems, their samples and events ([da3772c](https://github.com/cowlhq/lotse/commit/da3772ced88d4ec1197585ffbc7e4f49324ea61d))
* **store:** open SQLite or Postgres and migrate either to the same schema ([3ffa879](https://github.com/cowlhq/lotse/commit/3ffa87927bac1f9c46fa26a3736cf0ad42bd488d))
* **store:** the newest readings of hosts, filesystems and containers ([d5b6dc6](https://github.com/cowlhq/lotse/commit/d5b6dc67053099fc8d973093010bb9febc705818))
* the compose agent watches the machine, next to three demo containers ([1a2acd8](https://github.com/cowlhq/lotse/commit/1a2acd845f97e497ebf716cb67f5d53fffbb1e06))
* **update:** a starter that runs the newest good version and goes back from a bad one ([eedc3ec](https://github.com/cowlhq/lotse/commit/eedc3eca073344a1113813db550e0888a3450d46))
* **web:** a React app that shows the hub's health and checks setup links ([f140470](https://github.com/cowlhq/lotse/commit/f1404700414a7625276f5c17faed614f6010969b))
* **web:** hosts at a glance, and each host and container with its graphs ([5dd8624](https://github.com/cowlhq/lotse/commit/5dd86246195c7c10069c778b9741b7b63ef963cb))
* **web:** live readings in the overview, host and container pages ([ee4f8b3](https://github.com/cowlhq/lotse/commit/ee4f8b3a67170f6ab86d15830712eeb95073f207))
* **web:** show how many agents are connected ([61726b8](https://github.com/cowlhq/lotse/commit/61726b89a222e5acb24c56dd77c85bfefa049687))


### Bug Fixes

* **agent:** report the root filesystem when it is a bind mount ([0584915](https://github.com/cowlhq/lotse/commit/05849154a14fb6f211051321fdb4d1ec577c1425))
* **hub:** count only running containers as unhealthy ([f37bfbd](https://github.com/cowlhq/lotse/commit/f37bfbdf3104c7b841d02557e0eb7c766464ace9))
