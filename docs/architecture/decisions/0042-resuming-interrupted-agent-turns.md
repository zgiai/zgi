# ADR 0042: Resuming Interrupted Agent Turns

- Status: Accepted
- Date: 2026-10-29

## Context

ADR 0035 resumes a turn whose server process stopped in the middle of it. Only DigitalHuman runs use it: their reconcile loop finds the stopped run and rebuilds its capabilities from the run record.

An Agent turn cannot be rebuilt that way:

- Its run configuration comes from the request. That covers the console account, the published web app and its signed-in user, or the API key and external user.
- Background code would have to repeat that authorization without a request.

Every other Agent continuation is already started by the client: approvals, user input, token budget and workflow continuation. A stopped Agent turn therefore stayed `error` with `runtime_lease_expired`, and the only option was to regenerate it from the beginning.

## Decision

1. **A resume endpoint for every Agent surface.**
   - `POST .../conversations/{conversation_id}/messages/{message_id}/resume` exists on the console Agent, the published web app, the external Agent API and the Runtime Workspace app routes.
   - Each builds its run configuration exactly as its other continuations do and calls the ADR 0035 resume.
   - The response is the same SSE stream as other continuations.
2. **The runtime decides.** The rules of ADR 0035 are unchanged:
   - the turn must be the latest of its conversation and must not have been resumed before;
   - every unfinished tool call must be read-only.

   A turn that cannot be resumed is reported as an `error` event with code `interrupted_turn_not_resumable`.
3. **The web chat offers it.**
   - The latest message that stopped with its process shows "Continue" in its error.
   - The error explains that the service restarted.
   - A turn that cannot be resumed shows that it should be regenerated instead.
   - A chat that is open when its turn stops learns it without a reload. When reconnecting fails and the server still reports the turn running, the chat checks the conversation every 10 seconds, for up to 5 minutes, until the lease sweep settles the turn; then it loads the turn and its Continue action (added 2026-10-30).
4. **Not covered.**
   - DigitalHuman apps resume their own runs, so the Runtime Workspace route serves only Agent apps.
   - The console AI chat has no continuation endpoints, and its interrupted turns are still regenerated.

## Consequences

- Agent users and API clients can finish a long task after a deploy or crash, instead of paying for it again from the start.
- Resuming waits for the client. A turn nobody resumes stays as it is.
- External clients need to handle one more state; the API documentation lists it among the recovery states.
