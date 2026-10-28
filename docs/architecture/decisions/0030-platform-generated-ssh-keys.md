# ADR 0030: Platform-Generated SSH Keys

- Status: Accepted
- Date: 2026-10-28
- Builds on: ADR 0025

## Context

Connecting a server asks for a password or a private key. Both are friction:

- a password login has to stay enabled on the server, and many hardening guides turn it off;
- most people do not have a spare key pair at hand, and pasting their personal key gives the platform more than it needs.

Hosted tools usually offer a third way: the platform makes a key for the connection, and the person installs only its public half.

## Decision

1. The SSH setup panel offers **Generated key (recommended)** next to password and private key. It is the default for a new connection when the browser can generate Ed25519 keys; reconnecting keeps password as the default.
2. The browser generates the pair with WebCrypto (Ed25519). The private key is exported as unencrypted PKCS#8 PEM and goes into the private-key field. From there it travels exactly like a pasted key: write-only, through the probe and the strict save, stored encrypted, never shown or returned. The server needs no new endpoint or credential type, because `ssh.ParsePrivateKey` accepts PKCS#8 Ed25519.
3. The panel shows the public key as an `authorized_keys` line with the comment `zgi-ops`, plus one command that installs it for the login user. Both have copy buttons. Quotes are stripped from the line, so it cannot break out of the command. **Generate another key** replaces the pair, and the earlier probe result becomes stale.
4. Switching method clears the key. If generation fails, the panel says so and the person picks password or private key.

## Consequences

- A server can keep password login disabled, and each connection has its own key, which the person revokes by deleting one `authorized_keys` line.
- The private key exists only in the form until it is saved, as a pasted key does. Nobody can download it later. Reconnecting with a new key means generating and installing another one.
- Browsers without Ed25519 in WebCrypto fall back to the existing methods.
