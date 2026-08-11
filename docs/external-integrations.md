# External integrations

ZGI external integrations let AIChat, Agents, and future Workflow connector
nodes use approved third-party applications through organization or personal
connections. Providers describe their authentication methods, actions,
schemas, health probe, and governance metadata. Each product surface selects a
Connection through its own tool-specific selection. AIChat discovers those actions
through one hidden Connected Apps runtime capability, so adding a provider does
not require adding one visible Skill per application.

The current built-in providers are:

- GitHub REST: account identity, repository list/search, issue and comment
  reads, plus approval-gated issue and comment creation.
- Exa Web Search: public web search and bounded webpage retrieval. Search,
  citation, data-egress, and prompt-injection guidance is published by its
  Actions through the same Connected Apps runtime as every other provider.
- Gmail: Google account identity, mail search/read, draft creation, and
  approval-gated send/reply through Google OAuth 2.0.
- Feishu (China): delegated user identity, contact discovery, chat and calendar
  listing, message and calendar-event reads, Drive and document reads, plus
  approval-gated message and calendar-event creation.
- WeCom: organization custom-application identity, department and member
  discovery, member reads, and approval-gated application messages.
- DingTalk: organization internal-application department, member, and role
  discovery; bounded attendance reads; approval-gated member or department
  work notifications; and delivery-result checks.
- Standard Mail: provider-neutral IMAP folder/search/read and approval-gated
  SMTP send/reply over validated TLS endpoints.
- X API v2: account and public-user lookup, own-user and selected-user post
  reads, with recent search and post creation disabled by default until an
  administrator enables them.

OAuth 2.0 browser connection, token refresh, reconnect, and scope upgrade are
included for Gmail, delegated Feishu accounts, and X. Workflow integration
nodes, MCP transport, dynamic OpenAPI imports, and the global Lark provider are
not part of this release.

## Capability coverage and roadmap

The built-in catalog implements the basic progression from identity, through
search/list and detail reads, to narrowly scoped writes protected by connection
grants, provider scopes, organization policy, and approval. Provider
definitions and their live Action metadata are the authoritative source for
the current Provider and Action counts.

The next P1 capability layer is intentionally not implemented yet: GitHub pull
requests and code reads, Gmail labels and attachments, Feishu group/member and
document search, and X conversation and engagement reads. P2 is event-driven
execution through triggers, webhooks, and Workflow integration. Those items are
roadmap directions, not currently available Actions.

## Runtime model

The main runtime boundary is:

```text
ProviderDefinition
  -> Connection (encrypted credential and non-secret configuration)
  -> Connection grant and organization Action policy
  -> tool-specific selection (AIChat preference, Agent binding, future Workflow node)
  -> target-specific runtime discovery
  -> Integration Executor
  -> Provider Adapter
  -> execution audit and health signal
```

Provider definitions and Action schemas are authoritative server-side. The
browser never submits provider credentials as model arguments. At invocation
time, the Executor rechecks the organization, account, workspace, Connection,
grant, Action allowlist, provider scopes, policy, and health/auth state before
decrypting a request-scoped credential. Resolved secrets are destroyed after
the adapter call.

Tool-specific selections are not authorization. AIChat preferences and
Agent resource bindings remain separate because they have different ownership,
versioning, and runtime semantics. Revoking a grant, disabling a Connection,
changing an Action policy, or invalidating a token takes effect on the next call
even when the target already selected the Connection.

## Authentication method model

Provider authentication is described with stable, composable metadata rather
than a growing list of provider-specific core types. Each authentication
method declares:

- `identity_kind`: `user`, `application`, `channel`, or `service`;
- `acquisition_strategy`: `browser_redirect`, `manual_form`, or `none`;
- `lifecycle_strategy`: `static`, `oauth_refresh`, `exchange_on_demand`, or
  `signed_request`;
- `request_auth_strategy`: `bearer_header`, `api_key_header`,
  `api_key_query`, `basic_header`, `oauth1_signature`, `webhook_url`,
  `provider_custom`, or `none`.

The existing storage-oriented method types (`oauth2`, `api_key`,
`custom_credential`, and `service_account`) remain compatible. `no_auth`
remains a reserved catalog type but fails closed when marked available until a
credential-free Connection runtime is implemented. Legacy Provider
definitions that omit the four strategy fields receive safe defaults during
registration. New definitions should declare the fields explicitly so the UI
can present browser authorization and manual connection methods without
provider-name conditionals.

An Action still lists its compatible authentication method IDs. Adding a
manual token method therefore does not automatically grant that method access
to user-context or write Actions. Credential ownership also remains explicit:
personal and organization-managed variants use separate authentication method
IDs and the existing `account` or `organization` Connection source.

This catalog-only expansion does not require a database migration. Connections
continue to persist the stable `auth_method_id`; credentials continue to use
the encrypted Connection vault.

## Action permission model

Provider permissions are expressed with three separate fields:

- `required_scopes`: an AllOf set; every listed permission must be present;
- `required_any_scopes`: an AnyOf set; at least one listed alternative must be
  present;
- `preferred_scopes`: the least-privilege alternative ZGI requests during a
  new OAuth consent or scope-upgrade flow.

The runtime accepts an already-granted non-preferred alternative when it
satisfies the AnyOf set. OAuth never requests every alternative merely because
the provider offers both read-only and broader permissions. Registration fails
when an Action references an undeclared scope, an AnyOf set has no preferred
member, or a preferred scope is not one of its alternatives.

This distinction is important for provider APIs that expose multiple valid
permission names for the same read operation, while some writes require two
permissions together. It prevents both false permission-denied errors and
accidental requests for unnecessarily broad access.

## Deployment configuration

Enable the external integrations subsystem with:

```env
EXTERNAL_INTEGRATIONS_ENABLED=true
INTEGRATION_ORG_DAILY_LIMIT=1000
INTEGRATION_TIMEOUT_SECONDS=20

# Each value must be exactly 32 bytes. Use an opaque key ID.
INTEGRATION_CREDENTIAL_ACTIVE_KEY_ID=2026-07
INTEGRATION_CREDENTIAL_KEYS_JSON={"2026-07":"0123456789abcdef0123456789abcdef"}

INTEGRATION_HEALTH_FAILURE_THRESHOLD=3

# OAuth browser flow and refresh behavior.
INTEGRATION_OAUTH_FLOW_TTL_SECONDS=600
INTEGRATION_OAUTH_REFRESH_WINDOW_SECONDS=600
INTEGRATION_OAUTH_CALLBACK_URL=https://api.example.com/console/api/integrations/oauth/callback
INTEGRATION_OAUTH_RESULT_URL=https://app.example.com/console/integrations/oauth/result
```

This single switch registers every built-in Provider, including Exa Web
Search. Individual Provider availability is then determined by its configured
Connections, connection health, usage rules, and caller selection or binding;
Web Search has no separate enable flag.

The example key above is illustrative; generate a unique random 32-byte value
for every deployment and store it in a secret manager. Do not commit the JSON
keyring or expose it through a browser environment variable.

For backward compatibility, ZGI can read `API_KEY_ENCRYPTION_KEY` as a legacy
credential key when the explicit keyring is absent. New deployments should use
the named keyring. The active integration credential key also derives the
audit input-HMAC key for every Provider.

Apply migrations before enabling traffic:

```bash
cd api
go run ./cmd/migrate up
go run ./cmd/migrate status
```

Restart every API instance after changing provider or keyring configuration.

### OAuth application ownership

ZGI does not depend on an OpenConnector, OOMOL, Creao, or other hosted
credential broker. An organization owner or administrator configures its own
Google, Feishu, or X OAuth application from **Connection Center → Management
center → OAuth application**. The UI displays the exact callback URL that must
be registered at the provider.

OAuth client secrets are write-only and encrypted with the same credential
keyring as Connection tokens. Client ID changes and OAuth application deletion
fail closed while dependent or disabled Connections, or an unexpired pending
authorization flow, still exist.

`INTEGRATION_OAUTH_CLIENTS_JSON` is an optional deployment-level fallback for
self-hosted installations that intentionally share one OAuth application. An
organization-owned UI configuration takes precedence. The JSON shape is:

```json
{
  "gmail": {
    "client_id": "google-client-id",
    "client_secret": "google-client-secret",
    "config": {}
  },
  "feishu": {
    "client_id": "feishu-app-id",
    "client_secret": "feishu-app-secret",
    "config": {}
  },
  "x": {
    "client_id": "x-client-id",
    "config": {}
  }
}
```

X public PKCE clients may omit `client_secret`. If an X application is
configured as a confidential web client, include its client secret instead.
Google and Feishu clients always require their provider-issued secret.

The OAuth configuration dialog includes a provider-owned setup guide before
the credential fields. The guide is catalog metadata, not organization data,
so it does not require a database migration and never contains credential
values. It explains where to create the provider application, where to copy
the client identifier and secret, how to register the exact callback URL, and
which provider-specific publishing or test-user steps are required.

The built-in guides intentionally differ:

- Feishu points administrators to Credentials & Basic Info, Security Settings,
  permission management, and application publishing. `user_profile` and
  `auth:user.id:read` are implicit identity scopes and must not be manually
  inserted into the authorization request.
- Gmail requires a Google Cloud project, the Gmail API, Google Auth Platform
  branding/audience/data-access setup, test users when the application is in
  Testing, a Web application OAuth client, and an exact authorized redirect
  URI.
- X requires OAuth 2.0 Authorization Code with PKCE, an exact callback URI,
  and an explicit client type. Public clients omit the client secret;
  confidential Web/Automated/Bot clients use one.

Provider setup guides are declared on authentication methods and are also
rendered in the personal and organization Connection dialogs for manually
entered credentials. The guide changes with the selected authentication
method; OAuth application setup is not reused for a PAT, API key, Bearer token,
or service-account credential.

The built-in manual credential guides cover:

- GitHub fine-grained PAT creation, repository ownership, minimum permissions,
  expiration, and possible organization approval;
- Exa dashboard API-key creation plus credit and budget checks;
- X application-only Bearer tokens, including the distinction between
  app-only public-data access and delegated OAuth user actions; and
- Feishu tenant application App ID/App Secret, application-identity
  permissions, availability scope, and publishing.

New service-account or token-based providers can reuse the same safe step
contract without adding provider-specific UI branches. Guide links must be
absolute HTTPS URLs, steps and notices are bounded and localized, and only the
built-in open-console, open-documentation, and copy-callback actions are
accepted. The metadata contains no credential value and does not require a
database migration.

Never commit this value. Prefer the organization-owned encrypted UI
configuration unless the deployment operator deliberately manages OAuth
applications centrally.

The callback and result URLs must be HTTPS in production. Loopback HTTP is
accepted only for local development. The callback URL is server-owned and
must not be derived from an inbound `Host` header.

For direct source development, the default callback follows `SERVER_PORT`
(normally `http://127.0.0.1:2670`). Docker explicitly routes callbacks through
its public gateway (normally `http://localhost:2679`). Production deployments
must set `INTEGRATION_OAUTH_CALLBACK_URL` and
`INTEGRATION_OAUTH_RESULT_URL` to externally reachable HTTPS URLs; startup
fails closed if a production environment still uses a loopback URL. After
changing either URL, restart every API instance, update the provider's exact
registered redirect URI, and begin a new OAuth flow because an existing flow
retains its original callback URL.

When the Web UI and API use separate origins, keep them on the same
schemeful site (for example `app.example.com` and `api.example.com`), allow
the exact Web origin in `WEB_API_CORS_ALLOW_ORIGINS`, and enable credentials
on the reverse proxy. The OAuth start request is the only application API
request that accepts the HttpOnly browser-binding cookie; the cookie is then
sent directly to the API callback during the provider redirect.

### OAuth connection lifecycle

The **Connect** button opens the provider authorization page in a new browser
window. The flow uses:

- a short-lived opaque flow reference;
- one-time, server-stored OAuth state;
- a high-entropy HttpOnly browser binding whose SHA-256 digest is checked
  atomically with state, so a callback opened in another browser cannot
  consume the original browser's flow;
- PKCE S256;
- an exact allowlisted callback URL;
- a callback that rechecks current organization membership and, for shared
  Connections, current administrator/owner authority;
- a result page that receives only a safe status and opaque flow reference,
  never an authorization code, state, token, Connection UUID, or provider
  error description.

On success, access and refresh tokens are encrypted in the Connection vault.
Before a call, ZGI refreshes an expiring token under a distributed Redis lock
and persists rotating refresh tokens with compare-and-swap protection.
Provider-reported refresh-token expiry is stored separately from short-lived
access-token expiry; an expired refresh token moves the Connection to
reconnect-required without making another provider request. A successful
provider refresh whose database write is temporarily unavailable is retained
as an encrypted recovery task, so the old rotating token is not reused.
Immediately after a provider exchanges an authorization code, ZGI also commits
an encrypted compensation task containing the issued tokens and an immutable
OAuth client snapshot before it performs local scope, profile, or Connection
work. The task is guarded by the OAuth flow outcome: a successful Connection
commit only acknowledges it, while a failed, expired, or interrupted flow is
revoked immediately when possible and retried by a leased worker after restart.
This guard prevents a delayed worker from revoking tokens that belong to a
successfully committed Connection. The callback fails before calling the
provider token endpoint when this durable recovery path is not fully wired; it
never downgrades silently to in-process-only compensation.
Deleting an OAuth Connection commits its encrypted provider-revocation task
and the local deletion in one database transaction. Provider or Redis outages
therefore cannot lose cleanup work. The task also contains an encrypted,
immutable OAuth client snapshot, so a later client configuration rotation or
deletion cannot orphan the revocation; leased workers retry it safely after a
process restart.
In Connection Center this operation is presented as **Disconnect account** for
OAuth Connections. It removes the local encrypted tokens and AIChat selection
as one operation, then requests provider revocation when the provider supports
it. Shared accounts remain blocked from disconnection while an Agent binding
still depends on them. API-key and PAT Connections continue to use the
**Delete connection** label because they do not represent a delegated OAuth
account.
`invalid_grant`, revoked access, or missing scopes changes the Connection to a
reconnect/attention state rather than silently falling back to another
account. Reconnect and scope-upgrade flows update the existing Connection
without exposing its internal identifier in the UI. When a replacement issues
a different revocation token, the old encrypted credential snapshot is queued
in the same transaction as the Connection update and revoked asynchronously.
An unchanged refresh or access token is never revoked as superseded.

OAuth success has different next steps:

- a personal Connection is immediately eligible for its owner's AIChat
  Connected Apps selector;
- a shared Connection still requires an explicit usage rule before members or
  Agents can use it.

### Rotating credential keys

Key rotation is read-old/write-new:

1. Add a new 32-byte entry to `INTEGRATION_CREDENTIAL_KEYS_JSON` while keeping
   every key ID still referenced by stored envelopes.
2. Change `INTEGRATION_CREDENTIAL_ACTIVE_KEY_ID` to the new ID and restart all
   API instances.
3. New and updated Connections are encrypted with the active key. Existing
   envelopes remain readable with their embedded key ID.
4. Keep old keys until all corresponding Connections have had their
   credentials replaced. Removing an in-use old key makes those Connections
   unavailable and fail closed.

Key IDs are metadata, not secrets. Key values are secrets.

## Using Connection Center

Open `/console/integrations` from **Authoring tools → Connection Center**.
The page deliberately separates two tasks:

- **Available** discovers providers and starts a personal or organization-owned
  connection with the authentication methods that provider supports.
- **Connected** groups existing Connections by provider and shows credential
  health, usage-rule coverage, available tools, and management actions.

Creating a Connection now continues into a resumable required-setup flow. The
flow verifies the credential, shows the provider capabilities covered by the
granted scopes, requires at least one explicit usage rule for a shared
Connection, and then presents available tools as a separate step. AIChat can be
selected for the explicitly named current workspace. Agent bindings remain in a
specific Agent draft/version, and Workflow bindings will remain in a specific
Workflow node once connector nodes are available. No target is required to mark
the Connection itself ready. Closing the flow does not discard the Connection;
incomplete Connections show **Continue setup** in **Connected**. After the
required connection steps are complete, **Connected** remains the place to
maintain credentials, health, and usage rules, and to review or enter the
configuration boundary for each supported tool.

Gmail and X remain registered adapters so existing Connections, encrypted
credentials, execution authorization, and audit history continue to work.
They are temporarily hidden from the **Available** discovery catalog, so new
Connections, including the **Add another connection** action on an existing
provider group, are not offered through the UI until those product surfaces
are ready to be enabled again.

Every organization member can manage personal Connections and view shared
Connections they are authorized to use. Organization owners and administrators
can also create, edit, test, disable, or delete organization-owned Connections;
define who can use them at the organization, workspace, or account level;
configure Action policies; and inspect execution records from **Management
center**.

The legacy `/dashboard/organization/integrations` routes redirect to Connection
Center. They no longer expose a second connection-management surface.

Connection ownership, usage rules, and chat selection are separate concepts:

- **Personal connections** contain credentials owned by the current account.
  Only that account can manage or use them, and Agents cannot bind them.
- **Shared connections** contain organization-owned credentials. Organization
  administrators manage them and explicitly define eligible callers and
  available Actions through usage rules.
- **Connected Apps** in AIChat only selects which currently available
  connections a conversation may use. Selection never creates a usage rule or
  bypasses a removed rule.

Connection details use one permission-summary contract for personal and shared
Connections. The summary separates:

- **ZGI capabilities**, which are the provider Actions currently adapted by
  this deployment and compatible with the Connection authentication method;
- **provider grants**, which are the raw scopes or permissions reported by the
  provider;
- **identity and lifecycle permissions**, which authenticate the external
  identity or keep an OAuth Connection signed in; and
- **missing permissions**, which require a credential replacement or OAuth
  scope upgrade.

Provider-reported raw scopes remain the source for fail-closed runtime
authorization and are not replaced by this presentation summary. Unknown future scopes are shown
by their safe provider identifier instead of being collapsed into an ambiguous
"other permission" label. Broad provider grants are highlighted, while ZGI
still limits execution to adapted Actions that pass usage rules, organization
policy, approval, and runtime scope checks. Providers that do not return a
scope list are reported as such; no permissions are inferred from an empty
scope response.

Authentication methods also declare the source of their scope evidence.
OAuth providers that return an exact scope set use `provider_reported`.
Connectors such as DingTalk, WeCom, standard mail, and X app-only access, whose
authentication APIs do not return a complete grant list, use
`connector_declared`: the displayed access groups map
adapted Actions to their expected provider permissions, but are not presented
as provider-verified grants. Those Actions remain callable and are verified by
the provider on each real request. A successful connection probe verifies only
the endpoints named by that probe; it must not imply that unrelated attendance,
message, role, or contact Actions were tested.
For these connectors, ZGI stores evidence by exact Action: a successful real
request verifies only that Action, while a provider error that unambiguously
means a missing application permission marks only that Action as denied. A
generic HTTP 403 remains diagnostic evidence and never contaminates unrelated
Actions because it may represent a resource ACL rather than a missing provider
grant.

Personal authentication methods create account-owned Connections. A user can
manage, test, disable, and remove only their own personal Connections from the
**Connected** view in Connection Center. Personal
credentials are not exposed to organization peers through catalog or Agent
candidate APIs. Secret fields are write-only: leaving them unchanged during an
edit preserves the existing encrypted value, while submitting a replacement
rotates the credential version. Disable or delete a Connection that should no
longer retain a usable credential.

Every usage rule stores explicit Action IDs. The management API represents
these rules as grants internally and does not accept a wildcard that could
silently authorize Actions added by a provider later. When a
ProviderDefinition changes, administrators review and enable the new Actions
deliberately. If a previously enabled Action is removed from the provider, the
management interface marks it as unavailable and requires the administrator to
remove it before saving other changes.

Usage-rule scopes mean:

- **Entire organization** applies to organization members. Agents still need
  an explicit binding to the exact Connection and Action.
- **Specific workspace** applies only in that workspace context. Agents in the
  workspace still need an explicit binding.
- **Specific member** applies only to that member's AIChat usage and never
  authorizes an Agent.

Only active organization members and non-archived workspaces can receive new
usage rules. Existing rules whose subject is no longer active remain visible
as needing attention so an administrator can replace or delete them.

Applicable usage rules are additive: if any matching rule permits the requested
Action and effect, the call may proceed to the remaining policy checks. A more
specific member or workspace rule does not reduce an organization-wide rule.
To remove broad access, narrow or remove the broad rule, or disable the Action
through the organization Action policy. In the API, `access_mode=write` means
read and write; the management interface displays it as **Read and write**.
Legacy provider-owned rules with resource-level constraints are shown as
read-only. The list API exposes only whether such constraints exist and does
not return their non-editable policy body to this editor.

A successful connection test proves only the provider authentication and
reported scopes at that moment. Provider availability and connection health
are shown separately; “configured” is never treated as synonymous with
“healthy.” ZGI does not run periodic provider probes. Health changes only
after an explicit connection test, credential lifecycle operation, or a real
AIChat/Agent invocation, so background monitoring cannot consume provider
quota or account balance.

## Using connected applications in AIChat

1. Create and test a personal Connection, or ask an organization
   administrator to add a usage rule for your account/workspace.
2. Open AIChat and choose **Connected Apps** in the composer toolbar.
3. Select one or more healthy Connections. When an application has multiple
   Connections, choose its preferred Connection.
4. Save the selection, then ask AIChat for the task in normal language, for
   example “list my recently updated GitHub repositories.”

The Connected Apps selector is separate from the Skill selector. A generic
provider such as GitHub does not appear as a visible Skill. The hidden runtime
can list the selected Connections, search their allowed Actions, request an
Action guide, and execute the exact Action. All final calls still pass through
the Executor and current usage, policy, and permission checks.

Action search results include a compact list of required and optional argument
names. If a dynamic Action call does not match the provider schema, ZGI rejects
it before approval, quota use, credential resolution, audit creation, or an
external request. The runtime returns bounded, value-free field diagnostics and
the current safe input schema so the model can read the Action guide, change the
arguments, and retry once. Repeating the same invalid call is stopped; the final
answer must not claim that the provider lacks an Action that was merely called
with invalid arguments.

Provider validation errors contain only provider-neutral structural facts. The
Connected Apps surface may recommend `get_action_guide` and `execute_action`,
while a direct non-integration Skill tool retries the current
`call_skill_tool` contract. Provider Actions never depend on a visible Skill.

Actions that require a target identifier may declare bounded preparation hints.
For example, a message Action can point to a read-only contact or chat search,
and an issue write Action can point to repository or issue lookup. AIChat only
receives a hint when that preparation Action is available for the same
Connection, authentication method, caller, current provider scopes, usage
rules, Agent binding, and effective organization policy. Declared result paths
must exist in the preparation Action output schema or provider registration
fails. The provider definition remains authoritative; the generic Skill does
not hardcode Feishu-, GitHub-, or future provider-specific orchestration.

Web Search is selected and invoked exactly like any other connected
application. Its `web.search` and `web.fetch` Action guides carry the search,
citation, minimal-query, source-selection, and untrusted-content rules; there
is no separate Web Search Skill or Skill preference.

## GitHub authentication and Actions

GitHub supports two PAT-based methods in this release:

- personal access token: an account-owned personal Connection;
- organization personal access token: an administrator-managed shared
  Connection that still uses a GitHub PAT but is governed by ZGI grants.

Fine-grained PATs with the minimum repository access are recommended. ZGI does
not return the token after creation. Available read Actions are:

- `github.user.get`
- `github.repository.list`
- `github.repository.search`
- `github.issue.list`
- `github.issue.get`
- `github.issue.comment.list`

The following write Actions are disabled by default and require explicit
approval whenever enabled:

- `github.issue.create`
- `github.issue.comment.create`

Repository and issue responses are bounded before entering model context.
GitHub primary and secondary rate limits are distinguished from authentication
and repository-access failures. Safe retry metadata uses `Retry-After` or
`X-RateLimit-Reset`; redirects are rejected so a credential-bearing request
cannot be silently moved to another endpoint.

GitHub remains PAT-based in this release. It does not reuse the Gmail, Feishu,
or X OAuth application because provider tokens and authorization semantics are
kept provider-specific.

## Gmail OAuth and Actions

Gmail offers personal and organization-owned delegated Google OAuth methods.
The default consent request asks only for OpenID identity and email scopes.
Search and read request `gmail.readonly`; send requests `gmail.send`; reply
requires both read and send so the server can preserve the original thread and
reply headers; draft creation requests `gmail.compose`.

Available Actions are:

- `gmail.account.get`: enabled by default, read-only;
- `gmail.mail.search`: enabled by default, read-only, and returns bounded
  message summaries;
- `gmail.mail.get`: enabled by default, read-only, safely parses MIME, and
  returns a bounded plain-text body;
- `gmail.mail.send`: disabled by default, high risk, non-idempotent, and every
  invocation requires explicit approval after an administrator enables it;
- `gmail.mail.reply`: disabled by default, high risk, non-idempotent, preserves
  `threadId`, `References`, and `In-Reply-To`, and always requires approval;
- `gmail.draft.create`: disabled by default and always requires approval. It
  creates a draft without sending it.

Email bodies and recipients are validated and bounded before network I/O.
ZGI sends a plain-text RFC 2822 message through the official Gmail API and
never asks for or stores the user's Google password.
Google's structured error reasons distinguish quota, rate-limit, domain-policy,
authentication, and missing-permission failures. Email sends are never
automatically retried because a lost response could otherwise produce a
duplicate message. Reply and draft creation are also not automatically retried.

## Feishu authentication and Actions

This release registers the China-region Feishu provider only. It supports:

- personal delegated Feishu OAuth;
- organization-owned delegated Feishu OAuth;
- an organization-owned Feishu tenant app using write-only App ID and App
  Secret fields.

Available read Actions are:

- `feishu.account.get`;
- `feishu.drive.list`;
- `feishu.document.read`;
- `feishu.contact.search`;
- `feishu.chat.list`;
- `feishu.calendar.list`;
- `feishu.message.list`;
- `feishu.calendar.event.list`.

`feishu.contact.search` is delegated-user only. Drive and document reads, plus
chat and calendar listing, can use a delegated user or a tenant app when the
selected authentication method has a valid corresponding permission. A tenant
app can only see files and documents that are available to the application
identity; enabling a provider permission does not bypass Feishu application
availability or resource-level access. All list and search inputs, pagination
tokens, returned records, and text fields are bounded before entering model
context.

Delegated OAuth code exchange and refresh use the current fixed Feishu OpenAPI
endpoint `https://open.feishu.cn/open-apis/authen/v2/oauth/token`. The
authorization-code exchange includes the server-held PKCE verifier.
`auth:user.id:read` and `user_profile` may appear in the token response as
implicit identity scopes, but ZGI never sends them as application permissions
in the authorization URL. The account identity Action therefore needs no
explicit business permission. Long-lived connections request
`offline_access`, which must be enabled and published in the Feishu app.

Sending as a delegated user requires both `im:message` and
`im:message.send_as_user`. Tenant-app messaging accepts either the focused
`im:message:send_as_bot` permission or the compatible broader `im:message`
permission, and requests the focused permission by default.

Connections begin with the minimum identity permissions. Connection health
therefore proves that the credential and account identity work; it does not
claim that every adapted Action is authorized. The Connection detail shows
each Action as ready or requiring additional provider access. For delegated
OAuth, **Grant access** starts a `scope_upgrade` flow for exactly the selected
Action and updates the existing Connection after consent; users do not need to
delete or duplicate the Connection.

`feishu.message.send_user` accepts `recipient_type: self` for messages to the
connected account. The server resolves the account's Open ID from the
request-scoped Connection, while the model and approval UI see only “Myself”.
Explicit `open_id`, `user_id`, `union_id`, and `chat_id` targets remain
available through `recipient_id`. Tenant-app bot messages always require an
explicit recipient.

Tenant-app messaging uses `feishu.message.send_bot`. Both message Actions are
disabled by default, high risk, and non-idempotent. The first invocation
requires approval. A remembered approval is limited to the exact Connection
and recipient; an organization policy may still require approval for every
invocation. `feishu.calendar.event.create` is also disabled by default and
requires approval before creating an event in a writable calendar. Reads remain
bounded and use fixed `open.feishu.cn` endpoints.

Before enabling either send Action, publish the current Feishu application
version with the required permission and availability scope. For bot messages,
enable bot capability and ensure that the bot is available to—and, for a group
target, present in—the destination chat. Use `feishu.contact.search` or
`feishu.chat.list` to discover a permitted target instead of asking the model
to invent an Open ID or Chat ID. A connection health check proves the identity
credential works; it does not prove that a particular recipient, group, or
write Action is available.

Message-history reads are bounded and require a visible chat plus one of the
supported read-permission alternatives for the selected authentication method.
Calendar-event reads require an explicit calendar and time range. Neither
capability bypasses Feishu application availability or resource-level access.

Lark global is intentionally not presented as a region selector. Its endpoints,
application registration, and governance destination differ; add it later as
an independently reviewed Provider rather than switching domains through a
user-controlled field.

Feishu does not publish a general revocation endpoint for this delegated web
flow. If authorization succeeds at Feishu but ZGI cannot finish validating or
committing the Connection, ZGI stores only an encrypted compensation envelope
and marks the operation as requiring administrator remediation. The
administrator must ask the user to remove the application authorization in
Feishu account settings, then explicitly acknowledge either that provider
access was removed or that the token was confirmed expired. Unacknowledged
dead letters are never deleted automatically and remain visible in Connection
Center; their API and UI summaries contain provider, reason, attempt count, and
timestamps only, never tokens, client secrets, or Connection identifiers.
Acknowledgement atomically destroys the encrypted token and OAuth client-secret
payload. ZGI retains a secret-free audit tombstone with the provider, auth
method, sanitized reason, attempt count, failure time, acknowledging
administrator, acknowledgement time, and explicit resolution; acknowledgement
does not delete the recovery history.
Gmail and X support a provider revocation endpoint, so ZGI attempts immediate
revocation and retains the encrypted task for restart-safe retries until it
succeeds or requires the same explicit administrator remediation.

## X authentication and Actions

X supports two intentionally separate authentication identities:

- delegated user OAuth 2.0 Authorization Code with PKCE, requesting
  `offline.access` so an eligible application can issue refresh tokens;
- an organization-managed, manually entered X app Bearer Token for supported
  public, read-only user lookup, user-post listing, and recent search.

The app Bearer Token never represents a user. It cannot read the current
account, list that account's posts, or publish a post. Those Actions remain
restricted to delegated OAuth methods. The token is write-only in the browser,
encrypted before storage, and sent only in the fixed `Authorization: Bearer`
header to the official X API.

Available Actions are:

- `x.account.get`: enabled by default, read-only;
- `x.user.get_by_username`: enabled by default, read-only public-user lookup;
- `x.post.list_own`: enabled by default, read-only;
- `x.post.list_by_user`: enabled by default, read-only public-post listing for
  one confirmed user ID;
- `x.post.search_recent`: disabled by default because availability and cost
  depend on the connected X developer plan; it supports delegated OAuth and
  the app-only Bearer method;
- `x.post.create`: disabled by default, high risk, non-idempotent, and every
  invocation requires explicit approval after an administrator enables it.

X responses, pagination tokens, queries, and post text are bounded. All API
traffic uses fixed official X API v2 endpoints. RFC 7807 errors are normalized
into stable platform errors. A nominal HTTP success that still contains X
`errors` is treated as a failed response instead of exposing partial data as a
successful Action.

## Adding another provider

Adding Notion, Slack, or another provider normally requires an Adapter,
not a new Skill:

1. Add a provider package under
   `api/internal/modules/integrations/adapters/<provider>`.
2. Define a stable `ProviderDefinition`: identity, categories, documentation,
   explicit authentication methods and write-only credential fields, the four
   generic authentication strategy dimensions, health probe characteristics,
   and bounded Actions.
3. Provide `en-US` and `zh-Hans` metadata for every user-visible provider,
   Action, authentication method, credential field, option, category, tag,
   scope, health-probe description, and input field. Use
   `category_labels_i18n`, `tag_labels_i18n`, and `scope_labels_i18n` for
   stable identifiers; use `title_i18n` and `enum_labels_i18n` in input JSON
   Schema properties. Registration fails when a supported locale or declared
   enum value is missing, so a new provider cannot silently leak English or
   technical identifiers into a Chinese interface.
4. Give every Action strict Draft 2020-12 input/output schemas, caller support,
   required scopes, effect, risk, data-egress destination, idempotency, and a
   conservative default policy. The non-interactive Agent runtime may only be
   advertised for read-only Actions that do not require interactive approval;
   Registry validation rejects a non-read Action that declares Agent support.
   AIChat may expose write Actions, but the Executor still enforces usage rules,
   current provider scopes, organization policy, and approval.
5. When an Action needs a stable identifier that users normally describe by
   name, add `preparation_hints` that reference an existing read-only Action.
   Declare the target argument names and safe result paths instead of adding
   provider names or special cases to the generic Connected Apps prompt.
6. Implement `Adapter.Execute`; optionally implement connection validation,
   health probing, credential validation, and dynamic governance. For OAuth
   methods also implement the provider OAuth contract for authorization URL,
   code exchange, profile resolution, refresh, and revocation. Keep base URLs
   fixed or allowlisted and normalize provider output.
7. Register the provider in the service container only when the external
   integrations subsystem is enabled.
8. Test authentication headers, request mapping, pagination/limits, retries,
   safe errors, output bounds, scope drift, health mapping, and secret
   non-disclosure. Every built-in Action must reject unknown arguments before
   an external request, return value-free diagnostics, and satisfy the shared
   caller contract. Also test both supported locales, nested input-property
   labels, and every enum label. Add a container registration test and an
   AIChat meta-tool execution test.

Do not add a visible Skill for an external application. Put provider-specific
instructions, preparation hints, approval requirements, and schemas in its
Action definitions. Skills remain available for reusable non-integration task
playbooks, but they are not an application-connection mechanism.

## Legacy platform-credential compatibility

New Connections use only `organization` or `account` credential sources.
Provider credentials are never read from provider-specific server environment
variables. The legacy `platform` source and auth identifiers remain internal
constants only so older database rows can be recognized and rejected safely.
They are not advertised in the Provider catalog, accepted by create or update
APIs, returned as usable Connections, or resolved at runtime.

Legacy platform rows cannot be converted automatically because they do not
contain an encrypted provider secret. Create and test an explicit organization
or personal replacement Connection, update grants, AIChat preferences, and
Agent bindings, and then retire the old deployment secret.

## Security and audit behavior

- Credentials are AES-256-GCM envelopes bound to organization, Connection,
  Integration, credential revision, and key ID.
- Organization grants, personal ownership, Action scopes, and current policies
  are checked again immediately before each call.
- Sensitive outbound values and unsafe URLs are rejected before network I/O.
- Quota and initial audit creation fail closed.
- Audit records store bounded operational metadata and an HMAC fingerprint,
  not raw prompts, credentials, webpage bodies, or upstream responses.
- Failed executions and health events may store only bounded provider
  diagnostics: provider error code, request ID, HTTP status, and retry-after
  time. Provider messages, response bodies, headers, URLs, and request
  arguments are never stored as diagnostics.
- Completion updates use a durable Redis outbox so a paid successful call is
  not repeated when the database update temporarily fails.
- Guarded non-idempotent Actions use durable operation receipts. A confirmed
  success is replayed without another provider call when the model repeats the
  same Action for the same target within one originating user message. A new
  user message or a different target remains a new operation. Definite input,
  authentication, and provider-rejection failures release the claim and may be
  retried; a timeout or ambiguous network failure is recorded as
  `outcome_unknown` and must be verified before another send.
- A request that intentionally performs the same Action more than once is
  frozen as a batch before approval. Every item receives a distinct
  `operation_item_id`, receipt, and operation key, even when all items use the
  same Connection, Action, and target. The operation identity is:

  ```text
  HMAC(
    organization_id,
    user_message_id,
    connection_id,
    action_id,
    target_identity,
    operation_item_id
  )
  ```

  `batch_id`, `operation_item_id`, `item_index`, `item_count`, and the
  server-derived `frozen_input_hmac` are stored with the receipt. Raw message
  bodies are not stored. The database uniqueness boundary remains
  `(organization_id, operation_key)`.
- Batch approval binds the exact Connection, Action, target set, item count,
  and frozen item digest. Changing a recipient, item body, or item count makes
  the previous approval invalid. The approval's public copy shows a bounded,
  credential-redacted summary; internal batch/item IDs and digests remain
  server-only.

  ```text
  approval binding = (
    batch_id,
    connection_id,
    action_id,
    target_identity,
    item_count,
    frozen_items_digest
  )
  ```

- Batch retries are item-scoped. Confirmed successes are replayed and never
  sent again. Only definite `failed_safe` items may be attempted again.
  `outcome_unknown` items are never retried automatically, including after a
  process restart. A new explicit user message such as "send again" creates a
  new batch and is allowed. One message containing ten content entries is one
  operation; ten separately requested messages are ten operation items.
- Batch results aggregate to `pending`, `executing`, `partially_succeeded`,
  `succeeded`, `failed`, or `outcome_unknown`, and report the planned, safely
  failed, confirmed-success, and unknown counts. Retrying a partial batch
  replays confirmed items locally and calls the provider only for
  `failed_safe` items.
- Passive runtime outcomes and explicit manual tests update orthogonal health,
  auth, and scope states with bounded history. No periodic provider request is
  issued for health monitoring.
- Approval prompts for dynamic external Actions show the actual provider,
  Action, Connection, external destination, and a credential-redacted summary
  of the data that will be sent—not merely the generic meta-tool name.
- Remembered approvals for dynamic Actions are scoped to the provider Action
  and exact Connection. Guarded side effects such as sending a message are
  additionally scoped to the external target, so approval for one recipient
  cannot authorize another.

## WeCom, DingTalk, and standard mail connectors

The enterprise connector batch includes WeCom, DingTalk, and one
provider-neutral standard mail connector. QQ Mail and NetEase Mail remain
connection methods of Standard Mail rather than separate Providers. Their
server endpoints, TLS modes, and ports are selected on the server, so users
only enter the mailbox address and the provider-generated authorization code.
Other standards-compliant services remain available through the advanced
custom IMAP/SMTP method. These connectors are available whenever
`EXTERNAL_INTEGRATIONS_ENABLED=true`; no provider-specific deployment
environment variable or database migration is required.

- WeCom uses an organization-owned custom application. Administrators enter
  its Corp ID, Agent ID, and Secret. The connection can inspect the application,
  list departments, resolve visible members, read a resolved member, and send
  one plain-text application message after approval. The application visible
  range remains the provider-side security boundary.
- DingTalk uses an organization-owned internal application. Administrators
  enter its AppKey, AppSecret, and AgentId, configure the application's contact
  visibility, and grant only the contact, attendance, and work-notification
  permissions needed by enabled Actions. The connector provides 12 Actions:
  department list/search/detail/member listing, member search/detail, role and
  role-member listing, bounded attendance records, member and department work
  notifications, and delivery-result checks. Department, role, member, and
  notification references are bound to the exact Connection. Attendance is
  limited to one resolved member and seven days per call and omits exact
  coordinates, addresses, photos, and remarks. Notification Actions are
  disabled by default and require explicit approval; submission is reported as
  `pending`, never as confirmed delivery. Department delivery queries report
  partial success separately from full success or failure.

### WeCom connection checklist

1. In the WeCom administration console, create an organization-owned custom
   application under **Applications**. A group robot webhook is not compatible.
2. Copy the **Corp ID** from **My Enterprise > Enterprise Information**.
3. From that same custom application, copy its numeric **AgentID** and
   **Secret**. Do not use a contacts Secret, callback Token, or EncodingAESKey.
4. Configure the application visibility range to include every department or
   member ZGI must resolve.
5. When trusted-IP protection is enabled, add the public egress IP of the
   server running the ZGI API. The provider sees the backend server egress IP,
   not the browser address or `localhost`. WeCom error `60020` normally means
   this IP is missing from the trusted list.
6. Save the Connection. ZGI automatically obtains an application token and
   reads the application profile; this check never sends a message.

The Corp ID, AgentID, and Secret must identify one application in one
enterprise. A successful token request with a failed application-profile check
usually indicates an AgentID mismatch or an application visibility/trusted-IP
restriction rather than a ZGI credential-storage failure.

### DingTalk connection checklist

1. Select the correct organization in the DingTalk developer console and
   create an **enterprise internal application**. A group robot webhook is not
   compatible.
2. Copy **AppKey**, **AppSecret**, and the numeric **AgentId** from that same
   application.
3. Before testing, grant department and organization-contact read access. The
   ZGI connection check obtains an application token and reads the root
   department list, so this minimum read access is required even when the
   eventual goal is only to send a notification.
4. Configure the application's availability/visibility range, create and
   publish an application version, and confirm the application is enabled in
   the selected organization.
5. Add attendance permissions only for attendance Actions, and work-notification
   send/status permissions only for notification Actions.
6. If the DingTalk security settings enable a source-IP allowlist, add the
   public egress IP of the ZGI API server.
7. Save the Connection. ZGI automatically tests token exchange and the root
   department query; it does not send a notification.

When either enterprise connector fails validation, Connection Center records
the provider error code, bounded request ID, and HTTP status without exposing
the Secret or raw provider response. Use those diagnostics to distinguish
wrong credentials, missing provider permissions, an unpublished application,
and an IP allowlist rejection.
- Standard Mail offers three guided connection methods: QQ Mail, NetEase Mail
  (`163.com`, `126.com`, and `yeah.net`), and Other mailbox (advanced). QQ and
  NetEase users enable IMAP/SMTP in their provider settings and paste a
  dedicated authorization code, never the normal mailbox password. NetEase
  sessions also send the provider-required IMAP client identity. Advanced
  connections accept validated public DNS endpoints: IMAP uses implicit TLS on
  port 993, while SMTP uses TLS on port 465 or STARTTLS on port 587. All
  resolved addresses are checked and the connection is pinned to a validated
  public address to close DNS-rebinding gaps. Existing advanced Connections
  remain compatible and require no migration.
- The connection-completion flow now includes an explicit Action review before
  AIChat selection. Administrators can enable the required organization Action
  policies without leaving the flow, and shared Connections still require a
  matching usage rule. Mail send and reply are available by default for new
  organizations but remain high-risk, AIChat-only Actions that require explicit
  confirmation on every execution. Connections completed by the earlier setup
  contract are prompted once to review these settings; no credential or schema
  migration is required.
- Standard Mail supports account inspection, folder listing, bounded
  search/read, plain-text send, and reply. Creating a Connection tests both
  IMAP and SMTP authentication but never sends a test message.
- Send and reply Actions are disabled by default, AIChat-only, high-risk, and
  require explicit approval. They use durable success-only operation receipts.
  Once SMTP enters the DATA phase, an interrupted or ambiguous response is
  recorded as `outcome_unknown` and is never retried automatically.
- SMTP acceptance means the provider accepted the message for delivery; it is
  not proof that the destination mailbox ultimately received it. The connector
  returns a generated RFC Message-ID and the accepted envelope recipients.

## Troubleshooting

- Provider missing: verify `EXTERNAL_INTEGRATIONS_ENABLED=true` and restart the
  API.
- Cannot create a Connection: choose an available authentication method and
  configure a valid active credential key.
- OAuth button says configuration is required: an organization administrator
  must save that provider's OAuth client ID/secret and register the exact
  callback URL shown by Connection Center.
- OAuth application credentials are managed separately from connected
  accounts. After initial setup, administrators can reopen **OAuth application
  settings** from the provider card, rotate the write-only client secret, or
  change the client ID. The previous secret is never returned to the browser.
- Removing an OAuth application configuration is blocked while account
  Connections or pending authorization flows still depend on it. The settings
  dialog shows the current dependency counts before enabling removal.
- OAuth window was closed or expired: restart the connection. Flow references,
  state, and PKCE verifiers are intentionally short-lived and cannot be reused.
- OAuth succeeded but the shared account is unavailable: add an explicit usage
  rule for the organization, workspace, or member; connection success does not
  grant shared access.
- Reconnect required: the refresh token was revoked/expired or the provider no
  longer reports required scopes. Reauthorize the same Connection or run a
  scope upgrade; ZGI does not silently use a different account.
- Connection is not selectable: test it, resolve reconnect/scope warnings, and
  confirm a matching organization/workspace/account grant exists.
- AIChat selection disappeared: preferences are authoritative and stale or
  revoked Connections are removed rather than silently retained.
- Action denied: check the grant Action allowlist, read/write mode, provider
  scopes, and organization Action policy.
- Provider rejected the request: inspect the safe provider error code, HTTP
  status, request ID, and retry-after time in the execution or health view.
  Access and provider business-rule failures are not reported as generic
  upstream outages.
- Feishu message denied: confirm the published app version contains the
  Action's AllOf/AnyOf permissions, its availability scope includes the
  account or chat, bot capability is enabled when applicable, and the bot is
  present in the target group. Reconnect or use **Grant access** after changing
  OAuth permissions.
- Health is unknown: no successful probe or runtime signal has been recorded;
  “unknown” must not be interpreted as healthy.
