# ADR 0005: DigitalHuman Task Ownership and Visibility

- Status: Proposed
- Date: 2026-10-19

## Context

DigitalHuman tasks are stored as automation tasks with `owner_kind` set to one of three namespaces:

- `digital_human`: shared tasks managed from the task console;
- `user_subject`: tasks created from console, API, or authenticated WebApp conversations, private to one account or API end user;
- `conversation`: tasks created from one external channel conversation.

The namespaces are disjoint. During a conversation, the DigitalHuman's task tools can reach only the namespace of the current user or channel conversation, and its instructions forbid presenting that view as its complete todo list. The DigitalHuman cannot see its own workload, detect scheduling conflicts, or prioritize across requesters. Ownership is used as the visibility mechanism, which conflicts with the DigitalHuman being one independent subject (ADR 0001).

## Decision

### The DigitalHuman owns every task

Every DigitalHuman task is owned by its DigitalHuman. Each task records:

- `requester`: the user subject or channel conversation that asked for it. The existing owner columns keep this role, so no requester data is duplicated;
- `origin`: the surface, conversation, and source reference where it was proposed. These are the existing origin and source fields;
- `confirmed_by`: the principal that approved the proposal, and when;
- `visibility`: `requester`, `conversation`, `workspace`, or `restricted`.

Business scopes for tasks, using the model from ADR 0004, are left for a later decision.

### Visibility is enforced in one query layer

A single server-side visibility function serves REST handlers and runtime task tools. It uses the source trust levels defined in ADR 0004:

| Viewer | Visible tasks |
|---|---|
| Task center user with DigitalHuman update permission (manager) | All tasks of the DigitalHuman |
| Other task center users | `workspace` tasks, plus shared tasks they created |
| Workspace member in a console conversation or the personal task view | Tasks they requested, plus `workspace` tasks |
| `channel_participant` | Tasks originated in the current channel conversation |
| `external_end_user` | Only tasks they requested |

`restricted` tasks are visible only to their requester and to task center managers. Each task read reports `can_change`, so a visible `workspace` task requested by someone else is shown read-only.

### Awareness without disclosure

Task tools return a task the viewer cannot see only as occupancy: its time window, lifecycle status, and priority band. They never return its title, content, requester, or result. This lets the DigitalHuman reason about its workload and schedule conflicts without leaking another requester's work. Occupancy covers committed open tasks only, never unapproved proposals, and is never returned to `external_end_user` runs.

### Mutation authority is separate from visibility

- A requester may change, pause, resume, or cancel tasks they requested. The requester of a shared task is the account that created it.
- Task center managers may change any task. Only managers may change a task's visibility.
- Scheduled runs keep their current restriction: they may update only the task being executed, and only to `waiting` or `blocked`.
- Proposal and confirmation remain mandatory for every conversational task creation, and `confirmed_by` is recorded.

### External contracts stay stable

The external API task endpoints keep their meaning: they return and mutate tasks requested by the supplied `user`. The console's shared view and personal view remain available as filters over the same data.

### Migration

The existing namespaces map deterministically:

| Current `owner_kind` | Requester (kept in the owner columns) | Visibility |
|---|---|---|
| `user_subject` | The previous owner subject | `requester` |
| `conversation` | The previous owner conversation | `conversation` |
| `digital_human` | The creating account (`created_by`) | `workspace` |

Visibility is nullable. Rows written by a build without visibility support resolve their visibility from the owner kind using the same mapping, so a rollback can keep serving traffic. Changes stay limited to the requester namespace except for task center managers. Delivery and scheduled execution keep reading the owner columns. The visibility layer ships behind `ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED`, which defaults to off. The existing ownership checks stay authoritative until it is enabled.

## Consequences

- The DigitalHuman can answer for its whole workload while each viewer still sees only what they are entitled to.
- Task access logic moves from namespace predicates to one visibility function, which needs exhaustive tests across every surface and trust level.
- Runtime task guidance changes from "this is only the current user's list" to describing visible tasks and occupancy.
- A future approval center can attach approval requests to tasks by identifier and reuse `requester`, `origin`, and `confirmed_by` as the approval source. A future artifact library can link task results to artifacts the same way. Both are outside this decision.
- Per-user instances for external integrations are outside this decision. If they are introduced, instance tasks are owned by the instance, and this visibility model applies within it.
