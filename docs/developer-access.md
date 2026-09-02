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

## Runtime attribution

Gateway usage records include the workspace account, principal type and ID, access grant ID, API key ID, and authentication method. Personal-key calls are charged against the shared developer grant rather than an independent per-key balance. This keeps key rotation from resetting a user's assigned quota and lets administrators audit usage across all keys owned by one user. Billing retry and reconciliation records persist the same attribution so failure recovery does not downgrade a personal-key call to legacy-key usage.
