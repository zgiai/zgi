# ADR 0036: Read-Only Tool Calls of a Round Run Together

- Status: Accepted
- Date: 2026-10-29

## Context

A model often asks for several tool calls in one round: a few reads to compare, a few calculations, several files. The agent loop ran them one after another. Around each call, the loop does work that depends on order:

- events and the process timeline;
- per-skill call counts and budgets;
- governance and policy checks;
- plan gates;
- checkpoints of the transcript;
- stopping the round when a call pauses the turn.

Reads that wait on a slow source, such as a file, a knowledge base or a database query, therefore added up.

The runtime had no shared notion of a call that changes nothing:

- A governance manifest can declare `effect: read`, but pure tools such as the calculator and the time skill had no manifest. A manifest also brings asset governance and audit, which they do not need.
- Crash recovery (ADR 0035) needed the same notion.

## Decision

1. A tool is read-only when either of these holds:
   - its governance manifest declares `effect: read` and no external side effect;
   - it belongs to a system skill whose SKILL.md declares `side_effects: none`.

   A member's own skill cannot declare `side_effects: none`, and a connector operation never counts, because its dispatch keeps a ledger per call.
2. The calculator and time skills declare `side_effects: none`. `skills.ToolIsReadOnly` and `skills.SystemSkillToolIsReadOnly` answer the question for the agent loop and for crash recovery.
3. When a round of the native agent loop has at least two read-only skill tool calls, they start at once, at most four at a time, before the round is handled. A call is started early only if the sequential pass would run it as it stands:
   - its skill is active;
   - the organization's policy allows the skill;
   - no plan update after user input holds it back;
   - it did not fail the same way earlier;
   - it is not an Agent workflow run.
4. The round is then handled exactly as before, one call at a time in the model's order. A call that was started early waits for its result where it would have run. Results reach the model, the transcript and the events in the model's order.
5. A call started early that the round then does not reach is a read whose result is dropped. This happens when an earlier call pauses the turn.

## Consequences

- A round of several slow reads takes about as long as the slowest of them.
- With `side_effects: none`, crash recovery (ADR 0035) also resumes a turn stopped during a calculator or time call.
- Runtime tools, such as a DigitalHuman's memory and task reads, still run one at a time. They can join when the runtime tool declares that it is read-only.
- A read started early but never reached costs a call. Reads are cheap, and the case is rare.
