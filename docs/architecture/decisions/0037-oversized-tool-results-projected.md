# ADR 0037: Oversized Tool Results Reach the Model Projected

- Status: Accepted
- Date: 2026-10-29

## Context

A tool result, such as a log, a long file or a query result, went into the model request whole. A 50,000-token result was paid in full the first time it was sent. It slowed the first token and buried what the task needed in text it did not.

Only when a request reached 80% of the model's window did the context manager compress older results over 32K tokens to their beginning and end. That compressed form was written back to the transcript, so the middle was lost for good, with no way to read it.

An earlier design projected oversized results and kept the originals in files under the API's local storage, with `read_context_artifact` to read them back. That store was removed in September 2026. Local files are not shared between instances, can disappear with a restart, and keep business data on disk outside any retention policy. The projection code and the reader stayed in the tree, unused.

## Decision

1. Before every model request, a tool result over 16K tokens is replaced by its beginning and end, about 12,000 characters in all with three quarters from the beginning. A reference `agent-context://tool-results/<run>/<sha256>` is added.
2. Only the outgoing request is projected:
   - The run's messages and the persisted turn transcript keep the original.
   - The transcript sync no longer writes a projection over an original.
   - The projection is computed from the original each time, so it is the same in every request and does not disturb the provider's prompt cache (ADR 0032).
3. There is no separate store. The original is found by its content hash among the run's messages and transcript:
   - A continuation or a resumed turn (ADR 0035) reloads the transcript, so it can still read the original.
   - The original lives and is deleted with its message.
4. `read_context_artifact` reads a part of the original: a range of characters (`offset`, `limit`), or the lines that contain a `query`, with their line numbers. A read returns at most 24,000 characters, so it cannot bring back the size the projection removed.
5. The native agent loop always offers `read_context_artifact` when it manages context, so the tool list does not change when a first projection appears.
6. The 80% compaction stays as it was for results that are still large in the request; with projection, those are rare.

## Consequences

- A request carries at most about 16K tokens of any one tool result. A model that needs the rest reads it in parts, which costs a round.
- The originals are kept in the transcript, as before. This decision adds no storage, no retention concern and no per-instance state.
- Every native-loop request carries one more tool definition, about 200 tokens, which providers cache.
- The model may answer from the beginning and end without reading the middle. The preview says how many characters were omitted and how to read them.
