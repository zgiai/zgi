# ADR 0045: Keeping Reasoning Across Turns

- Status: Accepted. Supersedes decision 3 of ADR 0033.
- Date: 2026-10-30

## Context

ADR 0033 sends a model's reasoning back within its turn, which DeepSeek and Kimi thinking models require for tool calls. The durable transcript never stores reasoning, so later turns replay assistant messages without it.

A provider caches the requests a turn sent, with the reasoning in them. The next turn replays the same assistant messages without that reasoning, so its request stops matching the cached one at the previous turn's first tool call. Everything after that point is billed again: tool results, generated file content, skill instructions.

DeepSeek Harness reports a cache hit rate above 96%. A sequential, tool-heavy workload was measured to compare.

- Two conversations of eight turns each, every turn a chain of dependent tool steps: calculations, date arithmetic and file generation.
- 49 to 52 model calls in total.

## Decision

1. **Models that replay reasoning keep it across turns.** This is the default.
   - For DeepSeek and Kimi models, the turn transcript keeps the reasoning of assistant messages, and later turns replay it.
   - Other models never receive reasoning (ADR 0033), so their transcript stores none.
   - `ZGI_REASONING_HISTORY=turn` restores ADR 0033: reasoning stays within its turn.
2. **Guidance added after a tool call is recorded as well.**
   - Examples are the instructions of a skill activated mid-turn.
   - It is recorded as the user turn the provider received, so the replayed request still matches past that point.
   - Like reasoning, it is recorded only for models that replay reasoning.
3. **Reasoning is sent to the model and nowhere else.**
   - It lives in the transcript entries, which only build model requests. They are deleted with their message and conversation.
   - The HTTP and SSE boundaries already drop the agent transcript from message metadata, and runtime logs drop `reasoning_content`.
   - The reasoning a user sees streamed is a separate, existing field of the message, unchanged by this decision.
4. **Earlier turns' reasoning is the first thing a full context gives up.**
   - The token estimate counts reasoning for models that are sent it.
   - When a request does not fit, the reasoning of earlier turns is left out before tool results are shortened, history is compacted or left out. The decision records `reasoning_dropped`.
   - The current turn keeps its reasoning, which its tool calls need.

## Results

| Measure | reasoning within its turn (`turn`) | reasoning kept (default) |
|---|---|---|
| Turns completed | 16/16 | 16/16 |
| Model calls | 52 | 49 |
| Uncached input | 45,706 | 24,771 (-46%) |
| Cached share | 91.3% | 95.2% |
| First call of a turn, uncached | about 170 to 5,000 | about 160 to 800 |

With reasoning kept, each turn pays only for what it adds. The runtime suite passed 326/327 with reasoning kept; the one failure is a single-turn translation that has failed in earlier runs too. No provider rejected the replayed reasoning.

The same measurement found that the stable skill directory left out skills active from the start of a turn. When the set of recently used skills changed, so did the system prompt, and the whole cache was lost for that turn. That is fixed in both modes (ADR 0043).

## Consequences

- Tool-heavy DeepSeek and Kimi conversations read about half as much uncached input, and the cached share comes close to what DeepSeek Harness reports.
- The transcript grows by the reasoning of those models. A request that no longer fits loses that reasoning first, at the cost of one cache miss.
- Kimi thinking models also receive earlier turns' reasoning; only DeepSeek was measured.
- The tests that pinned "reasoning is never stored" now pin that only models replaying reasoning store it.
