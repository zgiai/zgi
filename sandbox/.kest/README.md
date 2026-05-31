# Kest sandbox flows

These flows are black-box checks for the sandbox HTTP API. They cover lifecycle,
file I/O, short-code structured results, bounded template rendering, command
execution, archive upload, skill-script style execution, artifact manifests,
timeout behavior, and security rejection paths.

Run the full local suite from the sandbox directory:

```bash
cd sandbox
make kest
```

From the repository root:

```bash
make test-sandbox-kest
```

The runner starts a temporary local sandbox server, generates the
archive inputs needed by the flows, runs Kest, then stops the server.

By default, the runner chooses a random local port and a unique worker ID so the
flows do not attach to a stale sandbox process or share active-sandbox capacity
with another local worker. Set `ZGI_SANDBOX_KEST_PORT` when a fixed port is
required.

To target an already running sandbox instead of starting a local one:

```bash
ZGI_SANDBOX_KEST_BASE_URL=http://127.0.0.1:2660 make kest
```

To run a flow against an already running sandbox:

```bash
kest run .kest/sandbox-lifecycle-files-command.flow.md --var base_url=http://127.0.0.1:2660
```

Archive flows need generated base64 variables. Use the local runner as the
reference command for those variables.

## Kest CLI notes

These flows currently avoid `@type exec` output capture for generated archive
content. In local testing, `$line.0` did not reliably capture a one-line base64
value from an exec step, so the runner generates archive variables outside Kest
and passes them with `--var`.

Kest also prints debug blocks for expected non-2xx responses in rejection tests.
That is useful while debugging, but noisy for security flows where `400` is the
passing result. A quieter mode for expected failure assertions would improve the
test experience.

Request cancellation is covered by Go tests rather than Kest flows because the
current CLI flow runner does not provide a first-class way to open an HTTP
request and abort the client connection while the server is still executing it.
