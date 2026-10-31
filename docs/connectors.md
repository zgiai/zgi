# Personal Connectors

Personal Connectors give one ZGI account a direct, reusable connection to an
external service. GitHub and Exa use personal Keys. Feishu / Lark additionally
supports a versioned self-built application credential contract and controlled
QR registration. DingTalk, WeCom and WeChat use a versioned messaging
credential contract. Gmail supports IMAP/SMTP with a Google app password;
QQ Mail and 163 Mail use their respective client authorization codes.
Connections remain personal: no organization-shared credentials,
general-purpose OAuth editor, or editable provider permission policies are introduced.

### GitHub · MCP trial

**GitHub · MCP** is a separate, read-only Connector alongside the existing GitHub
REST Connector. Choose it in the Connector catalog, provide a GitHub PAT with
access to the target repository's Issues, and connect. Setup verifies the account
through GitHub's official remote MCP server and checks that its issue tools are
available. The PAT uses the existing personal encrypted Key lifecycle.

Bind the connection to a chat and select it there. For example, ask: “查看
`gin-gonic/gin` 最近更新的 5 个未关闭 Issue，列出标题和链接，再详细说明其中一个。”
The chat can list open Issues in descending update order and read one Issue's
details. Execution passes through the normal Connector binding, authorization
and audit path. The [MCP guide](mcp-client.md) records the endpoint, tool
allowlist, read-only headers and focused tests.

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

## Catalog categories and tags

The catalog keeps its existing **Connector / Data Source** sections. It also
shows one platform-curated product category and a small set of discovery tags.
Categories describe the main user purpose: **Social & communication**,
**Productivity tools**, **Information retrieval**, **Developer tools**, or
**Other tools**. Tags add cross-cutting terms such as social, messaging, email,
collaboration, documents, search, code and repositories.

The first mappings are Weixin as Social & communication; WeCom as Social &
communication with enterprise/messaging/collaboration tags; DingTalk and Feishu
as Productivity tools; Gmail, QQ Mail and 163 Mail as Social & communication with
the email tag; GitHub as Developer tools; and Exa Web Search as Information
retrieval. These labels describe public product positioning and the capabilities
currently integrated by ZGI. They do not grant access, change risk, or bypass
confirmation. Unreviewed Connectors use Other tools with no tags.

Sources used for the initial review include [Tencent's Weixin product page](https://www.tencent.com/zh-cn/products/weixin-wechat/),
[Tencent's WeCom product page](https://www.tencent.com/zh-cn/products/wecom/),
[Feishu documentation](https://docs.feishu.cn/docs), [DingTalk](https://www.dingtalk.com/),
[GitHub documentation](https://docs.github.com/en/get-started/start-your-journey/what-is-github),
and [Exa Search documentation](https://exa.ai/docs/reference/search). Mail mappings
follow the providers' [QQ Mail help](https://help.mail.qq.com/detail/106/985) and
[NetEase help](https://help.mail.126.com/faqDetail.do?code=d7a5dc8471cd0c0e8b4b8f4f8e49998b374173cfe9171305fa1ce630d7f67ac22532c23a1d70ca26)
and Google's [Gmail learning center](https://support.google.com/a/users/answer/9259748).

## Runtime setup

Startup restores validated, digest-checked Connector definitions from the database.
Older accepted schema `1.3.0` / contract V4 definitions retain the
`digital_human` surface alongside Chat, Agent, and Workflow, with their original
authentication family and contract digest. The newer `serverops_ssh` V4 built-in
declares the `digital_human` surface only: its SSH credential dialect has no
earlier published base to project onto. The `digital_human` surface supports a
runtime binding target for the phase-one operations specialist. That target is
admitted per Connector: retaining the surface on an existing definition, or
admitting a DigitalHuman-only built-in, does not by itself add the capability,
grant runtime access, or change any existing connection or binding.
If a bundled package cannot compatibly replace an accepted version, startup keeps
the accepted version and reports the rejected update in the Connector admission
log. Invalid stored contracts and database errors still fail startup.

### SSH Server Operations Connector

The SSH connector creates and verifies a per-owner Linux server connection using
the server `host`, `port`, `username`, and exactly one authentication method:
password or SSH private key (with an optional private-key passphrase). The same
setup panel is available in Connector Center and can be opened in place from an
Agent or DigitalHuman connection picker, so users do not have to leave the
current task to create a credential.

For a new connection the panel recommends a platform-generated key (ADR 0030).
The browser generates an Ed25519 pair, keeps the private key hidden in the
private-key field, and shows the public key with a one-line command that adds
it to the login user's `~/.ssh/authorized_keys`. The server can keep password
login disabled, and the connection is revoked by deleting that line.

Host identity verification is automatic by default. The first test connects
with the submitted credential, reads the server's SHA256 host-key fingerprint,
and asks the user to confirm the observed identity before saving. The confirmed
fingerprint is pinned and checked on every later connection. If an operator
already has a trusted fingerprint, it remains available under Advanced settings
for strict verification during the test; the normal form does not require users
to find and paste one. A changed host key blocks the connection and requires a
new test and explicit confirmation.

The private key, password, and passphrase are write-only setup values. They are
used for the probe and final verification, then cleared from the form and are
never placed in URLs or browser storage. The connection becomes usable only
after the user confirms the server identity and the strict credential save
succeeds.

The setup panel also asks for the connection's command privilege, stored as the
optional, non-secret credential field `privilege`:

- **Run as the SSH user** (the field is omitted): every command runs as the SSH user.
- **Use sudo (no password)** (`privilege: "sudo"`): the service write actions run
  behind `sudo -n`, so a server that asks for a password fails the action instead
  of waiting. The server must allow that user to run `systemctl` through sudo
  without a password. The read actions never use sudo.

The privilege is part of the connection; changing it means reconnecting with the
new choice. It is recorded with the verified connection as non-secret evidence,
so the confirmation preview can show it without reading the credential.

One DigitalHuman can bind multiple independently named, verified server connections
and select an operation subset for each. When a request could match more than one
bound server, the operations specialist template instructs the agent to ask the
owner to clarify the target instead of choosing one. That is template guidance,
not a platform-enforced deterministic selection, so name the intended target in
the request.

The connector offers seven fixed operations, each for at most one systemd unit
named `*.service`:

| Operation | Effect | Command that runs |
| --- | --- | --- |
| `server.system.snapshot` | read | fixed read-only snapshot of hostname, `/etc/os-release`, `uname -sr`, `/proc/uptime`, `/proc/loadavg`, `/proc/meminfo`, and `df -P -k` |
| `server.service.status` | read | `systemctl show --no-pager --property=LoadState --property=ActiveState --property=SubState --property=Description -- '<unit>'` |
| `server.journal.tail` | read | `journalctl --no-pager --quiet --output=cat --unit='<unit>' --lines=<1-200, default 100> --since=-<1-1440, default 60>min` |
| `server.service.restart` | write | `systemctl restart -- '<unit>'` |
| `server.service.start` | write | `systemctl start -- '<unit>'` |
| `server.service.stop` | write | `systemctl stop -- '<unit>'` |
| `server.service.reload` | write | `systemctl reload -- '<unit>'` |

With the sudo privilege, a write command becomes, for example,
`sudo -n systemctl restart -- 'nginx.service'`. After a write command succeeds,
the action reads the unit again (unprivileged) and returns its `load_state`,
`active_state`, and `sub_state`. A failed write command returns a provider
error; its bounded error text is kept for operators and is never shown with
credentials. When sudo or systemd refuses the SSH user, the failure is
`connector_provider_permission_missing`, and the person is told the account
lacks the permission rather than that the server is unavailable. No operation accepts a command, argument, path, or other free-form
input.

Every invocation, read or write, is high risk. It shows a complete target and
action preview, including a **Command** field with the exact command line that
will run, rendered as code in the connector confirmation card of the
conversation and of DigitalHuman run details. The preview is built by the same
code that builds the executed command, so the two cannot differ. Each call
requires fresh owner approval unless the connection lets proven read-only
commands run (below); no session grant is created or remembered. Scheduled
and background DigitalHuman runs reach a server only for inspection (ADR 0027):
the owner must give the connection a task grant, and each call must be proven
read-only when it runs. Changes are never unattended.

When a DigitalHuman confirmation is decided, in the console, by a channel
reply or through the external API, the same turn continues: the DigitalHuman
receives the operation's result, exactly as a call without confirmation would,
and answers in the same message. A cancelled or failed operation is reported as
such and is not retried with the same arguments. Running the operation never
ends the task by itself; a channel receives the answer, or the next
confirmation prompt, instead of a fixed outcome text.

Write actions are opt-in. When a server connection is added while creating a
DigitalHuman, only the read operations are selected by default; each write
action must be ticked explicitly.

Release `1.2.0` adds **Run a shell command** (`server.command.run`, ADR 0025).
It runs one command exactly as written, as the SSH user, for at most 45
seconds (longer jobs are started detached and checked with later commands),
and returns the exit code, stdout, and stderr (each keeping its last 15,000 characters, with the
connection's secrets redacted). The command is at most 8,192 bytes and may not
contain control characters other than tab and newline, or invisible format
characters. The connection's sudo privilege does not apply; a command that needs
root writes `sudo -n` itself. The platform does not check what a command does:
it can read, change, or delete anything the account may. It is never selected by
default, ticking it asks the person to accept that risk, and every run shows the
whole command for approval. Enable it only on a test server or a dedicated
account with limited permissions. A connection verified before `1.2.0` offers it
after reconnecting.

A DigitalHuman's SSH connection card also chooses how calls ask (ADR 0026):

- **Always ask me** (default): every call waits for approval.
- **Run read-only commands without asking**: a call the platform proves
  read-only runs at once and is still recorded in the run's process; every
  other call waits for approval as before.

The proof is a parse of a narrow shell grammar against an allowlist of read
commands with safe arguments, such as `systemctl status`, `journalctl` without
maintenance flags, `ls`, `cat`, `tail` without `-f`, `grep`, `find` without
`-exec` or `-delete`, `git status/log/diff`, `docker ps/logs/inspect`, `ss`,
`ps`, `df`, and `curl` GET requests to a local address. `$` expansion,
backticks, subshells, `sudo`, any redirection other than `2>&1`, `>/dev/null`,
or `< file`, and reading credential files such as `.env`, private keys, or
`/etc/shadow` always ask. Arguments count, not only the program: `ss -K`,
`getent shadow`, `jq env`, and recursive `grep` or `diff` ask, and a shell
wildcard is read-only only for programs that report names, sizes or checksums
(`ls`, `du`, `stat`, `wc`, …) with their display flags, never for one that
prints file content or reads a list of paths from a file (`--files0-from`). The snapshot, service status, and journal actions are
read-only. Commands that would destroy the server or cut off access, such as
`rm -rf /`, `mkfs`, `dd` to a disk, or `reboot`, are refused outright and never
reach approval.

The card also offers full trust (ADR 0028), each option behind its own risk
confirmation:

- **Fully trusted (no asking while I am present)**: while the connection's owner
  is the person in a console conversation, every call runs without asking,
  changes included. Conversations by anyone else, channels, and the API still
  ask.
- **Fully trusted, including scheduled tasks**: additionally, the owner's
  scheduled and background task runs make any call unattended, without a task
  grant.

Forbidden commands are still refused and every call is still recorded. Trust is
pinned to the connection revision, credential, and contract it was given for:
reconnecting, rotating the credential, or a new connector contract returns the
connection to asking until the owner trusts it again.

Organization owners and administrators can add server command rules on the
dashboard's organization settings page (ADR 0029). A rule is a command prefix,
such as `git push` or `systemctl restart postgresql`, with an action:

- **Forbid**: the command is refused before approval, whatever the connection's
  policy.
- **Always ask**: the command needs a person's approval even when the
  connection runs read-only commands without asking or is fully trusted, so it
  never runs in a scheduled task.

A rule matches the program and then its other words in order, through wrappers
such as `sudo` and through the fixed service actions. A command that cannot be
parsed is matched on its words, erring toward matching. Rules only tighten; they
never let a command run without asking.

Scheduled and background runs have no one to approve (ADR 0027). A task grant
on the DigitalHuman's SSH connection can cover only the snapshot, service
status, journal, and shell command actions; the service write actions cannot be
granted. With such a grant, an unattended call runs only when it is proven
read-only, by the same rules as above, and consumes one use of the grant. A
change, or a command that cannot be proven read-only, is blocked before the
grant is consulted, so an unattended specialist inspects and reports and leaves
every change for a person to approve. Such a blocked change does not end the run: the specialist
finishes its report, and the answer names the change that did not run.

A DigitalHuman task can also carry a **server alert** (ADR 0031). The platform
checks up to ten systemd services on one of the DigitalHuman's SSH connections
every 1 to 60 minutes, without a model, through the fixed read-only service
status action. When a service is not `active`, or the server cannot be reached
or logged in to, for the chosen number of checks in a row, the watch starts the
task once for that outage with the trigger `server_event`. The run's
instruction names what went down; the run itself is an ordinary unattended task
run under the rules above. Only the connection's owner points a watch at it,
and anyone who may change the task can turn it off.

Release `1.1.0` of the connector adds the write operations and the `privilege`
field. Existing installations accept it through the ordinary compatible-update
check at startup: the accepted `1.0.0` contract stays valid and pinned, its
three read operations are unchanged, and the one reviewed authentication change
(adding the optional `privilege` field) is pinned by digest. Existing connections
keep working with their read operations; reconnect a connection to verify it for
the new write operations or to choose sudo.

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

The fixed `feishu-app-template-v3` (profile 3.0.0, plugin 1.2.0) requests 40 application scopes, `offline_access`, six
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

The v3 permission review preserves the previous 34 application scopes and adds
`tenant:tenant:readonly`, `im:message`, `im:message.group_msg`,
`im:chat.members:read`, `directory:employee:search` and
`directory:employee.base.name.name:read`. The template already included
`im:message.p2p_msg:readonly`, `im:message:readonly` and `im:message:send_as_bot`.
All these scopes belong under `addons.scopes.tenant`, with literal underscores.
The ordinary user grants remain unchanged; `cardkit:card:read` remains excluded.

Permission names and identity requirements were checked against the official
[tenant information](https://open.feishu.cn/document/server-docs/tenant-v2/query),
[message reception](https://open.feishu.cn/document/server-docs/im-v1/message/events/receive),
[chat history](https://open.feishu.cn/document/server-docs/im-v1/message/list),
[message sending](https://open.feishu.cn/document/server-docs/im-v1/message/create) and
[chat member](https://open.feishu.cn/document/server-docs/group/chat-member/get) documentation.
In particular, app-identity group history additionally requires `im:message.group_msg`,
a sensitive permission; the bot must also belong to that group. Adding the scope
does not bypass Feishu's approval or resource access checks.

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
diagnostics to verify them. The v3 registration template is a fixed baseline;
adding runtime actions or supported host surfaces does not change its permissions.
The reviewed v2 → v3 transition pins the complete old and new authentication
contracts (credential fields and setup configuration), independently of product
release names. Other identity, existing-operation, permission, risk, surface and
same-version checks still apply. Unknown templates, changed authentication
configuration and reverse template transitions remain incompatible. The historical
v1 → v2 reduction retains its existing exact-release rule. Catalog admission neither
changes saved credentials/bindings nor makes external configuration calls or
expands an existing connection's authorized operations. Existing registration receipts
keep their original template and idempotency key. Active flows pinned to an older template stop instead
of silently switching templates or creating another app; check Feishu for any app
already created before explicitly starting again or using manual entry.
For an existing app, explicitly choose **Configure application permissions** in
connection management to request the new template. Complete the provider's approval
and publishing requirements, then verify the connection again. Permission changes
require a published version and administrator approval under the official
[self-built application lifecycle](https://open.feishu.cn/document/home/introduction-to-custom-app-development/self-built-application-development-process).

The native driver currently exposes 38 bounded operations covering bot and tenant identity,
chats and their user members, messages, existing document blocks, comments, reactions, pins, CardKit
creation/update, file metadata, Wiki nodes, basic contacts, name search and IM resources.
Batch messages are limited to 20 distinct Open IDs; system messages use only the
single-user conversation-divider template. Resource operations accept explicit
message resource IDs or authorized platform file IDs, not arbitrary URLs. Files
are limited to 10 MiB and JPEG, PNG, WebP, PDF or plain text; downloads become
private platform references rather than model-visible binary or storage URLs.
Card reading is outside this release's operation scope; `feishu.card.get` is
not registered, and the current template does not request `cardkit:card:read`.
`cardkit:card:write` remains for the existing card creation/update operations.

**Find users by name** (`feishu.contact.search`) calls the official
[Directory employee search](https://open.feishu.cn/document/directory-v1/employee/search)
with `tenant_access_token`, `employee_id_type=open_id`, and only
`base_info.employee_id` / `base_info.name.name` requested. It requires the search
scope and either the narrow name-reading grant or an already-approved basic-info grant.
The QR template requests only the narrow name grant, not phone, email or HR fields.
It returns one page of candidate names and Open IDs (20 by default, at most 50),
plus an explicit continuation token. Same-name users remain separate candidates;
no recipient is chosen automatically and no message is sent. Field permission
failures are not treated as no matches. Results obey the app's directory data
permissions and do not include external or departed users. The older
[user search](https://open.feishu.cn/document/server-docs/contact-v3/user/search-users)
requires `user_access_token` and is not used as a fallback.

**Get tenant information** (`feishu.tenant.get`) queries the selected application's
own tenant using `tenant:tenant:readonly`, with no arbitrary tenant input. It returns
the name, display ID, tenant key and, when provided, tenant tag; it does not return
avatars or request the additional domain-reading permission. Both
[Feishu](https://open.feishu.cn/document/server-docs/tenant-v2/query) and
[Lark](https://open.larksuite.com/document/server-docs/tenant-v2/tenant/query)
document the app-token endpoint.

**List chat members** (`feishu.chat.members.list`) takes a chat ID and optional
page size/cursor, fixes `member_id_type=open_id`, and returns names/Open IDs of
user members, not robots. Any one of `im:chat`, `im:chat:readonly`,
`im:chat.members:read` or `im:chat.group_info:readonly` is sufficient at the scope
layer; the existing template already requests `im:chat.members:read`. The app's
bot must already be enabled and in the target chat; internal chats must belong
to the same tenant. It never joins a chat or sends a message automatically.

The [Feishu](https://open.feishu.cn/document/server-docs/group/chat-member/get)
and [Lark](https://open.larksuite.com/document/server-docs/group/chat-member/get)
member APIs default to 20 and allow a requested page size up to 100, but members
joining together can make the actual page larger. ZGI preserves the complete
page within its local 1,000-member/1-MiB protection limits rather than silently
cutting it to 100 and advancing the cursor. An oversized or malformed page fails
without a success cursor. `has_more`, not page length, determines continuation;
short or empty pages can have more results. One action fetches one page. Missing
or non-advancing continuation tokens fail; the stateless driver does not claim
to detect longer cursor cycles across calls. Member totals and tenant keys are
not exposed as user-list metadata.

These two reads add operations, not permissions: the v3 template remains at 40
application scopes. Missing/inaccessible tenants, invalid/dissolved chats, bot
membership and application-state failures are not successful empty results or
automatically classified as invalid App Secrets. The plugin's additive operation
upgrade does not expand existing Connection or Binding allowlists. Reverification
alone may therefore leave new actions unavailable on older connections; published
Agent/Workflow bindings are never rewritten. Grant verification and local runtime
authorization remain separate checks.

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

### Gmail with a Google app password

Choose Gmail in **Connectors**, enter the original Google login email and a
16-character Google app password, then select **Read mail**, **Send mail**, or
**Read and send**. Grouping spaces copied from Google's password display are
accepted. Do not enter the Google account's ordinary password. ZGI does not
require an OAuth client for this connection and does not use this password with
the Gmail API.

Google requires two-step verification before creating an app password. Some
organization accounts, accounts configured with security keys only, and Advanced
Protection accounts cannot use app passwords. ZGI cannot query eligibility in
advance: follow Google's [app password instructions](https://support.google.com/accounts/answer/185833)
and applicable administrator policy. Personal Gmail accounts do not need the
old “enable IMAP” step; see Google's [mail client instructions](https://support.google.com/mail/answer/7126229).
Google recommends OAuth for applications that support it; this Connector's
current authentication method is explicitly the app-password option.

The **Email** entry groups Gmail, QQ Mail and 163 Mail. Choose a provider and enter
its mailbox address and app password or client authorization code. New connections
always enable reading and sending; there is no separate use selector. Both IMAP
and SMTP authentication must pass before the connection is saved. Setup does not
send a test email, and partial verification never falls back to a single direction.

Existing read-only and send-only connections remain unchanged when viewed,
renamed or diagnosed. Their **Reconnect and enable read and send** action upgrades
the same login only after both checks succeed. Failure leaves the saved credentials,
uses, bindings and other restrictions intact. The API retains the historical
`read`, `send` and `read_send` values; omitting use on credential maintenance retains
the old use. These local restrictions do not change the provider password's scope.
Dotted addresses, plus addresses, Googlemail addresses and Workspace aliases are
not merged into another saved identity.

Email account names show a provider prefix, such as `QQ Mail-Work mailbox`. If no
custom alias was set, the name uses the authenticated login's local part. The full
login remains visible separately. Prefixes are presentation only: language changes
do not rename saved records. The combined entry does not combine credentials,
connection IDs or target bindings. Chat and Agent can retain selections from
multiple mail providers; Workflow operations still require their own provider.
Existing provider detail links open the unified email view, while connection UUID
management links continue to identify the original connection.

The driver connects only to `imap.gmail.com:993` and `smtp.gmail.com:465` using
verified TLS. The protocol driver also supports deployment-selected port 587
with mandatory STARTTLS; the default container wiring uses 465. Hosts and TLS
settings are not user inputs. Credentials use the existing encrypted personal
Connection storage, independently of ZGI's notification-mail configuration.

Four operations are available: list folders, list messages, read a message, and
send a plain-text message. Message lists default to 20 and allow up to 50 items
per page. Mailbox reads use read-only selection and do not mark messages read.
Message references are scoped to the owner, connection, credential generation
and target; a changed mailbox UID generation requires listing again. Returned
body text is limited to 256 KiB. Attachment transfer, deletion, label changes,
automatic replies and background mailbox synchronization are outside this release.

Sending uses the existing interactive confirmation and durable invocation claim.
If a recipient is rejected before message submission, the message is not sent to
the remaining recipients. A successful result means **accepted by the mail
server**, not delivered. When submission may have occurred but the final response
is missing, the result stays unknown and ZGI does not resend automatically.
Check the mailbox before explicitly attempting another send. Password revocation
and account policy changes require reauthentication; removing a local Connection
does not revoke the password at Google.

The Gmail definition uses ZGI extension `1.4.0`, canonical contract V5 and
authentication `gmail.app_password`. Deploy a reader that understands this
contract before admitting it. It does not change the meaning of existing
GitHub, Exa, Feishu or messaging credentials. QQ/163 authorization codes use
separate profiles described below. Microsoft authorization and Gmail API OAuth
are outside this implementation.

### DingTalk, WeCom and WeChat

These three built-in definitions use extension `1.2.0`, canonical contract V3,
and registration protocol version `1`. Existing GitHub/Exa V1 and Feishu V2
accepted contracts keep their original encoding and digests. The messaging
Connectors and their supported setup methods are enabled by default. Each
connection still requires user authorization and credential verification.

| Provider | Default setup methods | Explicit operations | Verification boundary |
| --- | --- | --- | --- |
| DingTalk | QR or Client ID / Client Secret | Text/Markdown to specified users or a group | An application token verifies credentials; permissions and recipient eligibility remain checked when sending |
| WeCom | QR or Bot ID / Secret | Read authorization description, list recent sessions, send to a returned session | Signed CLI token plus a verified identity read; session capability is checked separately, and public schema discovery is insufficient |
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
provided. Once WeChat or WeCom returns candidate credentials, their verification
window lasts at most fifteen minutes from acceptance, independently of the old
QR expiry. Waiting does not extend that deadline. All old and new intents share the owner/organization start and active
flow quotas described above. Eight bounded worker loops per API instance share
the database-wide limit of 32 active leases. GET status never advances upstream
registration. Expired or cancelled flows clear their private registration data.

Apply migration `20260906120000_messaging_auth_reliability` before running the
updated API. WeChat/WeCom verification now separates the external probe from
the final database transaction. An encrypted checkpoint is promoted without
calling the provider again, with flow generation, owner and original connection
revision checked. A cancelled flow cannot promote late results. WeChat's first
messages and cursor are saved together and remain available after activation.

**Waiting for verification** keeps a valid candidate for an explicit retry.
If a consuming WeChat request may have reached the provider without a reliable
checkpoint, the flow requires a new authorization; it is not automatically
replayed. Server diagnostics record only a trace ID, stage, error category,
status/code numbers, elapsed time and transport completion flags. They exclude
credentials, URLs, message bodies and raw provider errors. Already-failed flows
whose candidate secrets were deleted cannot be restored by this migration.

Repeated QR authorization has provider-specific semantics. Feishu resumes the
fixed original App ID without resubmitting creation-only name, avatar or `desc`.
DingTalk's official page allows selecting an existing application; select the
original application when updating a connection, because a different AppKey is
rejected. WeCom's original Bot ID/Secret can be updated manually; selecting the
original bot through its QR page still requires validation in the applicable
tenant. WeChat must return a usable session token for the original account;
an “already bound” response alone is not proof of a ready connection.

The platform capability notices describe the current subject and resource
limits. They do not copy Feishu's scope template to other providers or request
extra permissions automatically. New operations require a documented API,
matching identity, permission and resource checks, and separate verification.

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
Connector function descriptions retain their English usage/safety instructions
and add bounded Simplified Chinese Connector/action names for Chinese lexical
matching. Long localized descriptions are not used as extra ranking text: a
phrase such as "does not send messages" must not promote an unrelated read.
This does not change ordinary Skill ranking, aliases, continuation priority or
budget limits, and is not a guarantee of semantic recall for every paraphrase.
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

### Feishu apps awaiting approval

Apply forward migration `20260904160000_connector_pending_setups` before running
the updated API. Existing connections, bindings, credentials and permission
templates are preserved. This migration does not reconstruct old failed attempts
whose App ID was never recorded.

When a valid registration response supplies an App ID but the app is not ready,
the Connector saves a separate, non-secret pending record. Approval, publishing
and availability must be completed in Feishu. The record keeps the original App
ID/region for 30 days; QR sessions keep their own provider-bounded expiry. Closing
the window or cancelling a scan does not delete the pending record or external
app. Removing the pending record cancels only local recovery, and terminal receipts
are retained for seven days. Capacity is reserved before external creation (20
pending records per owner, 200 per organization, including active new creations).

Find pending records in the Connector catalog, Feishu details, My connections, or
the Chat/Agent Connector menu. They are not selectable connections or defaults and
cannot be used by tools or bound to a conversation. Resuming stays in the existing
setup panel. A GET reads local state only; “last verification” is not live approval
status. Successful verification atomically creates one formal connection; only a
still-current user selection can subsequently attach it through the normal host save.

Same-app QR resume is **enabled for both Feishu and Lark**. From the pending
record, select **Scan to resume this app**, then explicitly start the scan.
Opening a record never starts registration. The request fixes the original App ID,
omits `createOnly`, and supplies only `addons={preset:false}` instead of the creation
template. A different returned identity or region is rejected; there is no fallback
to app creation. Existing expiry, ownership, revision and one-active-attempt checks
still apply. Approval and readiness must pass verification before a connection is saved.

Real-tenant validation of the provider's configuration/permission/Secret side
effects is still outstanding; enabling this feature is not proof of zero side effects.
Review the app identity and proposed changes on the official confirmation page.
The manual fallback verifies the original, locked App ID/region with an App Secret from that
app's developer console, under **Credentials & Basic Info**. Failed candidate
Secrets are discarded, not retained for approval polling. Successful credentials
use the existing encrypted `connector_connections.encrypted_credentials` store;
there is no renewal-token object or browser credential cache.

Existing connections use **Update application Secret** in their management page;
the same-app form keeps identity fixed and verification failure preserves the
current credentials and bindings. Permission configuration remains a separate
action. A `pending_setup_id` detail link opens the pending view without starting a
scan. Legacy `connection_id` detail links resolve ownership and open the original
manual credential dialog through `action=credentials`. They cannot automatically
switch organizations or recreate a deleted connection.

### QQ Mail and 163 Mail

Open **Email**, choose **QQ Mail** or **163 Mail**, then enter the full mailbox
login and its 16-character client authorization code. Reading and sending are
enabled together. Use the mailbox provider's account settings to enable IMAP
and SMTP and generate the code. A normal account password is not this code.
The current profiles accept only `@qq.com` and `@163.com` respectively; enterprise,
Foxmail, 126 and other domains are not inferred from these profiles.

| Profile | Read endpoint | Send endpoint | Credential fields |
| --- | --- | --- | --- |
| QQ Mail (`qqmail`) | `imap.qq.com:993` | `smtp.qq.com:465` | `email`, `authorization_code` |
| 163 Mail (`netease163`) | `imap.163.com:993` | `smtp.163.com:465` | `email`, `authorization_code` |

Both endpoints require TLS with certificate validation. SMTP selects an advertised
PLAIN or LOGIN mechanism after TLS; an authentication rejection does not trigger
another mechanism or endpoint attempt. 163 IMAP identifies the client as ZGI using
IMAP ID. Unsupported mechanisms and client-identity requirements have separate
safe error messages. These new profiles use extension schema `1.5.0`; Gmail keeps
its `1.4.0` definition, original credential field and existing handle format.

The login's local part and plus suffix are preserved, while its domain is
lowercased. QQ authorization codes keep their exact 16-character value; 163
codes also accept whitespace separators copied from the provider page and remove
those separators before authentication. New product connections require both
IMAP and SMTP authentication.
Historical single-direction API clients remain supported. Setup never sends a test email. Sending mail still
requires runtime confirmation; a lost acknowledgement is not automatically
resent. Credential verification does not guarantee later message delivery.

Client codes do not supply separate provider-side read and send permissions.
ZGI enforces the saved use locally. Regenerating or revoking a code happens in
the provider's settings; reconnect the same mailbox afterward. Disconnecting ZGI
removes the local connection and is not a provider-side code revocation.
QQ/163 one-click authorization, QR authorization and platform application
registration are outside this implementation.

Provider instructions: [QQ Mail authorization codes](https://help.mail.qq.com/detail/106/985)
and [NetEase authorization codes](https://help.mail.126.com/faqDetail.do?code=d7a5dc8471cd0c0e8b4b8f4f8e49998b374173cfe9171305fa1ce630d7f67ac286624f309a1a7089).

### Messaging recovery and resource reads

WeChat consuming reads honor bounded server polling hints, with request and
durable lease budgets that cover the long poll. Concurrent readers wait, and a
read whose consumption is uncertain requires recovery rather than automatic
replay. A confirmed QR response without a base URL uses the fixed official origin;
unrecognized origins and IDC routes are rejected. An already-bound QR result
directs the owner back to their connections and does not manufacture credentials.
The optional-field send receipt is accepted only as a valid, recognized JSON
object; malformed or contradictory responses remain unconfirmed.

The WeCom `1.1.0` catalog adds seven read operations while preserving the three
original messaging operation contracts:

| Operation | Fixed official CLI path |
| --- | --- |
| `wecom.documents.search` | `/cli/doc/search` |
| `wecom.documents.get` | `/cli/doc/contents/get` |
| `wecom.calendar.list` | `/cli/calendar/schedules/list` |
| `wecom.calendar.get` | `/cli/calendar/schedules/get` |
| `wecom.calendar.search` | `/cli/calendar/schedules/search` |
| `wecom.todos.list` | `/cli/todo/list` |
| `wecom.todos.get` | `/cli/todo/get` |

These capabilities are conditional on the connected bot and resource access;
discovering an API or verifying bot identity does not verify resource permission.
Existing connections keep their saved usable operations and local restrictions.
Owners must explicitly reconnect and accept new uses before execution. Diagnostics
refresh evidence without admitting newly shipped resource operations.
QQ/163 setup and these WeCom resource reads have protocol-fixture coverage;
successful use with a real account must be validated separately in the deployment.

Resource and cursor references are encrypted, expire after 15 minutes, and are
bound to the owner, organization, connection, credential version, binding and
target. Cursors also bind to the original query. Document results are decoded in
memory with a 256 KiB text limit; a returned file-content field is never opened as
a local path or fetched as a URL. Both documented long-task polling modes are
supported, bounded by cancellation, a 120-second polling budget and a
150-second document operation budget. Polling cannot restart the original task.
Calendar listing does not invent pagination; absent time bounds default to the
next seven days using the deployment's local clock. Partial get results report
unreturned references without claiming that a resource was permanently deleted.

The wire contracts follow the official
[WeCom CLI source](https://github.com/WecomTeam/wecom-cli/tree/78c514b2afee7c0d3d7be715628478421f37ee63)
and [WeChat adapter source](https://github.com/Tencent/openclaw-weixin/tree/2f4dcbf57bedf0e17e266fdedcf0cd2dd141b7d3).
Feishu's 38 catalog operations are grouped for display into messaging, documents,
people and resources, with credential subject and permission alternatives shown
when declared. Its existing 40 tenant-scope template and seven-permission baseline
are unchanged. DingTalk continues to expose its implemented messaging capabilities;
no additional QR-template permissions are inferred from another platform.

### Execution safety

There are no member-facing grants, scopes, action allowlists, audit switches,
approval policies, or risk settings in Personal Connector setup.

Owners of ready WeChat and WeCom connections can restrict their **local uses**
from connection management. Saving uses a revision and idempotency key; clearing
every selection pauses execution. Only operations already accepted for that
connection can be selected. Newly cataloged operations do not automatically
expand old connections. This setting cannot grant provider permissions, change
target bindings, or bypass runtime send confirmation. A revision conflict requires
refreshing the connection before editing again.

- Trusted low-risk operations run immediately when the owner and binding remain
  eligible.
- Interactive high-risk or unclassified operations require confirmation at the
  moment of use unless the current conversation has a matching remembered
  Connector authorization.
- Unattended high-risk or unclassified operations are blocked.
- Connector lifecycle and operation decisions are audited automatically.
- An operation with an unknown provider outcome is not submitted again
  automatically.

These are platform defaults and cannot be weakened by plugin metadata or member
configuration.

Connector and Skill calls share the existing expandable invocation row. Connector
details use their safe operation projection, without generic argument/debug dumps.
In Chat, pending Connector approvals use the existing Tool Governance approval
panel in the conversation input area, including approve/reject and the optional
remember-for-session choice; the timeline does not add a second confirmation card.
Completed, cancelled, failed and uncertain outcomes use the existing Connector
terminal projection without confirmation actions. Ordinary tool session approval behavior is unchanged. Only trusted Connector
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

### Setup guidance and shared selection

Setup keeps decision-relevant help beside its fields: official credential steps,
the fixed read-and-send mail use, and the original identity when reconnecting.
Connection management shows actionable causes for known failures, local-use
scope, permission verification counts, and the specific account being removed.
Removing a ZGI connection does not revoke the provider-side authorization.
Detailed platform instructions remain in expandable help.

Chat Skill and Connector selection share the existing dialog structure. Chat
and Workflow creation share the existing setup content while retaining their
own draft/save protocols. Provider choice remains limited to the directory's
mail family; a host repairing a Gmail selection cannot bind a QQ Mail account.
Agent selection dialogs retain their existing layout and saving behavior.

### Private confirmation preview rollout

The status query accepts optional `presentation_version=1`. With that parameter,
`presentation` contains version 1 and a safe preview, or `preview: null` for an
older record. Without it the response shape is unchanged. Unknown/duplicate
query parameters are rejected. The authorized query is read-only and uses
`Cache-Control: no-store`; pending reads never authorize retrying an uncertain send.

Preview facts are captured from the accepted account and frozen operation input.
Mail shows the sending account, all recipients, subject and a bounded plain-text
body. Other supported operations use explicit field allowlists; conversation
handles resolve through the existing scoped local store. Credentials and context
tokens are excluded. Long content is marked as shortened; an incomplete or altered
account/target prevents the new UI from enabling confirmation. Cancellation remains
available for a verified pending record. Historical message cards are read-only.
Unknown operation shapes also remain unavailable until a safe preview is implemented.

The preview is encrypted with the continuation and is available only through the
owner/organization-authorized query. It is not added to runtime events, message
metadata, Workflow outputs, model context, or shared transcripts. New
continuations always write the v3 envelope with the frozen preview; readers remain
compatible with v2 envelopes created before this behavior was made mandatory.
This follows the existing continuation retention policy and adds no database
columns. Historical v2 pending records have no preview and cannot be confirmed in
this UI; cancel them and initiate a new request.

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

Optional Connector persistence tests use `ZGI_CONNECTOR_TEST_POSTGRES_DSN`:
`go test ./internal/modules/connectors/connections ./internal/modules/connectors/providers/messaging -run Postgres -count=1` from
`api/`. The suite creates a unique temporary schema, validates concurrency,
receipt replay, audit rollback, provider evidence and consuming-read leases,
then removes that schema.
Supply credentials through the environment; do not place them in commands,
logs or committed files. These tests do not replace real-provider acceptance.
