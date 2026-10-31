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
2. A DigitalHuman puts its per-run guidance in `TurnContext`, in the first run and in every continuation (connector confirmation, user input, task approval):
   - the current time;
   - per-user connector and approval notes;
   - channel context and preview guidance;
   - the memory files and the index of recent artifacts.

   The rules for memory, tasks and artifacts stay the same from run to run of a conversation, so they go in the system prompt (decision 9).
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
7. The tool list grows only at its end within a run. A tool keeps the position it had in the round where it first appeared, and a tool added later, such as one from a newly active skill or `read_skill_reference`, goes after the tools already sent.
8. `activate_skills` keeps offering every candidate the model has been shown, including active ones, so its definition does not change when a skill is activated. Activating an active skill again reports it as already active.
9. Guidance that changes only with the configuration goes at the end of the system prompt, not after the history:
   - the native loop's rules;
   - an Agent's memory rules and its builder's memory instructions, passed in `RunRequest.SystemPromptAdditions`;
   - a DigitalHuman's memory, task and artifact rules, returned by its runtime providers as `StableGuidance`.

   Sent after the history, this guidance was the same in every turn but never part of a prefix the provider had seen, so every turn's first request paid for it in full. Loops other than the native loop still send these additions as system messages.
10. A continuation of a DigitalHuman run sends the requests its first run sent, extended:
    - every continuation prepares the time, connector notes, channel context, memory and tasks with the first run's code;
    - a task-approval continuation keeps the task-change tool but refuses its calls, instead of removing it from the tool list;
    - a continuation within 15 minutes of the run's start keeps the run's start time in its turn context;
    - page context is restored only for a turn that sent some.
11. When every candidate skill fits in the skill directory, the directory lists them in skill ID order and goes in the system prompt, before the native-loop rules. Only when some must be left out does the ranking for the current question decide which are shown, and that directory stays with the turn. `activate_skills` offers the candidates in the directory's order, so its definition is also the same from turn to turn.

## Consequences

- A DigitalHuman conversation repeats the same system prompt on every run, so the system prompt and the earlier history can come from every provider's cache.
- Within a turn, each round reads the previous rounds from Claude's cache.
- Per-run guidance, the skill catalog and loop notices now reach the model as reminders in a user turn, close to the current request. Models trained on such reminders follow them; regression evaluations should watch for drift.
- Measured on a DigitalHuman on DeepSeek V4 Flash, the second and later messages of a conversation went from 384 cached input tokens of about 11,300 to about 6,700. The first round of a tool-calling turn rose from 512 to 6,656. The rest is the current turn: the turn context, the skill catalog and the loop notices.
- Before decisions 7 and 8, activating a skill removed `activate_skills` from the end of the tool list. DeepSeek renders tools before the messages, so the round after an activation read only 6,528 cached tokens. With them, the tool list is the same in every round of the turn, and that round reads 11,648; rounds 3 and 4 read 13,824 and 14,336.
- A tool a skill adds still lands after the existing tools, which costs the part of the prefix after them in that one round.
- Decision 9, measured on DeepSeek V4 Flash with four short questions in one conversation:

  | Turns 2 to 4, per request | Before | After |
  |---|---|---|
  | Agent, input tokens not read from the cache | about 1,800 | about 950 |
  | DigitalHuman, input tokens not read from the cache | about 3,050 | about 520 |
  | DigitalHuman, input tokens read from the cache | about 4,800 | about 7,300 |

  What is left uncached is the newest exchange, the turn boundary, the skill catalog, and, for a DigitalHuman, the time and the memory files.
- Decision 11, measured on an Agent with three skills on DeepSeek V4 Flash, four short questions in one conversation: turns 2 to 4 left 150 to 275 input tokens uncached per request instead of about 950.
- Decision 10, measured on a task-approval continuation of a DigitalHuman on DeepSeek V4 Flash: its first request read 10,496 input tokens from the cache instead of 5,760, and left 1,968 uncached instead of 5,979. The rest is what the continuation adds: the tool call, its result, the skill catalog and the continuation note.
- A DigitalHuman continuation (user input, tool approval, task approval, connector confirmation) prepares the same capabilities as its first run, so it sends the same system prompt and tools (decision 10).
- Claude writes to the cache at a premium of 25% on the cached part. A conversation that never sends a second request pays that premium without a read. Agent turns almost always send a second request.
