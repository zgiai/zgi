# ADR 0046: Retiring the Protocol Loop

- Status: Accepted
- Date: 2026-10-30

## Context

The skill loop ran two loops.

- **The native loop** offers each tool as a function and takes the final answer as ordinary assistant content. Every new turn of an Agent, the AI chat and a DigitalHuman uses it.
- **The protocol loop** came first. The model called meta tools:
  - `load_skill` to read a skill;
  - `call_skill_tool` to run one of its tools;
  - `submit_intermediate_answer` and `submit_final_answer` to show and end its work.

  It had a variant without planning tools (`legacy_tool_chat`) and one with protocol tools only.

Only messages persisted with the `agent_loop` or `legacy_tool_chat` execution mode, or with none, still reached the protocol loop, and only when such a turn was continued after an approval, a question to the user or a client action. The protocol loop accounted for most of the branches in `Run` and the handlers, a large set of tool builders in the skills package, and a third of the loop's tests.

## Decision

1. **Every tool loop is the native loop.**
   - `RunRequest` loses `NativeAgentLoop`, `LegacyToolChat`, `ProtocolToolsOnly` and `PreferExplicitFinalAnswer`.
   - A terminal-only continuation stays, and runs in the native loop.
2. **Persisted protocol modes continue natively.**
   - `agent_loop` and a missing mode restore as `native_agent_loop`; `legacy_tool_chat` restores as `native_tool_loop`.
   - The history of such a turn is replayed as it is. Its old meta-tool calls are past messages, and the native loop rejects any new call to them.
3. **The protocol code is removed.**
   - **The loop:** its system messages, its tool lists, the explicit final answer, restored skill-load state, `load_skill`, intermediate answers, and the legacy tool-chat continuation note.
   - **The skills package:** the builders of `load_skill`, `call_skill_tool`, `submit_intermediate_answer` and `submit_final_answer`, and the metadata prompt.
   - **Kept:** `call_skill_tool` stays the internal form a native call is dispatched in; it is never offered to the model.
4. **Tests follow the code.**
   - Tests of the protocol behaviour are removed.
   - Tests of behaviour that remains (workflow takeover and workflow events, client-action and approval continuations, ownership loss) now drive the native loop: direct tool calls, and scripted responses replayed as streams.

## Consequences

- The loop has one path. `Run` is shorter, and the next step, moving its per-call state into a struct, no longer has to carry two protocols.
- A continued turn that began in the protocol loop resumes in the native loop. It sees native tools and answers in ordinary content. Turns old enough to have no persisted mode are rare.
- Net removal of about 6,400 lines: 1,475 of production code across the skill loop, its service and the skills package, and 4,900 of tests of the retired protocol.
