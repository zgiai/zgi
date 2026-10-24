# ADR 0011: Runtime Workspace End-User Plans and Credits

- Status: Accepted
- Date: 2026-10-23
- Builds on: ADR 0010

## Context

ADR 0010 gives every Runtime Workspace end user a points quota: the workspace's limits per rolling window, per-application caps, and an optional quota of their own. Integrators who sell memberships or top-ups to their users must today translate each purchase into a quota of that end user's own, and undo it themselves when the membership ends. They cannot grant points that last beyond a window, cannot tell an end user what remains in a form their product can show, and learn that someone ran out only when a request is refused.

Payments and membership rules belong to the integrator's product. ZGI should not take payments from end users or model what a membership means.

## Decision

ZGI provides entitlement primitives. The integrating server calls them after its own payment or membership change; ZGI enforces them, records them, and reports them.

### Plans

- A Runtime Workspace policy defines plans next to its default limits:

  ```json
  {"end_user_quota": {
     "limits": [{"window": "24h", "points": 20}],
     "apps":   [{"app_key": "app_x", "limits": [{"window": "30d", "points": 100}]}],
     "plans":  [{"key": "pro", "name": "Pro",
                 "limits": [{"window": "24h", "points": 200}],
                 "apps": [{"app_key": "app_x", "limits": [{"window": "30d", "points": 2000}]}]}]}}
  ```

  A plan key is lowercase letters, digits, `_` and `-`, at most 64 characters, unique in the workspace. A plan's `limits` replace the default per-end-user limits; its `apps`, when present, replace the workspace's per-application caps.
- An end user holds at most one plan, with an optional expiry. A plan whose expiry has passed, or whose key the policy no longer defines, no longer applies; nothing needs to run at expiry.
- Limits that apply to an end user, in order: their own quota (own limits or no limit, ADR 0010), else an applying plan, else the workspace defaults.
- The plan is set with the end user's `PATCH`, in the console and through `/api/connect/v1/end-users/{user}`: `{"plan": {"key": "pro", "expires_at": "..."}}`, or `"plan": null` to remove it. The integrating server may set a plan before the end user's first request; the end user is then registered.
- Changes are end-user events `plan_set` (with key and expiry) and `plan_cleared`. `GET /api/connect/v1/plans` lists the workspace's plans.

### Credits

- An end user has a credit balance in points. The integrating server grants credits with an external ID, such as its order ID, which makes the grant idempotent; reusing it for another amount is a conflict (409). A negative grant takes credits back. A positive grant may expire.
- Each grant is a lot. Usage is drawn from unexpired lots, soonest expiry first; what no lot covers is debt, and the next grant pays debt before it adds a lot. Every grant and draw is a ledger entry with the balance after it.
- A run is admitted while the end user is within the per-end-user limits that apply, or has a positive balance. Usage beyond those limits is drawn from credits: a bill is charged the part of it beyond the limit of the window it exceeds most. Credits never pay for per-application caps, and an end user without per-end-user limits (no limits, or an own quota of no limit) never spends them. A limit of 0 points includes nothing, so every point is paid with credits: pay-as-you-go is a plan, or default, with 0-point limits.
- Credits run from the first grant: earlier usage is never charged.
- Bills land after each model call, so settlement is lazy. Before admitting a run over its limits, and whenever credits are read, the end user's unsettled bills are settled in order under a lock on the end user; each settled bill is marked once, so settling again changes nothing. A balance can go below zero by at most the calls in progress; the next run is refused.
- Erasing an end user deletes their lots, ledger, and settlement marks.
- Credits are not money. The organization still pays for all usage; credits only allot part of it to an end user.

### Telling the integrator

- `GET /api/connect/v1/end-users/{user}/entitlement` returns the plan and its expiry, each window's usage, limit, remaining points, and when its oldest usage leaves the window, the credit balance with its unexpired lots, and whether a new run would be admitted (per-application caps aside).
- `GET` and `POST /api/connect/v1/end-users/{user}/credits` list the ledger and grant credits. A grant also registers an end user who has not made a request yet.
- A refused run still returns the catalog's `quota.exceeded` without details: application errors expose no parameters to clients. The integrator reads the entitlement to offer an upgrade or a top-up.
- Webhooks report `quota.near` (80% of a limit), `quota.exceeded`, `credits.low`, and `plan.expired`:
  - one endpoint per Runtime Workspace, subscribed to some of the events, with its signing secret encrypted at rest and shown once when created or rotated;
  - requests carry `X-ZGI-Event`, `X-ZGI-Delivery`, `X-ZGI-Timestamp`, and `X-ZGI-Signature: sha256=<HMAC-SHA256 of "<timestamp>.<body>">`, the scheme of inbox notification webhooks, and reach only public addresses;
  - events are written to an outbox in the transaction that detects them, and a worker on every instance delivers them, leasing rows with SKIP LOCKED and retrying with exponential backoff from 30 seconds, up to 8 attempts;
  - quota events are detected at admission and sent at most once per end user, limit, and window length; `credits.low` when a settlement or take-back brings the balance to the endpoint's threshold or below; `plan.expired` by the worker, once per plan and expiry;
  - erasure deletes an end user's notices and deliveries.

### Console

The quota page manages plans and shows end users per plan. An end user's detail sets their plan and expiry, and lists and adjusts their credits; adjustments are events like any other change.

### Delivery

1. Plans and expiry, in the policy, the end-user registry, enforcement, the console, and the external API.
2. Credits, the entitlement endpoint, and the details of `quota.exceeded`.
3. Webhooks.

## Consequences

- Integrators map a purchase to one call and never undo memberships themselves.
- An end user's own quota stays the strongest override, so a support exception survives plan changes.
- Plans live in the policy, so editing a plan changes every end user on it at once; removing a plan returns its end users to the defaults.
- Credits add a ledger and a settlement step after bills, but nothing changes for organizations' own balances or for end users without credits.
