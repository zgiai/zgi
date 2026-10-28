# ADR 0032: A Cache-Stable Prompt Prefix

- Status: Accepted
- Date: 2026-10-28

## Context

Model providers cache a prompt prefix that repeats between requests:

- Anthropic, when the request marks it with `cache_control`;
- OpenAI, DeepSeek and others, automatically.

A cache hit is billed at a fraction of the input price and starts answering sooner. An agent sends the same system prompt, tools and history on every round of a turn and on every turn of a conversation, so most of its input could come from the cache.

Four things prevented that:

1. A DigitalHuman run appended changing guidance to its system prompt: the current time, connector notes, channel context, memory, and task guidance. The time alone made the system prompt differ on every run, so no provider could reuse anything after it.
2. Before every model call, the runtime moved every system message to the front and merged them into one. That made the request acceptable to providers that allow system content only at the start, but it also put the turn boundary, the skill catalog ranked for the current question, and the loop's per-round notices into the system prompt. The first token that changed was in the system prompt, so no provider could reuse any of the request after a turn.
3. The Claude adapter did the same with every system message, and no request set `cache_control`.
4. Connector operations were ranked against each question, and the ranking also decided their order in the tool list. Tools come first in a request, so a different order invalidated everything after them.

## Decision

1. `RunConfig.TurnContext` carries server-owned guidance that changes from run to run. The runtime sends it as one system message after the history and just before the current user message, next to the memory and turn-boundary context. `SystemPrompt` keeps only what is the same on every run: the persona, its rules and its configuration.
2. A DigitalHuman puts all of its per-run guidance in `TurnContext`, in the first run and in every continuation (connector confirmation, user input, task approval):
   - the current time;
   - per-user connector and approval notes;
   - channel context and preview guidance;
   - memory and task guidance.
3. The runtime keeps system messages where they are while it manages the context. Only the request sent to the provider is placed for it:
   - the first message, when it is a system message, is the system prompt;
   - every later text system message stays in place as a user turn wrapped in `<system-reminder>`, merged with the user message next to it.

   Two original user messages are never merged, and a merged message keeps the original's transcript identity, so compaction still finds its boundary. Structured system content that is not text keeps the old behaviour and moves to the front. After a compaction, system messages are still moved to the front; compaction invalidates the cache anyway.
4. Ranking decides which connector operations fit the tool budget, not their order: the selected tools are listed in a fixed order.
5. The Claude adapter builds `system` only from the system messages that open the request. A later system message stays where it is, as a `<system-reminder>` text block in a user turn. Consecutive user turns, such as tool results followed by a reminder, are merged into one.
6. The Claude adapter marks three cache breakpoints, within Claude's limit of four:
   - the last tool;
   - the end of the system prompt;
   - the last block of the request.

   The last one lets the next round of a turn read everything sent before it.

## Consequences

- A DigitalHuman conversation repeats the same system prompt on every run, so the system prompt and the earlier history can come from every provider's cache.
- Within a turn, each round reads the previous rounds from Claude's cache.
- Per-run guidance, the skill catalog and loop notices now reach the model as reminders in a user turn, close to the current request. Models trained on such reminders follow them; regression evaluations should watch for drift.
- Measured on a DigitalHuman on DeepSeek V4 Flash, the second and later messages of a conversation went from 384 cached input tokens of about 11,300 to about 6,700. The first round of a tool-calling turn rose from 512 to 6,656. The rest is the current turn: the turn context, the skill catalog and the loop notices.
- A tool list that changes, for example when a skill is activated and adds tools, still breaks the cache from the tools onward. Loading tools on demand without changing the list is a later step, as is moving guidance that never changes into the system prompt.
- Claude writes to the cache at a premium of 25% on the cached part. A conversation that never sends a second request pays that premium without a read. Agent turns almost always send a second request.
