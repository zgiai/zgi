# Operation plan execution evidence

`update_plan` accepts an outcome contract, a phase plan, or both. When both are
present, the runtime preserves the phase IDs, action bindings, completion modes,
and outcome associations. Set `outcome_id` on every phase to associate phases
explicitly. For compatibility, equally sized arrays with no `outcome_id` pair
by position. Partial, unknown, or uncovered associations are rejected.

An outcome-only revision preserves existing phase bindings. Model-provided
`completed` statuses and notes do not prove that an outcome completed. The
runtime reconciles the contract against its own action attempts and successful
invocation ledger:

- Tool phases match the expected skill, tool, and authorized target. Repeated
  identical action bindings require phase correlation so one call does not
  satisfy two separate phases. A later failed matching attempt cannot be
  hidden by an older successful call.
- Compatibility tool phases can supply successful `evidence_refs` instead of
  an upfront action binding. The runtime resolves those references to actual
  calls; unknown references and references to different actions cannot create
  a binding.
- Explicit `final_answer` or `non_tool` phases complete when the final answer
  is submitted and their dependencies are satisfied. They cannot replace
  required tool evidence or stronger domain effect acceptance.
- Existing domain effects and server-projected external Action bindings remain
  authoritative. Generic tool success does not bypass resource targets,
  required effects, or external Action identity and execution-epoch checks.

Stopping a message persists a cancelled plan and cancels unfinished phases and
outcomes while preserving work already completed. A delivered final answer may
explain a failed action: message delivery can be `completed` while the plan and
its verification are `failed`. If required tool work has no binding yet and
the last business call failed, the runtime records a task-level failure while
leaving that outcome unverified; it does not invent a phase association.
A task with missing evidence and no terminal failure stays incomplete;
it must never receive verification `pass` merely because an answer was written.

The service tests include synthetic joint-snapshot, target, cancellation,
partial execution, and reference-validation regressions. An optional recorded
metadata replay can be supplied outside the repository using
`ZGI_OPERATION_PLAN_REPLAY_FILE` when running
`TestOperationPlanRecordedReplay`; `ZGI_OPERATION_PLAN_REPLAY_OUTPUT` saves its
reconciled metadata. Recorded customer or production data must remain outside
the source tree.
