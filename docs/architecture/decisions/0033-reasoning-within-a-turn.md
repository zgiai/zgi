# ADR 0033: Reasoning Goes Back Within a Turn

- Status: Accepted
- Date: 2026-10-29

## Context

Thinking models return their reasoning next to the answer, in `reasoning_content`. DeepSeek documents that when a thinking-mode turn calls tools, each later request in that turn must include the reasoning of the earlier tool-call rounds, or the API rejects it with a 400. Kimi thinking models use the field the same way. Reasoning from earlier turns is not needed: those providers drop it.

The agent loop cleared `reasoning_content` from every assistant message before the next round. DeepSeek V4 Flash and V4 Pro accepted the requests anyway when we checked on 2026-10-28. But the model lost its own plan between rounds, and the requests did not follow the documented protocol.

The provider adapters already pass the field through, and a contract test requires it. The gateway also serves API callers who send the field on purpose. So the adapters are not the place to decide what an agent replays.

## Decision

1. The loop keeps `reasoning_content` on the tool-call rounds of the current turn, both in its own message list and in the context manager's active request.
2. When the runner builds a model request, it keeps the field only for models that use it. These are models whose name contains `deepseek` or `kimi`, including names served by other hosts, such as `deepseek/deepseek-chat`. For every other model it clears the field, because other OpenAI-compatible APIs may reject an unknown message field.
3. Reasoning is never stored. The durable turn transcript, checkpoints, and history rebuilt for a later turn have no reasoning, so a new turn starts without it.

## Consequences

- DeepSeek and Kimi tool turns follow the documented protocol, and the model sees its earlier reasoning in later rounds of the same turn.
- Requests within a turn grow by the reasoning they carry. The context budget counts it, and compaction can remove it with the rounds it belongs to.
- Measured on DeepSeek V4 Flash: a four-round tool turn sent the reasoning from round 2 on, with no errors. Rounds 3 and 4 read 13,440 and 14,080 tokens from the cache.
- A new thinking model that needs the field must be added to `adapter.ModelReplaysReasoningContent`.
