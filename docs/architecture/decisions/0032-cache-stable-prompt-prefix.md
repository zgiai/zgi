# ADR 0032: A Cache-Stable Prompt Prefix

- Status: Accepted
- Date: 2026-10-28

## Context

Model providers cache a prompt prefix that repeats between requests:

- Anthropic, when the request marks it with `cache_control`;
- OpenAI, DeepSeek and others, automatically.

A cache hit is billed at a fraction of the input price and starts answering sooner. An agent sends the same system prompt, tools and history on every round of a turn and on every turn of a conversation, so most of its input could come from the cache.

Two things prevented that:

1. A DigitalHuman run appended changing guidance to its system prompt: the current time, connector notes, channel context, memory, and task guidance. The time alone made the system prompt differ on every run, so no provider could reuse anything after it.
2. The Claude adapter moved every system message into Claude's `system` field, including those sent after the history, such as turn boundaries, skill instructions, and per-round notices. Claude orders a request as tools, then system, then messages, so any change to them invalidated the whole prefix. No request set `cache_control` either.

## Decision

1. `RunConfig.TurnContext` carries server-owned guidance that changes from run to run. The runtime sends it as one system message after the history and just before the current user message, next to the memory and turn-boundary context. `SystemPrompt` keeps only what is the same on every run: the persona, its rules and its configuration.
2. A DigitalHuman puts all of its per-run guidance in `TurnContext`, in the first run and in every continuation (connector confirmation, user input, task approval):
   - the current time;
   - per-user connector and approval notes;
   - channel context and preview guidance;
   - memory and task guidance.
3. The Claude adapter builds `system` only from the system messages that open the request. A later system message stays where it is, as a `<system-reminder>` text block in a user turn. Consecutive user turns, such as tool results followed by a reminder, are merged into one.
4. The Claude adapter marks three cache breakpoints, within Claude's limit of four:
   - the last tool;
   - the end of the system prompt;
   - the last block of the request.

   The last one lets the next round of a turn read everything sent before it.

## Consequences

- A DigitalHuman conversation repeats the same system prompt on every run, so the system prompt and the earlier history can come from every provider's cache.
- Within a turn, each round reads the previous rounds from Claude's cache.
- Per-run guidance now sits close to the current request, where models also weigh it more.
- Tools that change from turn to turn still break the cache from the tools onward. That happens when connector operations are ranked against the query, or a skill is activated. Loading tools on demand without changing the list is a later step.
- Claude writes to the cache at a premium of 25% on the cached part. A conversation that never sends a second request pays that premium without a read. Agent turns almost always send a second request.
