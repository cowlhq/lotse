# 0007: Agents authenticate with a bearer token

Date: 2026-10-13

## Decision

- An agent enrolls once with its environment's enrollment token and gets its
  own credential: 256 random bits, sent as a bearer token.
- The hub stores only a hash of it. The agent keeps it in a file only it can
  read (mode 0600).
- Each credential can be revoked on its own. Probes work the same way.

## Why not mTLS

Client certificates need TLS to reach the hub unbroken, which rules out the
usual reverse proxy (0006) and Cloudflare in front of it.
