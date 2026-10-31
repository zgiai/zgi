# ADR 0034: The Agent Loop Retries Transient Model Failures

- Status: Accepted
- Date: 2026-10-29

## Context

The gateway fails over between the routes of a model, up to three, but it does not retry a route or wait between attempts. Many deployments have one route per model. On such a deployment, a single 429 or 503 from the provider ended the whole Agent turn, even when the turn had already run tools and spent several rounds.

The loop already handled two kinds of failure:

- When opening a stream fails, the round falls back to one call without streaming.
- The terminal-only path retries a call that gave no usable final answer once, and it treated most errors as worth that retry.

## Decision

1. Each model round of the Agent loop retries a call that failed with a transient error. It retries at most twice, waiting 1 and then 4 seconds. `Runner.ModelRetryDelays` can change the waits.
2. A transient error is one of these:
   - provider throttling: 429, or `ErrRateLimited`;
   - a server error: 5xx, or `ErrUpstreamError`;
   - a proxy failure: `ErrProxyError`;
   - a stream cut off: `ErrStreamClosed`.
3. These are never retried:
   - a 4xx other than 429, even under `ErrUpstreamError`, which adapters also use for unknown statuses;
   - rejected requests;
   - authentication, balance, quota and pricing states;
   - a failed settlement after a completed call, since a retry would bill again;
   - timeouts, which have already waited the full timeout once;
   - a failure after the user saw part of the answer.
4. The terminal-only retry no longer retries a transient error, so that one failure is not retried at both levels.
5. A lost connection counts as a cut-off stream. Adapters pass the read error of a broken connection through as it is: EOF, an unexpected EOF, a reset, or a TLS record failure. The round marks it as `ErrStreamClosed` (`connectionLost`) before deciding, for a stream and for a call without streaming. Network timeouts and cancellations keep their own errors. Added 2026-10-30, after a stream that failed with `tls: bad record MAC` before any answer ended a turn as an internal error.

## Consequences

- One provider hiccup no longer ends a turn that has done most of its work. A round can take up to three calls and five seconds longer before it fails.
- A connection that still fails after the retries reaches the user as an unavailable upstream, not an internal error.
- Each attempt is billed and traced as its own model call, as failover attempts already are.
- The loop still does not retry a round that stopped at the output limit (`finish_reason: length`). That is a separate decision.
