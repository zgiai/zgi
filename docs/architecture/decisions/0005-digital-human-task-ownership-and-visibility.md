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

- `requester`: the user subject or channel conversation that asked for it;
- `origin`: the surface, conversation, and message where it was proposed;
- `confirmed_by`: the principal that approved the proposal;
- `visibility`: `requester`, `conversation`, `workspace`, or `restricted`;
- an optional business scope, using the scope model from ADR 0004.

### Visibility is enforced in one query layer

A single server-side visibility function serves REST handlers and runtime task tools. It uses the source trust levels defined in ADR 0004:

| Viewer | Visible tasks |
|---|---|
| Task console user with task management permission | All tasks of the DigitalHuman |
| `authorized_operator` or `workspace_member` in a conversation | Tasks they requested, plus `workspace` tasks |
| `channel_participant` | Tasks originated in the current channel conversation, plus tasks whose scope that conversation resolves to |
| `external_end_user` | Only tasks they requested |

`restricted` tasks are visible only to their requester and to the task console.

### Awareness without disclosure

Task tools return a task the viewer cannot see only as occupancy: its time window, lifecycle status, and priority band. They never return its title, content, requester, or result. This lets the DigitalHuman reason about its workload and schedule conflicts without leaking another requester's work. Occupancy is disabled for `external_end_user` runs by default.

### Mutation authority is separate from visibility

- A requester may change, pause, resume, or cancel tasks they requested.
- Other changes require task management permission in the workspace.
- Scheduled runs keep their current restriction: they may update only the task being executed, and only to `waiting` or `blocked`.
- Proposal and confirmation remain mandatory for every conversational task creation, and `confirmed_by` is recorded.

### External contracts stay stable

The external API task endpoints keep their meaning: they return and mutate tasks requested by the supplied `user`. The console's shared view and personal view remain available as filters over the same data.

### Migration

The existing namespaces map deterministically:

| Current `owner_kind` | New requester | New visibility |
|---|---|---|
| `user_subject` | The previous owner subject | `requester` |
| `conversation` | The previous owner conversation | `conversation` |
| `digital_human` | The creating account (`created_by`) | `workspace` |

The legacy ownership columns stay readable for one release so the previous build can still serve traffic during rollback. The new visibility layer ships behind a runtime switch, and the existing ownership checks stay authoritative until the switch is enabled.

## Consequences

- The DigitalHuman can answer for its whole workload while each viewer still sees only what they are entitled to.
- Task access logic moves from namespace predicates to one visibility function, which needs exhaustive tests across every surface and trust level.
- Runtime task guidance changes from "this is only the current user's list" to describing visible tasks and occupancy.
- A future approval center can attach approval requests to tasks by identifier and reuse `requester`, `origin`, and `confirmed_by` as the approval source. A future artifact library can link task results to artifacts the same way. Both are outside this decision.
- Per-user instances for external integrations are outside this decision. If they are introduced, instance tasks are owned by the instance, and this visibility model applies within it.
