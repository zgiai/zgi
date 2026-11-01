# ADR 0024: A DigitalHuman Continues After a Connector Confirmation

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0023 (an operations specialist ready for production)
- Amends: the console resolution of a DigitalHuman connector confirmation

## Context

A DigitalHuman's connector call that needs a confirmation pauses the turn. The person decides on the card, and the decision executes the operation exactly once. Before this decision, the turn ended there:

- **No answer.** Nothing re-entered the model after the decision. The chat showed only "connector operation completed", and the person never learned what the operation found, for example whether `nginx.service` is active.
- **No result for the model.** The resolution keeps provider output out of chat metadata, so even a later turn could not read it.
- **A run that never ended.** On success the console path moved the run back to running and left it there (fixed separately; the run now ends).

An operation that needs no confirmation already gives its result to the model as the tool result of the call, in the same turn. Only the confirmed path lost it.

## Decision

### 1. The console decision resumes the same turn

- **Where:** `POST /connectors/runtime/confirmations`, the console and WebApp route, when the frozen origin is a DigitalHuman run.
- **In the request**, in one transaction:
  - the resolution is recorded on the message, as before;
  - the message is claimed for streaming instead of completed;
  - the run moves from waiting to running, pinned by the same owner, host, version, conversation, message, and receipt checks as before.
- **After the request returns**, the DigitalHuman runtime resumes the turn in the background:
  - it restores the run's sealed configuration, as a platform tool approval does;
  - it replaces the paused tool result of that call with the operation's result;
  - it adds a continuation note and runs the model loop;
  - the run then completes, fails, or waits again (for example on a second confirmation).
- **The browser** already reconnects to the conversation's stream after a decision, so the answer streams into the same message.

### 2. The model sees what an unconfirmed call would have shown it

- **Same projection.** The replaced tool result is the connector's model-visible result for that operation, the same value a call without confirmation returns. No new field, provider payload, or credential is exposed.
- **The continuation note repeats the result**, bounded to 12,000 characters. The turn's stored transcript keeps the paused tool result of the call, so later passes and later turns read the outcome from the note.
- **Cancelled and failed calls** give the model their state and error code. The note says the operation was not, or did not successfully, run, and must not be retried with the same arguments in this turn.
- **Chat metadata is unchanged.** Provider output still never enters message metadata or the stream events; it lives only in the in-memory continuation and the turn's transcript, like an unconfirmed call's result.

### 3. Other paths keep their behavior

- **Channel replies** keep their text outcome and end the run.
- **The external API** keeps its synchronous decision endpoint and ends the run.
- **Chat and Agent confirmations** are not changed.

Amended 2026-10-31: channel replies and the external API now continue the turn the same way. A channel run ended `succeeded` with an empty answer after its operation ran, so the operation was taken for the task. A channel receives the continued turn's answer or its next confirmation prompt; a repeated reply to a decided confirmation runs and delivers nothing. The API endpoint still answers synchronously with the decision, and the run then ends with the turn.

## Consequences

- A confirmed operation reads like any other tool call: the specialist reports the service state, explains a failure, or asks for the next confirmation.
- The continuation runs outside the request. If the process stops mid-turn, the message's runtime lease expires and the message ends in error; the operation itself already ran exactly once and is not repeated.
- Channel replies were brought into the same continuation (see the amendment above); a channel receives the model's answer instead of the fixed outcome text.
