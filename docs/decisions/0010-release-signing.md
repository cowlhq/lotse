# 0010: minisign for binaries, cosign for images

Date: 2026-10-13

## Decision

- Each release's `checksums.txt` is signed with minisign (Ed25519). The public
  key is built into the binary, which verifies updates by itself (0009) with a
  few lines of Go. The binary accepts two keys, so the key can be rotated.
- Images are signed keyless with cosign through GitHub's OIDC identity.
- The private minisign key is a GitHub Actions secret, with an offline backup
  kept by the author.
- The install script always checks checksums, and checks the signature too if
  `minisign` is installed. The documentation shows how to verify by hand.

## Why not cosign for binaries too

Verifying keyless signatures inside the binary needs sigstore-go and TUF, a
heavy dependency, and would make every update depend on Sigstore's services.
