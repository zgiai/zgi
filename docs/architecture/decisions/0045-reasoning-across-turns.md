# ADR 0045: Keeping Reasoning Across Turns

- Status: Proposed. `ZGI_REASONING_HISTORY=keep` turns it on; the default is unchanged.
- Date: 2026-10-30

## Context

ADR 0033 sends a model's reasoning back within its turn, which DeepSeek and Kimi thinking models require for tool calls. The durable transcript never stores reasoning, so later turns replay assistant messages without it.

A provider caches the requests a turn sent, with the reasoning in them. The next turn replays the same assistant messages without that reasoning, so its request stops matching the cached one at the previous turn's first tool call. Everything after that point is billed again: tool results, generated file content, skill instructions.

DeepSeek Harness reports a cache hit rate above 96%. A sequential, tool-heavy workload was measured to compare.

- Two conversations of eight turns each, every turn a chain of dependent tool steps: calculations, date arithmetic and file generation.
- 49 to 52 model calls in total.

## Decision

1. **With `ZGI_REASONING_HISTORY=keep`, the transcript keeps reasoning.**
   - It keeps the reasoning of assistant messages, and later turns replay it.
   - Models that do not replay reasoning still never receive it (ADR 0033).
2. **Guidance added after a tool call is recorded as well.**
   - Examples are the instructions of a skill activated mid-turn.
   - It is recorded as the user turn the provider received, so the replayed request still matches past that point.
3. **The default is unchanged.**
   - The durable transcript stores no model reasoning unless the switch is on. That policy is deliberate, and its tests say so.
   - Turning the switch on is a product decision: reasoning becomes durable data like the messages around it.

## Results

| Measure | reasoning within its turn (default) | reasoning kept |
|---|---|---|
| Turns completed | 16/16 | 16/16 |
| Model calls | 52 | 49 |
| Uncached input | 45,706 | 24,771 (-46%) |
| Cached share | 91.3% | 95.2% |
| First call of a turn, uncached | about 170 to 5,000 | about 160 to 800 |

With reasoning kept, each turn pays only for what it adds. The runtime suite passed 326/327 with the switch on; the one failure is a single-turn translation that has failed in earlier runs too. No provider rejected the replayed reasoning.

The same measurement found that the stable skill directory left out skills active from the start of a turn. When the set of recently used skills changed, so did the system prompt, and the whole cache was lost for that turn. That is fixed in both modes (ADR 0043).

## Consequences

- With the switch on, tool-heavy conversations read about half as much uncached input, and the cached share comes close to what DeepSeek Harness reports.
- Reasoning is stored with the transcript and grows it. It is replayed only to models that expect it.
- Kimi thinking models also receive earlier turns' reasoning with the switch on; only DeepSeek was measured.
