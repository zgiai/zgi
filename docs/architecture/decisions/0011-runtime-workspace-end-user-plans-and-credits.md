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

- An end user has a credit balance in points, kept as an append-only ledger. The integrating server grants credits with an idempotency key, its order ID; a negative grant takes credits back. A grant may expire.
- A run is admitted while the end user is within the limits that apply, or has credits left. Usage beyond the limits of a window is drawn from credits when the bill settles, so credits pay only for what the plan does not cover. An end user with no limits spends credits from the first point, which models pay-as-you-go.
- As with quotas, bills settle after each model call, so a balance can go below zero by at most the calls in progress; the next run is refused.
- Credits are not money. The organization still pays for all usage; credits only allot part of it to an end user.

### Telling the integrator

- `GET /api/connect/v1/end-users/{user}/entitlement` returns the plan and its expiry, each window's usage, limit, remaining points and reset time, and the credit balance.
- A refused run's `quota.exceeded` carries the window, its limit, when it resets, and the balance, so the integrator can offer an upgrade or a top-up.
- Webhooks report `quota.near` (80% of a limit), `quota.exceeded`, `credits.low`, and `plan.expired`, signed with a secret of the Runtime Workspace.

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
