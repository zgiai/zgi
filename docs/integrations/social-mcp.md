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
| Text/image posts | Official API; bounded base64 image input | Company organization author; bounded base64 image input | Reused MCP text/photo handlers; inline base64 or public HTTPS image URL |
| Post/comment reads and replies | Post lookup, recent conversation search and reply | Organization posts and comment threads | Page posts and comments |
| Private conversations | Read existing DM events/threads and reply | **Unavailable for company pages in this integration** | Messenger conversation reads and eligible replies |
| Search, mentions, notifications | Recent post search and connected-user mentions | Managed organization activity notifications | Tagged posts on connected Page |
| Repost/quote | Official repost and quote publishing | Official organization reshare, optional commentary | Unavailable |
| Event subscription | Pull reads | Pull notifications; partner webhook enrollment separate | Fixed Page feed and Messenger subscription |
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
For all three platforms, `image_data` is base64 (maximum 196,608 characters, approximately
144 KiB decoded) and requires matching `mime_type` (`image/png`, `image/jpeg`,
`image/webp`). The separate authenticated image preparation endpoint can normalize larger uploaded image sources to this runtime limit; video is unavailable.
Facebook also accepts `image_url` (mutually exclusive with `image_data`); the bridge does not fetch arbitrary image hosts
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

## Incremental reads, reshares and Page subscriptions

Plugin 1.1.0 adds `x-social.post.search`, `x-social.mention.list`, `x-social.post.repost`, `x-social.post.quote`, `linkedin-social.notification.list`, `linkedin-social.post.repost`, `facebook-social.mention.list`, and `facebook-social.webhook.subscribe`. Search requires `query`; reposts require `post_id`; X quote requires `text`, while LinkedIn reshare commentary is optional. There is no LinkedIn global search or company messaging endpoint and no generic Facebook search.

X post lists, recent search, mentions and comments accept `since`/`until` (RFC3339) and `since_id`/`until_id` (decimal IDs). LinkedIn organization notifications accept native date intervals. Other lists reject these filters explicitly. List/search outputs preserve raw vendor fields and add a `sync` object with source platform/IDs, continuation cursor and supplied filters. Pass the original query/account/filters on every page; checkpoint only after all pages are stored and processed. No checkpoint persistence, scheduling or full historical sync is implied. Offset pagination can shift as data changes.

Reposts and quotes require normal publishing confirmation. X repost and Facebook subscription use explicit native boolean acknowledgments; the bridge never invents a post ID for them. Unknown submissions are not retried. The Page subscription has fixed `feed,messages,messaging_postbacks` fields, requires a configured application webhook and `pages_manage_metadata`; it cannot target another Page or an arbitrary callback URL. Signed webhook storage and owner-scoped event reads/acknowledgements are API features; this does not create a full operations inbox. Feedback handoff can use the existing Feishu, DingTalk and email connectors, with their usual authorization and confirmation.

LinkedIn is pinned to supported `202609`. Its [scope migration](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/community-management-api-migration-guide?view=li-lms-2026-09) requires `r_organization_social_feed`/`w_organization_social_feed` for social actions, alongside post grants; [notifications](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/organizations/organization-social-action-notifications?view=li-lms-2026-09) require `rw_organization_admin`. Existing connections may need reauthorization. Facebook tagged reads require the corresponding Page content grant; API/app review restrictions still apply. Meta documentation returned rate-limit errors during implementation, so fixture success does not certify current real-account availability.

X date and ID bounds are mutually exclusive per side: use either `since` or `since_id`, and either `until` or `until_id`. IDs are bounded to 19 decimal digits. Invalid combinations fail before network access.

Top-level comments are available as `comment.create` (`comment_create` inside MCP) on all three platforms. Inputs require `post_id` and `text`, plus the usual LinkedIn organization `account_id`; no existing `comment_id` is needed. The operation is a non-idempotent external send with normal confirmation and native receipt checks. X verifies the requested post exists and replies as the authenticated user, including to visible posts by other authors; any explicit acting `account_id` must match the token. Facebook restricts the post ID to the verified Page, and LinkedIn verifies the post author against the selected managed organization. Commenting on external discussions through Facebook/LinkedIn is outside this contract. LinkedIn sends the managed organization actor and source object without a `parentComment`; nested replies continue using `comment.reply`.
