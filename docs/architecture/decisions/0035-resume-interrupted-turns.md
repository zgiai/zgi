# ADR 0035: Resuming Turns Interrupted by a Stopped Process

- Status: Accepted
- Date: 2026-10-29

## Context

An agent turn runs in one process: an API request goroutine for chat, or an asynq worker for DigitalHuman runs. When that process stops in the middle of a turn, through a deploy, a crash or an OOM kill, nothing finishes the turn.

The pieces for a resume already exist:

- **The transcript is durable.** Every model response is stored before its tool calls run, and every tool result as soon as it exists, as transcript entries of the message.
- **Unfinished calls are visible.** A tool call is recorded in `skill_invocations` as `running` before it executes, so after a stop one can tell which call started and has no result.
- **Leases are swept.** The lease sweep marks a message whose heartbeat stopped `error` with `runtime_lease_expired`, after about 90 seconds.
- **Continuations already rebuild a turn.** They rebuild it from its transcript and same-turn state and run its loop on: user input, approvals, connector confirmation, token budget.

What happened instead: the message stayed an error. A DigitalHuman run became `unknown` after 10 minutes, with "its external side effects may be unknown". The comment on `Execute` explains why runs were never replayed: a stopped run may have done things that must not happen twice.

## Decision

1. The chat runtime gains `RunConfiguredInterruptedTurnResume`. It resumes a message that the lease sweep stopped (`error`, `runtime_lease_expired`) and that is the current leaf of its conversation:
   - It rebuilds the turn the way the token-budget continuation does: the transcript, with unfinished tool calls closed, followed by the same-turn state.
   - It adds a note that the process stopped and the task is resuming.
   - It claims the message with a compare-and-swap from that error back to `streaming`, then runs the loop.
   - It drops the partial answer of the stopped attempt and marks unfinished `skill_invocations` as `interrupted`.
2. A turn is resumed only if every tool call it started without a recorded result is read-only. The tool's governance manifest must declare `effect: read`; a tool that declares nothing does not count. Otherwise the turn is not resumed, because repeating a change could do it twice. The common case passes: a stop while the model was answering or deciding.
3. A turn is resumed at most once. The outcome is recorded on the message under `interrupted_turn_resume`, whether resumed or not resumed with the reason. A turn that stops again during its resume stays an error.
4. DigitalHuman runs resume automatically:
   - `Reconcile` (every 15 seconds) finds running runs whose message is such an error.
   - It resumes each one in the background, with the capabilities of the first run (ADR 0032 decision 10).
   - It settles the run like any continuation: completed with delivery, or waiting.
   - The stale-run sweep no longer marks a run unknown while its message is `streaming`. A live owner holds that message, and the lease sweep catches a dead one.
   - Runs of every surface are considered, including API and WebApp runs, which have no console actor. The resume runs as the first run's caller, run ID included, so its connector access resolves the same way.
5. Interactive chat and Agent API turns are not resumed automatically yet. The client that asked has gone, and a resume needs the caller's own configuration. A later step can offer the same resume to them.

## Consequences

- A DigitalHuman run that a deploy or crash stopped during a model call finishes a few minutes later, instead of becoming `unknown`. Scheduled tasks and channel replies are delivered. Measured locally, the API container was killed 8 seconds into a long answer. The lease sweep stopped the message 2 minutes later, reconcile resumed it 15 seconds after that, and the run completed with the full answer about 3 minutes after the kill.
- A run stopped during a tool that may change something still becomes `unknown`, as before.
- System skills without a governance manifest, such as the calculator, count as not read-only. A stop during one of their calls, which take milliseconds, is not resumed.
- The resumed model sees the closed tool calls and the resume note. Regression evaluations should watch for repeated work.
