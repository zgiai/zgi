# ADR 0044: A Unified Entry for Skill Tools (Experiment)

- Status: Not adopted (measured 2026-10-30); the switch stays off by default
- Date: 2026-10-30

## Context

A ZGI skill brings its own business tools. When the native loop activates a skill, those tools are added to the tool list of the request.

- Providers place tools before the messages, so a tool added in the middle of a turn changes the request's prefix. The rest of that turn reads only the system prompt and the earlier tools from the cache.
- Measured on a three-turn conversation that generated files: after activation, the cached prefix fell from about 12K to about 3K tokens, and about 9K tokens a round were billed in full.
- Codex, Claude Code and DeepSeek Harness keep the tool list fixed for a session. A skill is instructions that the model reads when it needs them, and the instructions are appended to the conversation. Its work goes through tools the session already has, such as a shell.

A unified entry would give ZGI the same property. It is close to the retired `call_skill_tool` protocol, which carried every skill and tool pair in one large schema. Native per-tool schemas also constrain the model's arguments while it generates them, which a free-form input does not. Whether the cache gain outweighs any loss in accuracy has to be measured.

## Decision

1. **A switch selects how skill tools are offered.**
   - `ZGI_SKILL_TOOL_DISPATCH=native` (the default) keeps today's behaviour.
   - `ZGI_SKILL_TOOL_DISPATCH=unified` offers the business tools of activatable skills through one tool.
2. **The unified tool is `zgi`.**
   - It takes `command`, the skill ID and tool name separated by a space (`file-generator generate_file`), and `input`, the tool's arguments as a JSON object.
   - The input is not shell text, because tools such as file generation take whole documents.
   - Its definition is small and fixed. It does not list skills, tools or their schemas.
3. **Activation adds instructions, not tools.**
   - Activating a skill appends its instructions, followed by its tools: name, description and input schema.
   - The tool list of the request does not change. `read_skill_reference` is offered from the first round when any candidate skill has references.
   - Calling a tool of a candidate skill that is not active activates the skill, as a direct call does today.
4. **Some tools stay native:**
   - resident tools (ADR 0043);
   - memory and other runtime tools;
   - connector operations, whose schemas and target bindings the server enforces;
   - the protocol tools.

   These are fixed when the turn starts.
5. **A unified call runs as the native call it names.** The runtime translates it before handling it, so governance, approvals, read-only prefetching, traces and the timeline are unchanged. The conversation keeps the model's own `zgi` call. A command that names no available tool returns a recoverable error that says how to find the tool.
6. **The switch is decided by measurement.** The runtime and memory suites run in both modes, along with multi-turn conversations that use activatable skills, on:
   - pass rate;
   - argument errors and retries;
   - model calls;
   - input tokens and the cached share.

   This ADR is accepted, and `unified` becomes the default, only if accuracy does not drop.

## Results

Both modes ran on the same build and model (deepseek-v4-flash).

- The skill conversations used the only activatable skill this workspace allows an Agent: file generation, including a 40-line document and a docx export.
- The memory suite was not run. Its Agent has no activatable skill, so both modes send it the same requests.

| Measure | native | unified |
|---|---|---|
| Runtime suite | 327/327 | 327/327 |
| Runtime suite: model calls per turn | 1.98 | 2.02 |
| Runtime suite: prompt tokens per turn | 9,457 | 10,565 |
| Runtime suite: cached share | 91.1% | 84.2% |
| Skill conversations (8, 18 turns): completed | 18/18, 16 files | 18/18, 16 files |
| Skill conversations: tool errors | 0 | 0 |
| Skill conversations: model calls | 45 | 42 |
| Skill conversations: uncached input | 87,849 | 157,984 |
| Skill conversations: cached share | 81.3% | 61.4% |

Accuracy did not drop: no argument errors, the same files, and slightly fewer model calls. The cache went the other way.

- In native mode an active skill's tool schemas sit in the tool list, in the request's prefix. Every conversation of the Agent shares that prefix, so after activation about 9.7K tokens of a round came from the cache.
- In unified mode the same schemas travel with the skill's instructions, after the conversation's own messages. Each conversation pays for them in full the first time: the round after activation read about 3.8K tokens from the cache and paid about 7.6K.
- Unified mode only saves the history after the activation point of that turn. In these conversations that history is smaller than the schemas, so the trade is a loss.

`unified` therefore stays off. Keeping skill tool schemas in the cached prefix is worth more than keeping the tool list unchanged within a turn.

### Second round: signatures instead of schemas

Unified mode then listed each tool as a signature, such as `generate_file(content: string, format: txt|md|..., filename?: string)`, with the descriptions of its top-level fields. A rejected input still returns the full schema. `generate_docx` went from a 15K-character schema to a signature of about 1K. The same build also carried two changes that apply to both modes (ADR 0043):

- skills used in the last three turns start active;
- shorter docx and pptx schemas.

| Measure | native | unified |
|---|---|---|
| Runtime suite | 327/327, cache 89.3%, 9,219 prompt tokens | 327/327, cache 88.7%, 10,112 prompt tokens |
| Skill conversations: completed | 18/18, 16 files | 18/18, 16 files |
| Skill conversations: model calls | 45 | 45 |
| Skill conversations: uncached input | 80,913 | 88,680 |
| Skill conversations: cached input | 412,544 | 268,288 |
| Skill conversations: cached share | 83.6% | 75.2% |

Signatures cut unified mode's uncached input from 157,984 to 88,680 tokens, close to native mode.

- Unified mode still pays about 10% more uncached input.
- Its total input is a third smaller, so where a cache read costs a fraction of a miss, the two are close and unified mode may be slightly cheaper.
- Accuracy was the same.

The gap is too small to justify a second way of calling tools, so `native` stays the default. Unified mode is worth measuring again for Agents with many activatable skills, where its fixed tool list matters more.

## Consequences

- In `unified` mode a conversation's tool list stays the same from its first round, so activation no longer invalidates the cache.
- The model no longer gets per-tool argument constraints for activatable skills. Invalid input is caught by server-side validation and costs a retry.
- Both modes have to be maintained while the switch exists. It can be removed once the next direction is measured.
