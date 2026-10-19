# Built-in social connectors

ZGI exposes X, LinkedIn and Facebook Pages through its native Connector Center.
An isolated stdio MCP bridge executes a fixed set of operations; the normal
connection, resource-binding, confirmation and audit flow remains in control.

## Deployment

The API Docker image includes Node, the bridge and lockfile-pinned production
dependencies. It sets `CONNECTOR_SOCIAL_MCP_SCRIPT=/app/social-mcp/server.mjs`.
Build the API image with its existing `api/` build context.

For source deployments, install Node 22 or newer and run:

```sh
cd api/social-mcp
npm ci --omit=dev --ignore-scripts
npm test
```

Set the API environment to the installed bridge's absolute path:

```dotenv
CONNECTOR_SOCIAL_MCP_NODE=node
CONNECTOR_SOCIAL_MCP_SCRIPT=/opt/zgi/api/social-mcp/server.mjs
```

The executable and script are deployment settings, never member inputs. Empty
script configuration leaves the social bridge unavailable without preventing
the other connectors from starting. Missing Node, script or dependencies causes
connection verification to fail safely.

No packages are downloaded during a tool call. Each invocation gets a new
process and private temporary home directory; the process receives only that
connection's token. No account token is stored in a shared MCP home directory or
passed in a process argument. Continue configuring the existing Connector
credential encryption keyring independently.

## Connecting accounts

1. Create/authorize a developer application at the relevant platform.
2. Obtain an official access token with the endpoint permissions required for
   the intended actions. Browser cookies and vendor passwords are unsupported.
3. Select the platform's `· MCP` entry in Connector Center and supply the token.
4. Verification reads the account identity and records the bridge's implemented
   operations. This does not prove the platform granted every write permission;
   the platform checks each operation at execution time.
5. Bind the connection to the intended chat, agent, workflow or digital human.
   Existing confirmation rules apply. A successful connection is not a grant
   for unattended publishing or private-message sending.

This release uses manual access-token setup. It does not implement a browser
OAuth setup flow or automatic token refresh. Expired/revoked tokens require
reconnection through the existing connection lifecycle.

## Supported operation groups

| Group | X | LinkedIn | Facebook |
|---|---|---|---|
| Identity/account selection | Connected user | Connected member and approved administered organizations | Connected Page |
| Text/image posts | Official API; bounded base64 image input | Company organization author; bounded base64 image input | Reused MCP text/photo handlers; public HTTPS image URL |
| Post/comment reads and replies | Post lookup, recent conversation search and reply | Organization posts and comment threads | Page posts and comments |
| Private conversations | Read existing DM events/threads and reply | **Unavailable for company pages in this integration** | Messenger conversation reads and eligible replies |
| Metrics | Current account/post metric snapshots | Organization share statistics | Current Page/post snapshots and explicitly requested Insights |

All features depend on the developer application, vendor access tier, granted
permissions and account role. Post search windows, message history and available
metric fields are platform limits, not promises of full historical coverage.
There is no fallback to scraping or browser-cookie automation.

LinkedIn operations require `account_id` naming a numeric organization ID or
`urn:li:organization:…`; account selection lists approved administrator ACLs.
The bridge checks organization access again on each invocation. LinkedIn
company-page private-message operations remain in the public contract with an
unavailable description, are omitted from verified usable operations, and are
rejected before dispatch. Enabling them requires an approved partner API
contract and a separate implementation.

Facebook requires a Page access token. Replying to a private conversation checks
Page participation, one-to-one participants and a recent incoming message within
the standard response window. When those facts cannot be established, it refuses
to send. It does not initiate unsolicited private conversations.

## Inputs and results

Read lists accept bounded `limit` and platform cursors. Publishing accepts `text`.
For X/LinkedIn, `image_data` is base64 (maximum 196,608 characters, approximately
144 KiB decoded) and requires matching `mime_type` (`image/png`, `image/jpeg`,
`image/webp`). Larger assets and video are outside this initial contract.
Facebook accepts `image_url`; the bridge does not fetch arbitrary image hosts
with its own credentials.

Results contain `provider`, `transport: "mcp"`, and bounded platform data. IDs,
metric names and cursor fields retain their platform meaning. Missing metrics
are unavailable, not zero. X metrics are current snapshots and reject date
intervals. LinkedIn supports reporting intervals for organization aggregates;
specific-post statistics are lifetime-only and reject intervals.
Facebook permits bounded explicit Insights metric names, subject to vendor
support; post lifetime metrics and Page daily metrics have different intervals.
These statistics do not measure ZGI registrations, trials or consultations.

Writes submit once. A timeout or malformed success response after submission is
an unknown outcome; check the original platform before initiating a new action.
The existing invocation receipt prevents replay of the same logical operation;
it is not cross-task content deduplication. Content planning, scheduling,
publication archives and conversion attribution belong to a later operations
module.

## Reuse and validation

Facebook uses `@thenavidm/facebook-mcp-cli` 0.3.0's tool registrations and Graph
client with a bounded, no-retry transport. LinkedIn reuses the pinned upstream
API client and adds organization, read, comment and analytics adapters. See
`api/social-mcp/README.md` for exact source revisions and license notices.

X's official MCP startup and authenticated catalog have not been validated for
this server credential lifecycle. This implementation uses fixed official REST
calls behind the built-in MCP bridge; it does not claim to reuse that server.

Automated tests use fixtures plus a real stdio protocol exchange. Production
acceptance additionally requires real developer apps and accounts: verify each
platform's identity, one approved test post, read-back, comment reply, permitted
private-message reply, and available metrics. A missing vendor grant is not
certified by successful fixture tests.
