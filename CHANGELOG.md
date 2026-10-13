# Changelog

## [0.1.3](https://github.com/cowlhq/lotse/compare/v0.1.2...v0.1.3) (2026-10-13)


### Features

* **agent:** enrolling again with a token brings back a revoked agent ([e38c37e](https://github.com/cowlhq/lotse/commit/e38c37e68496e4a94c2b32817bd6da3b65e29f01))
* **hub:** hosts say whether their agent is revoked ([88528b7](https://github.com/cowlhq/lotse/commit/88528b7be6c01e657233170b875a94e39eeb1bba))
* **site:** the install command brings back a revoked agent ([5a36449](https://github.com/cowlhq/lotse/commit/5a36449d91c525ee0e57a80f07d880198636cacd))


### Bug Fixes

* **web:** a revoked agent shows as revoked, and revoking takes one click ([3b06d69](https://github.com/cowlhq/lotse/commit/3b06d69d79dc1a5c2f9a1a024d89ced694036599))

## [0.1.2](https://github.com/cowlhq/lotse/compare/v0.1.1...v0.1.2) (2026-10-13)


### Features

* **agent:** update when the hub asks, and tell it about rollbacks ([962421d](https://github.com/cowlhq/lotse/commit/962421dc0fbdf5c6f3bf8e31bd171894db8fe982))
* automatic updates in a window, one host per environment first ([4ba8eac](https://github.com/cowlhq/lotse/commit/4ba8eac743502178d7408f1b4e47909cb9004f1c))
* **hub:** know the newest release, update agents on request, and update itself ([6104488](https://github.com/cowlhq/lotse/commit/610448809ba5d67bc6373eb3b4e563527f38e325))
* **update:** install signed releases where the starter finds them ([2c21286](https://github.com/cowlhq/lotse/commit/2c2128647004a4b4e798cabe3e8b5eb086105d24))
* **web:** updates for agents and the hub, and every host's events ([2a71768](https://github.com/cowlhq/lotse/commit/2a71768f44822fcb77f4abc3010d78c00c96e443))


### Bug Fixes

* **site:** install where /usr/local/bin is missing, and stop early on NixOS ([f437aa9](https://github.com/cowlhq/lotse/commit/f437aa92cd54db45a154312427ab84c003955bf8))
* **web:** event texts with a colon in their kind ([87a85a5](https://github.com/cowlhq/lotse/commit/87a85a52eec96dfc4aa9c6b0e2fd692e55fdbbcd))

## [0.1.1](https://github.com/cowlhq/lotse/compare/v0.1.0...v0.1.1) (2026-10-13)


### Features

* **agent:** lotse agent enroll only enrolls, for the install script ([7c5c665](https://github.com/cowlhq/lotse/commit/7c5c6658b95f1c883d702798367ffd1cd40fef2d))
* agents enroll with their environment's token and keep a credential of their own ([f580c20](https://github.com/cowlhq/lotse/commit/f580c20c3aceeda0ec21b799700cc90bba2f7a36))
* **auth:** Argon2id passwords, session tokens and a brake on guessing ([3112441](https://github.com/cowlhq/lotse/commit/311244154bc127854640e737f40685b65945c1b7))
* **config:** the settings there are, and how they are inherited ([22f033e](https://github.com/cowlhq/lotse/commit/22f033e18d2491ed38681b3fa15ac14b2e4eb1b4))
* **hub:** effective settings with their origin, and agents told at once ([b53c2a9](https://github.com/cowlhq/lotse/commit/b53c2a9b92033dd0513c21497dca02d5b711d86c))
* **hub:** setup in two steps, database and admin, and login with sessions ([e221137](https://github.com/cowlhq/lotse/commit/e2211372c64c7e0108e288b1aa3e2a5ef35c439f))
* **hub:** the tree, hosts in environments, and the history of changes ([cfa34b9](https://github.com/cowlhq/lotse/commit/cfa34b9d3e0495132fd8580e0ae36fd886b619b6))
* **secret:** seal what the hub keeps but must read again ([a643e80](https://github.com/cowlhq/lotse/commit/a643e809dab43b8b9371120c4f52e81a6a70563a))
* **site:** publish lotse's signing key and check signatures in the install script ([02fffad](https://github.com/cowlhq/lotse/commit/02fffad7945ae6a8a1f9ba6bb5dd18e21fbd5e70))
* **site:** the install script, for agents with systemd and for the hub with compose ([797ba0b](https://github.com/cowlhq/lotse/commit/797ba0b44cd8ea9a46d31d2b5809165980343e17))
* **store:** customers, projects and environments, and a history of changes ([7a64b42](https://github.com/cowlhq/lotse/commit/7a64b425ba027792e62e1de6ee83959ba5389dce))
* **store:** enrollment tokens and agent credentials ([7ea8a2c](https://github.com/cowlhq/lotse/commit/7ea8a2c0bd843a44038b2d184306c8513f511ca1))
* **store:** settings on objects and scopes, with history ([2d18c08](https://github.com/cowlhq/lotse/commit/2d18c08ed97576dfde50f7f788c9f8ee75104d06))
* **store:** users by email, and sessions ([85a6a12](https://github.com/cowlhq/lotse/commit/85a6a12206a0f19c20e4c7a093c7a47d37dc01cb))
* the compose hub listens where LOTSE_BIND says, such as the tailnet ([2a617cd](https://github.com/cowlhq/lotse/commit/2a617cdca3d693647c92f1a8fbb2d457d8e3448f))
* **web:** add an agent with one line, and revoke it ([9474382](https://github.com/cowlhq/lotse/commit/947438272ae43a91a43f3c809ba9c0eaa4f210c4))
* **web:** settings and where they come from on every host and scope ([7cae9e3](https://github.com/cowlhq/lotse/commit/7cae9e3f3277423c569f743ad556a93747b6387e))
* **web:** the overview by customer, project and environment, new environments, history ([8255057](https://github.com/cowlhq/lotse/commit/8255057ceafec094603156a0e0b0c13536dbdaf7))
* **web:** the setup wizard and the login page ([9d32eb5](https://github.com/cowlhq/lotse/commit/9d32eb50de2b58a90c8c6a23c87092f89ca66da7))


### Bug Fixes

* **agent:** the first reading already has rates ([d03735a](https://github.com/cowlhq/lotse/commit/d03735a7292c33d84a9489d8751ba0c9a7de0df0))

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
