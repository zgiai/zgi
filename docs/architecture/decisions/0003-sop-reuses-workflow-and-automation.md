# ADR 0003: SOP Reuses Workflow and Automation

- Status: Accepted
- Date: 2026-08-18

## Context

DigitalHumans need repeatable operating procedures, manual and event-driven triggers, pauses, approvals, retries, and scheduled execution. The platform already has Workflow and Automation mechanisms for those responsibilities.

## Decision

An `SOPSpec` describes business intent, inputs, outputs, preconditions, risk, completion criteria, and evaluation rules. Its executable form references a published Workflow or Automation revision instead of introducing another orchestration engine.

Every trigger is normalized into an `InvocationEvent`. A resolver pins the referenced SOP and executable revision into the run's immutable `RunSpec`. Scheduled execution enters through Automation and enqueues the same event contract used by manual and channel triggers.

## Consequences

- Existing pause, approval, retry, history, and scheduling infrastructure is reused.
- SOP publication must validate parameter mappings and referenced revisions.
- A run never follows a mutable “latest” Workflow after its `RunSpec` is sealed.
- SOP-specific orchestration features should first be expressed as Workflow or Automation capabilities.
