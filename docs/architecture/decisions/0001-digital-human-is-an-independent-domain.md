# ADR 0001: DigitalHuman Is an Independent Domain

- Status: Accepted
- Date: 2026-08-18

## Context

A DigitalHuman has a stable name, identity, rules, roles, memory, triggers, and resource bindings. Its lifecycle and ownership differ from an Agent, even though both can use the same model, tool, workflow, and chat runtime capabilities.

## Decision

DigitalHuman is an independent domain aggregate and does not require an Agent record or Agent foreign key. Shared runtime code identifies its owner through a subject-neutral `RuntimeSubjectRef` and receives an immutable `RunSpec` for each execution.

Agent-specific persistence and APIs remain unchanged. Reusable mechanisms should move behind subject-neutral capability contracts only when a DigitalHuman implementation needs them.

## Consequences

- A DigitalHuman can be created, versioned, published, and retired without creating an Agent.
- Authorization and resource binding checks must accept an explicit subject type.
- DigitalHuman migrations and APIs are separate from Agent migrations and APIs.
- Runtime reuse is encouraged, but domain records and lifecycle rules are not shared implicitly.
