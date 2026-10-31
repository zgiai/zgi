# ADR 0049: Model-Owned Plans

- Status: Proposed
- Date: 2026-10-31

## Context

The runtime's operation plan was built for the runtime to supervise the model. It has about 6,000 lines across the service and skill loop.

- **Data:** phases with `expected_action`, `evidence_refs`, `completion_mode` and runtime bindings, and Outcomes with acceptance effects. Around them sit evidence, effect and attempt ledgers and a completion verification record.
- **`update_plan`:** accepts one of three exclusive payloads (`plan`, `outcomes`, `phase_updates`) in a schema of about 3.7 KB. Its description tells the model to call it "only when the user's goal or the required outcomes change", because "the runtime tracks ordinary progress".

In use it does little:

- **Rarely called:** of 3,161 turns in the last two days of runtime evals, 59 called `update_plan` (2%). Of 110 stored plans, 85 were model-advisory; 4 had server Outcomes.
- **Default plan is empty of content:** a plan is created up front only on the console sidebar, as one phase that repeats the user's request. Agents and DigitalHumans get a plan only if the model calls `update_plan`, and the tool is offered only when a non-resident skill is resolved.
- **Two owners, neither reliable:**
  - The runtime advances a phase only when a tool call matches its `expected_action`, so a phase without one never completes from tool work.
  - The model cannot mark a tool phase completed without citing ledger evidence, or the call is rejected.
- **Invisible:**
  - The web UI does not show plans.
  - The model sees the plan only in the `update_plan` result, and again only on approval, question and similar continuations.
- **Unused verdict:** at the final answer the runtime computes that required work is incomplete and records it in metadata. It does not tell the model or the user.
- **Forcing rules cost rounds:**
  - After a user clarification, business tools are refused until `update_plan` succeeds; its own comment notes a blocked call and two extra rounds.
  - Continuation prompts push the model to call `update_plan`.
- **Built for built-in skills:** acceptance effects, evidence keys and result summaries are hard-coded for console navigation, file and Agent-management tools, so custom skills and connectors get none of it. Intent branches the server never sets, duplicate phase logic in the service and skill loop, and fields nothing reads remain.

Mature agents use a plan differently. Claude Code's TodoWrite and Codex's `update_plan` are the same idea:

- **Shape:** a flat checklist of steps with a status, replaced whole on each call, with at most one step in progress.
- **Ownership:** the model owns it, and the runtime does not validate it.
- **Visibility:** the user sees it live.
- **Purpose:** it keeps the model from dropping steps in long work, shows progress, and lets interrupted work resume.

The runtime's own checks belong where an outside system must confirm a result, such as connector operations, and are separate from that checklist.

A baseline of eight multi-step scenarios (below) shows what to expect from the change. DeepSeek and Kimi passed all of them, three runs each, with the current runtime; they called `update_plan` in 3 of 27 and 2 of 28 turns. On tasks of this size, today's models do not drop steps without a plan. The gains are therefore a plan the user can follow, a checklist for long runs such as DigitalHuman tasks, and the removal of supervision code that has had little effect, not higher accuracy on short tasks.

## Decision

1. **`update_plan` is a checklist the model owns.**
   - The input is `{explanation?, plan: [{step, status}]}`, where status is one of `pending`, `in_progress`, `completed` or `skipped`. Each call replaces the whole list; at most 16 steps and at most one in progress.
   - The description asks for a plan when a request has three or more steps or several deliverables, an update as each step completes, and no plan for a single step or a direct answer.
   - No evidence references, expected actions or completion modes. The runtime does not reject a status the model sets.
2. **Runtime acceptance is separate from the plan.**
   - Connector acceptance follows the operations that actually ran. A turn cannot end while an executed operation awaits its native outcome, as `connectorTerminalStateDecision` already decides from the invocations. Plan steps no longer name connector operations, so the early end once every planned operation is confirmed, and the rule that an executed connector plan cannot be reordered, go with them.
   - An approved call runs as it was frozen, as today; it is no longer bound to a plan phase afterwards.
   - The evidence, effect and attempt ledgers stay as the record of what succeeded, for audit and resume. They no longer drive plan steps.
   - Model-created Outcomes, server-authority phases, `expected_action` matching, and the hard-coded effect maps for built-in skills are removed.
3. **The model keeps seeing its plan.**
   - The latest plan stays in the conversation as the `update_plan` result.
   - When context compaction removes it, the compacted request adds the current plan back.
   - When the model writes a final answer while steps are still `pending` or `in_progress`, the runtime returns once with a short reminder listing them. The model then continues, or marks them `skipped` and says why. This happens at most once per turn.
4. **The user sees the plan.** Each accepted `update_plan` is streamed as a plan event and stored with the turn. The AI chat, Agent and DigitalHuman views show the checklist and its progress.
5. **Every run that can call tools can plan.** `update_plan` is offered to Agents (console and API), the AI chat and DigitalHumans, including task runs, whenever the run has a tool beyond the resident ones. Runs that only answer from the conversation do not get it.
6. **The forcing rules go.** That covers:
   - the refusal of business tools after a clarification until a plan update;
   - the continuation text that asks for `update_plan`;
   - the default one-phase plan that repeats the request.
7. **Across turns:** an unfinished checklist is the open task state of ADR 0047. A stopped turn keeps its plan, with the in-progress step left as it was.

## Validation

A `planning` category in the runtime eval measures multi-step work before and after the change, on DeepSeek and Kimi, with one Claude pass:

Baseline before the change (three runs, current runtime): DeepSeek and Kimi passed all eight scenarios; plans were called in 3 of 27 and 2 of 28 turns.

- **Several deliverables:** three unrelated results and a file that holds them.
- **Many items:** eight discounted prices and their total, in a CSV.
- **Dependent chain:** each step uses the previous step's result, then a CSV.
- **Six comparisons:** six growth rates and the fastest and slowest.
- **Revision mid-task:** a price changes in the second turn, so the result and the file are redone.
- **Clarification:** missing figures are asked for, then the calculation and a summary file are completed.
- **Twelve items with a tool each:** twelve day counts, each a date calculation, then a table file.
- **Three files:** a CSV, a Markdown note and a JSON file in one request.

Measured per model:

- the conversations that pass and the expected values present, which counts dropped steps;
- files produced;
- model calls, latency and tokens;
- the share of multi-step turns that called `update_plan`, read from the stored model invocations.

The full runtime suite runs alongside. The change is accepted when:

- no group, the planning group included, passes less often than before;
- most planning-group turns write a plan the user can see;
- model calls and tokens per turn rise by no more than 10% in the planning group, and not measurably elsewhere.

## Consequences

- Most of the operation plan code goes, along with the duplicated phase logic in the skill loop; connector acceptance and the ledgers stay.
- Plans become visible to users and are written on most multi-step turns, which costs a tool call and its tokens on those turns.
- The model, not the runtime, decides whether a step is done. The runtime still confirms connector results, and the reminder at the final answer catches steps left open.
- Plans stored by earlier turns are read as checklists: their phases' `step` and `status` are kept, and the other fields are ignored.
