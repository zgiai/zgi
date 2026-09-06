# Workspace developer access

ZGI provides workspace-scoped personal API keys for interactive development. A personal key is owned by a user principal, consumes the user's shared workspace grant, and is invalidated when the grant or workspace membership is no longer active.

## Access model

- Workspace owners and administrators can create personal keys directly unless developer access is disabled.
- Other members follow the workspace policy: `self_service`, `approval_required`, or `disabled`.
- In approval mode, an approved request creates or updates one grant for the user. All of that user's personal keys share the grant's model scope, expiration, key count, and quota.
- Reviewers can set the approved quota, model scope, expiration, and maximum key count, but cannot approve their own requests.
- New personal keys store only a hash. The plaintext secret is returned once by the create endpoint and cannot be retrieved later.
- Disabling a key is reversible. Revocation is permanent. A user can rotate only their own key; rotation revokes the old key atomically and returns the replacement secret once.
- Removing a user from the workspace immediately makes their personal keys unusable. Organization managers who are not workspace members can administer access but cannot create a personal key for that workspace.
- When a self-service grant reaches its configured expiration, the next key creation starts a new grant period and revokes keys from the expired authorization period.

Service-account identities are represented by `principal_type = service_account` in the storage model, but service-account management is intentionally deferred to the next product phase.

## Console API

All routes require console authentication and a workspace membership or workspace-management permission.

```text
GET  /console/api/llm/workspaces/:workspace_id/developer-access/me
GET  /console/api/llm/workspaces/:workspace_id/developer-access/policy
PUT  /console/api/llm/workspaces/:workspace_id/developer-access/policy
GET  /console/api/llm/workspaces/:workspace_id/developer-access/audit

GET  /console/api/llm/workspaces/:workspace_id/access-requests
POST /console/api/llm/workspaces/:workspace_id/access-requests
POST /console/api/llm/workspaces/:workspace_id/access-requests/:request_id/cancel
POST /console/api/llm/workspaces/:workspace_id/access-requests/:request_id/approve
POST /console/api/llm/workspaces/:workspace_id/access-requests/:request_id/reject

GET   /console/api/llm/workspaces/:workspace_id/api-keys
POST  /console/api/llm/workspaces/:workspace_id/api-keys
PATCH /console/api/llm/workspaces/:workspace_id/api-keys/:key_id
POST  /console/api/llm/workspaces/:workspace_id/api-keys/:key_id/disable
POST  /console/api/llm/workspaces/:workspace_id/api-keys/:key_id/enable
POST  /console/api/llm/workspaces/:workspace_id/api-keys/:key_id/revoke
POST  /console/api/llm/workspaces/:workspace_id/api-keys/:key_id/rotate
```

The console UI is available at `/console/api-keys`. Members see their own keys, requests, and call audit. Workspace managers additionally see member keys, the full approval history, and workspace-scoped personal-key audit records. Query caches include the workspace ID so switching workspaces cannot display data from the previous workspace.

Both desktop and mobile navigation include **Model Plaza** and **API Keys**. On the API Keys page, **Refresh** and switching tabs reload active queries for the current workspace, including the shared quota balance. Disabled administrator queries are not fetched for ordinary members. Call audit rows display the request ID, input/output token counts, and points so a caller can correlate a response with its usage record.

If workspace navigation reports that permissions cannot be confirmed, inspect the browser warning `workspace.permissions.load_failed` and correlate its request ID, when available, with API access logs. The diagnostic includes allowlisted error/status codes and session/workspace lifecycle states, but excludes raw errors, request headers, response bodies, credentials, and account IDs. It is also sent to configured observability adapters. An HTTP 200 in server logs alone does not prove that the browser received or processed the permission response. Failed reads continue to deny unconfirmed access; diagnostics do not grant permissions or retry requests automatically.

## Runtime attribution

### Quota units

The console displays and accepts quota in points, consistently with workspace quotas. One displayed point equals 1,000 internal credits. The console preserves three decimal places (0.001 point is one internal credit), including in personal balances, access requests, approvals, policy limits, and call audit. Points are not tokens or a currency amount.

Organization invocation details also preserve this precision: a charge of 45 internal credits is displayed as 0.045 points, not rounded to 0.04 points. The API source label covers both personal and legacy organization keys; it does not imply ownership by the organization.

Personal key rows show their own expiration, separately from the grant expiration. On refresh, an expired key is labeled **Expired** even when its stored status is still active; activation and rotation are not offered for it. Revocation remains available for cleanup. When all active key slots are occupied, the page explains how to free a slot or rotate an existing key. Expiration and slot limits are enforced by the API independently of the UI.

Request history includes the reviewer's explanation so members can understand a rejection or approval conditions without access to administrator controls.

Personal-key authentication also distinguishes expired and disabled keys in its localized public message, using the authoritative database row rather than cached lifecycle state. These message overlays preserve the existing HTTP status and protocol error codes.

### Grant allowance versus actual model cost

The grant limits the amount charged to that allowance, not necessarily the full upstream cost of an admitted request. For a Cloud route without a local price estimate, the gateway reserves the entire remaining allowance and serializes such requests. Settlement caps the grant charge at its remaining limit and records any difference as `quota_overage_points`; `total_points` still records the actual model cost. Thus a final request costing 45 internal credits can charge the last one credit to its grant and record 44 as overage. The subsequent request is denied, but the first request's actual cost is not reduced to one credit.

For reconciliation, sum `quota_charged_points` against the grant's usage, and sum `total_points` against model usage costs. Include `quota_overage_points` when explaining the difference. Do not present this as a strict monetary spending cap. A strict enterprise cost ceiling requires a reliable admission-time upper-bound quote or an explicit rule denying unpriced routes; it must not be inferred from the grant balance alone.

When a personal grant has no available allowance, gateway authentication now explains that the allowance is exhausted or reserved by in-flight requests, rather than claiming the key was revoked. Creating or rotating a key does not replenish the shared grant. This is a message-only compatibility fix: personal-grant authentication still returns HTTP 401 with OpenAI `invalid_api_key` or Anthropic `authentication_error`; an independent key-quota rejection retains its existing 429/402 contract. Clients must not infer revocation from HTTP 401 alone. A migration of the personal-grant status/code is a separate protocol change. The message supports English and Simplified Chinese through `Accept-Language`. Workspace membership, policy and key lifecycle failures continue to take precedence over quota details.

Console API quota fields and audit `*_points` fields retain their existing integer internal-credit contract. For example, a 100-point request is sent as `requested_quota: 100000`; an audit value of `total_points: 1234` displays as 1.234 points. API clients must not send UI point values directly. Missing or null limits retain their default/unlimited semantics; zero remains zero. This display conversion does not rescale stored grants, change billing, or replenish any balance.

### Shared billing subject

For `/v1/chat/completions` (streaming and non-streaming), record the response header `X-ZGI-Request-ID`. This server-generated invocation ID matches the audit `request_id` and links its provider attempts and billing records. It is exposed through CORS for browser clients. `X-Request-ID` remains the transport correlation ID and may be supplied by the caller; it is not a billing idempotency key. Reusing a client header does not reuse an invocation or skip charges. Authentication/JSON validation failures that never enter the gateway may not have an invocation ID or a provider-attempt audit record.

Gateway usage records include the workspace account, principal type and ID, access grant ID, API key ID, and authentication method. Personal-key calls are charged against the shared developer grant rather than an independent per-key balance. This keeps key rotation from resetting a user's assigned quota and lets administrators audit usage across all keys owned by one user. Billing retry and reconciliation records persist the same attribution so failure recovery does not downgrade a personal-key call to legacy-key usage.
