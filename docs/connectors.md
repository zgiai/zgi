# Personal Connectors

Personal Connectors give one ZGI account a direct, reusable connection to an
external service. GitHub and Exa use personal Keys. Feishu / Lark additionally
supports a versioned self-built application credential contract and controlled
QR registration. DingTalk, WeCom and WeChat use a versioned messaging
credential contract. Connections remain personal: no organization-shared credentials,
general-purpose OAuth editor, or member-managed permission policies are introduced.

GitHub and Exa both appear in the **Connectors** section. Exa is shown as
**Exa Web Search** in English and **Exa 网页搜索** in Simplified Chinese: it is an
external service requiring a personal Key, not model-native browsing or a
Data Source catalog item. Search/read operations remain its data capabilities;
those capabilities do not determine its presentation category. Separately
classified Data Source plugins retain their own section and use the same
secure Connection and target-binding lifecycle.

Platform-maintained Connector names, descriptions, Key-field prompts,
capability summaries, selectors, Workflow labels, and runtime confirmations are
localized. Service brands, user-defined Connection names, external account
identities, URLs, code, and provider-returned content remain unchanged.

## Runtime setup

### Feishu / Lark application Connector

Feishu is an external **Connector** for Chat, Agents and Workflows. It is not
an inbound bot channel: there is no listener that forwards Feishu messages to
a ZGI agent, no automatic reply, and no user-token runtime.

Choose **Connect** on the Feishu catalog card to create a personal connection. The existing
connection dialog offers manual `region`, `app_id`, `app_secret` entry or an
explicit QR registration. The browser renders the official URL locally. GET
polling only reads a durable flow; it never starts registration. Closing the
dialog does not delete an application in Feishu. Reopening restores the flow
reference, and uncertain outcomes require explicit recovery or manual entry.
Device codes stay encrypted on the server; App Secrets are write-only and are
saved only after application identity and available operations are verified.

Every successful new setup creates an independent connection. Management separates
**Update application Secret** from **Configure application permissions**. QR
configuration pins the current region, App ID, connection revision and credential
generation. It cannot replace a connection with another application. Manual entry
of a different verified app creates a new unbound connection; existing Chat,
Agent and Workflow bindings remain unchanged. A restored completed registration
requires an explicit use action before attachment to the current host.

The fixed `feishu-app-template-v2` (profile 2.0.0, plugin 1.0.2) requests 34 application scopes, `offline_access`, six
application events, three user events and one card callback. Requested configuration
is not a grant. The detail page shows capabilities once, with an expandable list.
A single collapsed permission section contains application identity and deduplicated
requested/verified scopes. Unverified evidence is never labeled granted. Full configuration-only
events remain in the existing setup template preview. The extra user grants required
by meeting/minutes events are not automatically requested; these events and card
callbacks are **not consumed by ZGI**. Grant evidence comes from the official
tenant-grant API (including its documented legacy response without `scope_type`);
explicit user-subject grants cannot authorize application operations. Feishu's
resource sharing and ACL checks still apply on every operation.

The expandable template preview is available before starting either QR creation
or configuration. Its permission list is maintained by the platform, not editable
by the browser. Following the [official registration protocol](https://open.feishu.cn/document/mcp_open_tools/integrating-agents-with-feishu/create-an-app-in-one-click-go),
only `addons` is encoded as JSON → gzip → Base64 RawURL and placed in the official
verification URL. `preset: false` selects the minimal base without merging the
upstream default permission list. The three user events are additional requested
configuration from the reviewed profile; their required user grants and subscription
APIs appear separately as informational dependencies, not requested scopes or API calls.

Configuration is additive: removing a scope from this template does **not** revoke
it on an existing Feishu app. Manage existing grants in Feishu and re-run connection
diagnostics to verify them. Upgrading this built-in plugin permits only the reviewed
v1 → v2 template reduction; it neither changes saved credentials/bindings nor makes
external configuration calls. Existing registration receipts keep their original
template and idempotency key. Active flows pinned to an older template stop instead
of silently switching templates or creating another app; check Feishu for any app
already created before explicitly starting again or using manual entry.

The native driver currently exposes 35 bounded operations covering bot identity,
chats, messages, existing document blocks, comments, reactions, pins, CardKit
creation/update, file metadata, Wiki nodes, basic contacts and IM resources.
Batch messages are limited to 20 distinct Open IDs; system messages use only the
single-user conversation-divider template. Resource operations accept explicit
message resource IDs or authorized platform file IDs, not arbitrary URLs. Files
are limited to 10 MiB and JPEG, PNG, WebP, PDF or plain text; downloads become
private platform references rather than model-visible binary or storage URLs.
Card reading is outside this release's operation scope; `feishu.card.get` is
not registered, and template v2 no longer requests `cardkit:card:read`.
`cardkit:card:write` remains for the existing card creation/update operations.

Writes use the same native confirmation, authorization, permanent claim and
audit path as other Connectors. Unattended high-risk writes remain blocked and
unknown outcomes are never automatically resubmitted. App creation/configuration
does not exempt subsequent tool execution from these checks.

Apply normal API database migrations before running the updated API. Migration
`20260903110000_create_connector_onboarding` adds flow, short-lived encrypted device
secret and non-secret evidence tables; it does not backfill existing credentials.
The API checks these tables before starting its registration worker. No Docker
dependency is introduced. Flows expire after at most ten minutes, terminal
receipts are retained seven days, and cancellation/removal clears their short-lived
secrets. Admission is transactionally bounded: one active create per Connector,
one update per connection, three active flows per owner, twenty per organization,
five/twenty starts per minute, and thirty-two deployment-wide in-flight leases.

The outer Agent Plugins format stays at 1.0. Only the ZGI extension uses 1.1.0
for `app_credentials`, credential fields, setup descriptor and application-scope
operation requirements. Its V2 canonical contract is separate from existing V1
GitHub/Exa contracts; their accepted bytes, digests, c1 envelopes and bindings are
unchanged. Unknown extensions remain rejected.

Registration protocol reference: [official Go SDK registration scene](https://github.com/larksuite/oapi-sdk-go/tree/v3.11.0/scene/registration).
The driver uses bounded, injected HTTP requests instead of SDK-global clients,
credential caches or implicit retry loops.

### DingTalk, WeCom and WeChat

These three built-in definitions use extension `1.2.0`, canonical contract V3,
and registration protocol version `1`. Existing GitHub/Exa V1 and Feishu V2
accepted contracts keep their original encoding and digests. The messaging
Connectors and their supported setup methods are enabled by default. Each
connection still requires user authorization and credential verification.

| Provider | Default setup methods | Explicit operations | Verification boundary |
| --- | --- | --- | --- |
| DingTalk | QR or Client ID / Client Secret | Text/Markdown to specified users or a group | An application token verifies credentials; permissions and recipient eligibility remain checked when sending |
| WeCom | QR or Bot ID / Secret | Read authorization description, list recent sessions, send to a returned session | Signed CLI token plus actual identity and session reads; public schema discovery is insufficient |
| WeChat iLink | QR only | Pull direct messages, reply with the returned conversation handle | A successful upstream HTTP response is required; a local long-poll timeout cannot make a connection ready |

Use **Connect** to add an independent personal connection. **Update connection
credentials** accepts the same external identity and keeps existing credentials
until verification and commit succeed. Another account or bot requires another
connection. New connections use the existing Chat, Agent and Workflow selection,
default preference and host save behavior. A completed connection is retained if
attachment to the current draft fails.

QR generation begins only after an explicit action. The browser renders an
provider HTTPS payload locally and stores only a flow reference and idempotency
metadata for recovery. Server steps persist initialization, polling and candidate
verification separately. Closing the dialog does not cancel its server flow;
local cancellation prevents late commits but cannot delete a bot already created
at the provider. Unknown registration outcomes require checking the existing flow
and saved connections before starting another one. WeChat verification codes are
write-only, revision checked and limited to three submissions per flow.

Registration lasts at most five minutes, shortened by a provider expiry when
provided. All old and new intents share the owner/organization start and active
flow quotas described above. Eight bounded worker loops per API instance share
the database-wide limit of 32 active leases. GET status never advances upstream
registration. Expired or cancelled flows clear their private registration data.

WeCom session handles and WeChat reply handles expire after one hour and cannot
cross owners, connections, credential generations, targets or binding revisions.
WeChat verification messages are buffered atomically with the cursor; an explicit
pull consumes the buffer before making another upstream request. At most 100
messages and 256 deduplication identifiers are retained per state; buffered
messages older than six hours are excluded from delivery. Encrypted cursor/state
records expire after 30 days without use. Expired handle/state cleanup runs in
bounded batches. Provider cursors, raw recipient identifiers and context tokens
are not returned as tool arguments.

Sends use the existing high-risk confirmation and permanent invocation claim.
An accepted response does not prove delivery. DingTalk preserves its partial
failure lists. Ambiguous sends are never automatically resent. WeCom refreshes
its derived token once only after explicit error `853004`, which rejects that
request. WeChat supports the initial `ilinkai.weixin.qq.com` service origin;
cross-region redirects are rejected until separately implemented and verified.
These operations do not start Stream mode, WebSockets, an inbound gateway,
automatic replies or background Agent execution.

#### Built-in defaults and rollout

No additional environment configuration is required to enable these providers
or start a QR connection. The retired `CONNECTOR_MESSAGING_ADMISSION_JSON`
setting is ignored. Public protocol parameters come from the official adapters:

| Provider | Registration parameter | Allowed QR payload host |
| --- | --- | --- |
| DingTalk | source `DING_DWS_CLAW` | `open-dev.dingtalk.com` |
| WeCom | source `wecom_cli_external` | `work.weixin.qq.com` |
| WeChat | iLink client ID `bot` | `liteapp.weixin.qq.com` |

These are public wire identifiers, not an account's App Secret or token. WeCom
starts at the observed `init` state and follows the official bounded wait for a
successful response containing complete bot credentials. Unknown waiting states
never establish scan confirmation or connection readiness. The current iLink
client identifies itself with `bot_agent=zgi/connector`, `channel_version=1.0.0`
and client version header `65536`. Rejections and network errors are handled when
the user starts a flow; they do not require a separate deployment approval step.

Apply migration `20260903120000_messaging_connector_protocol` after the existing
Connector/onboarding migrations. It preserves old Feishu rows, expands shared
constraints and adds encrypted messaging state/context tables. Upgrade all API
workers before serving the new methods. Old workers do not understand this
protocol, so do not run mixed old/new workers against active messaging flows.
For code rollback, pause setup traffic, drain or cancel active flows, stop new
workers, then roll back code while retaining the forward schema. This is a
coordinated rollback procedure, not automatic mixed-version compatibility.

Protocol fixtures follow pinned official sources:
[DingTalk registration](https://github.com/DingTalk-Real-AI/dingtalk-openclaw-connector/blob/0ccfba47e62dbb3078d066207de26a3df67019cc/src/device-auth.ts),
[WeCom CLI](https://github.com/WecomTeam/wecom-cli/tree/78c514b2afee7c0d3d7be715628478421f37ee63),
[Tencent iLink](https://github.com/Tencent/openclaw-weixin/tree/70ab695f6a1ca87da4102f857a452e2acb6b37cf).
Fixtures validate protocol handling; they do not certify completed real-account
authorization or message delivery.

### Shared configuration

Personal Connectors are available by default. The retired `CONNECTORS_ENABLED`
environment variable is ignored, including when an old environment file sets it
to `false`. Console navigation and `/console/api/connectors/*` routes no longer
depend on a feature switch; authentication and workspace permissions still apply.

Configure the independent keyring for both the migration command and API runtime:

```dotenv
CONNECTOR_CREDENTIAL_ACTIVE_KEY_ID=<opaque-key-id>
CONNECTOR_CREDENTIAL_KEYS_JSON={"<opaque-key-id>":"<32-byte-random-secret>"}
# Optional: trusted package roots, with one plugin.json in each directory.
CONNECTOR_PLUGIN_DIRECTORIES_JSON=["/opt/zgi/connector-plugins/example"]
```

Generate a unique 32-byte value for each deployment and keep the keyring in a
secret manager. Never commit it or expose it through a browser environment
variable. Missing Connector credentials are a configuration error; no other keyring is inherited. The old runtime and its enable switch have been retired.

For a new installation, apply the current database migrations before starting the
API. Existing installations must complete the offline upgrade described below
before restarting. The frontend entry is `/console/connectors`; the ordinary
`start` command does not apply database migrations automatically.

When the frontend and API use different origins (including different localhost
ports), CORS preflights must allow `Idempotency-Key` for Key setup, reconnection,
and target binding, plus `X-Impact-Token` for forced removal. The API includes
these headers in its explicit allowlist; reverse proxies must preserve them and
permit the corresponding `POST` or `DELETE` method. An `OPTIONS` response of
204 alone does not prove the browser will send the mutation: every requested
header must appear in `Access-Control-Allow-Headers`.

At startup, built-in and configured Connector capabilities pass through the
same Agent Plugins admission boundary. Accepted versions are retained in the
Connector catalog state. An invalid configured package is reported separately
and does not hide other valid plugins. Compatible updates preserve existing
Connections and target bindings; incompatible identity or operation-contract
changes are rejected before activation.

The built-in Exa catalog version 1.2.0 corrects its former Data Source
classification. Previously accepted 1.0.0/1.1.0 definitions with the reviewed
unchanged executable contract can upgrade during normal API startup. Existing
Keys, Connection and Binding UUIDs, and historical accepted definitions are
preserved; disabled plugins are not automatically enabled. This catalog
correction does not require a database schema migration or re-entering a Key.

## Connect and use an account

1. Open `/console/connectors` and choose a Connector or Data Source.
2. Choose **Add connection** and enter the Key requested by the provider. The Key is write-only: ZGI verifies
   it, encrypts it after successful verification, and never returns it.
3. Add the ready Personal Connection from the shared picker in AIChat, an Agent
   draft, or a Workflow Tool node. A successful contextual connection is added
   to an empty Connector slot without a second setup or policy step. If that
   target already uses the service, the new Connection is saved but the current
   binding remains until the owner explicitly confirms a switch.

An owner may save multiple independent Personal Connections for the same
service, including different Keys for the same external account. Each has its
own UUID, name, encrypted credential, verification state, capabilities, and
bindings. Starting a new setup creates a new Connection; updating a Key acts on
one existing Connection. Retry idempotency applies to one submitted setup and
does not collapse separately initiated Connections.

In a new AIChat, the plugin picker is available before the first message. Choices
are kept as local draft IDs (never credentials), with one selected account per
Connector. Switching accounts still requires confirmation. The first chat request
may include `initial_connector_bindings`, an array of `connector_id` and
`connection_id` pairs (up to 64). The server validates owner, organization and
readiness, then creates the conversation and its bindings in one transaction,
before capability discovery and model invocation. A failed binding rolls back
that initialization and does not start the model; the draft choices remain for
correction and retry. Existing conversations and replies cannot use this field
to bypass the regular binding revision checks. Starting another chat or changing
account, organization or workspace clears the selection draft.

If Key setup or reconnection loses its HTTP response, the UI reports an
unconfirmed result, clears the entered Key, and does not automatically replay
the submission. Refresh and inspect existing Connections before submitting a
Key again. If the server already returned a verification attempt but a status
read fails, **Check status again** retries only that attempt's read request;
it does not submit the Key again.

Verification distinguishes rejected credentials from insufficient provider
access and provider failures. A permission denial or an unreadable provider
response is not evidence that the Key is invalid.

One ready Personal Connection can be used by its owner across all three
surfaces. A user who can open a shared target does not inherit another user's
Personal Connection. Every run checks the organization, Connection owner,
acting user, target, and binding before credentials are opened.

AIChat and Agent use one current Connection per service for each target. The
selection menu opens beside its Connector button: choose a service in the first
level, then an account in its compact submenu. The current account has a
checkmark; connecting another account and managing the Connector are separate
actions. Hovering or opening a submenu does not change the selection.

The menu keeps the existing selection while another account is pending and
changes it only after explicit confirmation and optimistic-revision validation.
Turning a service off removes only its target binding, not the saved Key. With
multiple eligible accounts, choose one explicitly before enabling the service;
the menu never picks the first account automatically. Workflow
stores the Connection UUID per Tool node, so different nodes may choose
different Connections for the same service. Runtime never falls back to the
first ready Connection or rotates Keys automatically.

In AIChat and Agent turns, the runtime reads current target bindings and
provides the selected operations directly as model-callable functions. It
ranks candidates against the current request and applies the shared tool-count
and schema budgets; a Skill directory's result limit is not an operation limit.
Connector use does not require the model to first search for or activate its
internal Skill group. Array, nested-object, and date/time parameters retain the
admitted operation's JSON Schema. A skipped or detached operation is not
available through an alternative tool name.

Workflow Tool nodes save the native operation identity and selected Personal
Connection. Draft and published execution use that identity with the current
owner-scoped binding checks; they do not require legacy Action metadata.

## Manage a Connection

`/console/connectors/connections` lists actual Connection instances rather than
one summary row per provider. The Connector detail page also shows every saved
instance and always keeps **Add connection** available. Connection name is the
primary label and verified account identity is secondary; Key fragments,
fingerprints, and UUIDs are not used as display labels.

One healthy Connection does not hide another that needs attention: the service
summary includes an independent attention count and remains visible in that
filter. Even if every saved Key has expired, **Add connection** can verify a
new Key without first deleting or overwriting existing Connections.

The Connection page shows only non-secret account identity and health data. It
supports:

- renaming the local Connection;
- diagnosing provider access and supported capabilities;
- replacing the Key after verifying the candidate Key;
- reviewing every target affected by disconnection.

Opening management after setup loads the full Connection detail; the setup
summary is not a substitute for capability or binding data. If detail or
removal-impact data is incomplete, retry loading it. Missing usage data never
means that a Connection has no references or is safe to remove.

Updating a Key preserves the Connection UUID and bindings only when the
provider confirms the same external account. A different account creates a new,
unbound Personal Connection. A failed update leaves the previously verified Key
active.

Normal removal is blocked while active targets reference a Connection. Forced
removal requires a fresh impact review, disables the affected bindings, marks
them as needing replacement, and makes the removed credential unusable.

## Default safety behavior

### Personal defaults for new chats

Each person can choose one default connection per Connector within their current
organization. Defaults can be changed using the star inside a management account item,
or the unchecked “Use by default in new chats” option when adding a connection.
Selecting a connection in Chat or an Agent never opens a default-choice dialog.
Saving a Key alone does not set a default.
Existing saved connections and past conversations are never used to guess a
default. Clearing a preference does not choose another account.

New chat drafts (including contextual sidebar chats) load a default snapshot
without requiring the picker to be opened. The picker shows the current connection
name and availability directly, separately from the new-chat default marker.
Its second-level account menu supports switching, adding another connection and
removing from this target only. The default star there is read-only. It has no
cross-conversation management or third-level default menu. Supported catalog cards
offer direct setup at zero connections. With one or more saved connections,
the primary action is **Manage** and the adjacent dropdown offers only
**Add connection**, including for Feishu application credentials. Unverified capabilities do not mean the
plugin is unavailable; an actually unavailable plugin does not offer setup.

The Connector detail page's **My connections** panel offers **Set as default**
beside **Manage** for each saved connection and marks the current default with a
filled star and **Default** label. Selecting the current default again clears it.

The console currently lists Feishu/Lark, Exa and GitHub for discovery and new connections,
including Chat/Agent connection pickers and Workflow node creation. Other providers
are hidden from these entry points. Saved connections remain manageable and existing
selections remain visible; hiding a provider does not delete credentials, detach
bindings, or disable its backend implementation. The navigation grouping is
**Capabilities → Connectors**, matching the sidebar.

Usage locations, the detail page's saved connections, and capability lists are
capped at 20rem and scroll internally when longer. Their headings, counts, and
add or expand controls remain outside the scrollable content.

Switching or removing a connection from this chat does not change the personal
default or delete its Key. Changing the default does not rewrite an existing chat,
Agent, Workflow, or a manually edited draft. Agents share the picker presentation
but do not inherit personal chat defaults; Workflow nodes remain explicitly bound.

Defaults initialize an untouched draft once. Background preference changes do not
interrupt sending or rewrite the current selection. Loading failures are indicated
on the Connector control without blocking ordinary Chat. Sending or manually editing
the draft freezes the selected identities; late defaults cannot be added. Unavailable
selected connections stay visible for repair or explicit removal, never automatic
account substitution. New clients submit that explicit list without a preference revision;
older clients that provide a revision retain the existing conflict validation. Binding
validation/creation remains transactional before discovery. Defaults are initialization
preferences, never a runtime authorization fallback. A failed opt-in default save is
reported independently and never repeats credential creation or target attachment.

Connector service icons are resolved by stable IDs, not translated names or user
nicknames. Exa, GitHub, Feishu/Lark, DingTalk, Weixin and WeCom use bundled brand marks. Unknown or unsafe
image sources retain a visible local fallback, including during load failures.
Ordinary tools, models, Skills and the shared image component use their existing logic.

Renaming a connection updates its displayed name; same-account Key replacement
keeps its default reference. Deleting the default connection clears that preference
atomically without choosing a replacement. Apply the forward
`connector_chat_preferences` schema migration before enabling this feature in a
deployment. No data is backfilled from saved connections.

### Execution safety

There are no member-facing grants, scopes, action allowlists, audit switches,
approval policies, or risk settings in Personal Connector setup.

- Trusted low-risk operations run immediately when the owner and binding remain
  eligible.
- Interactive high-risk or unclassified operations require confirmation at the
  moment of use.
- Unattended high-risk or unclassified operations are blocked.
- Connector lifecycle and operation decisions are audited automatically.
- An operation with an unknown provider outcome is not submitted again
  automatically.

These are platform defaults and cannot be weakened by plugin metadata or member
configuration.

Connector confirmation reuses the existing operation confirmation card with the
action, destination and necessary risk information, without a separate confirmation
framework. Ordinary tool session approvals are unchanged. Only trusted Connector
events or Connector node execution metadata can select this UI; a tool-returned JSON
object with `state: unknown` does not establish that authority.

`GET /connectors/runtime/confirmations/{receipt}` is authenticated, owner/organization
scoped and read-only. It returns `unresolved` for a pending row or the saved terminal
result. It never invokes a provider, repeats a decision, records a terminal, or resumes
the host. Network/malformed responses are **Result not verified**, distinct from a
server-confirmed **Unknown outcome**. The former exposes a read-only status check,
not a submit-again button. Pending reads cannot prove an uncertain attempt safe to
replay. Receipt state is shared across views for the same owner and terminal evidence
does not regress to a replayed approval request.

## Complete runtime separation and upgrade

Connector owns its catalog, GitHub/Exa HTTP clients, validation, c1 credential envelopes,
bindings and invocation claims. No production or test import uses the retired
integrations module. Host adapters live outside the Connector core.

Bindings include the exact host scope: Chat live, Agent draft or published version,
and Workflow draft or published version plus node. Publishing creates independent
binding identities. Connection and binding restrictions intersect at execution;
changing a Key or replacing a connection cannot remove a connection restriction.

Unavailable references remain visible. Chat can replace or detach them directly.
Agent and Workflow apply repair commands in the same transaction as the host save.
Published versions are immutable; repair the draft and publish a new version.

Upgrade requires a maintenance window and the two offline stages in the
[conversion runbook](../tools/connector-migrate/README.md). Historical schema1 data
is first converted at the fixed checkpoint. The API finalizer then creates a safe
report and an encrypted archive, revalidates the reviewed report under locks, removes
the 22 retired tables and compatibility columns in one transaction, and verifies
schema2 before the runtime starts. A database with unconverted data cannot start
Connector execution.

Personal GitHub and Exa Keys with provable restrictions can convert without Key
re-entry. Organization/shared connections, OAuth/QR, unsupported providers or
unprovable restrictions remain non-executable and require explicit replacement.
Old API and page routes are absent and return the normal 404 response.
There is no dual execution, shadow call, rollout switch or online reversal.

The public [OpenAPI contract](connectors-api.yaml) documents native endpoints.
PostgreSQL upgrade tests require the dedicated `ZGI_CONNECTOR_FINALIZE_TEST_DSN`.
Use an isolated test database.
