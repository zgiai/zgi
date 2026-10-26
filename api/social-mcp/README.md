# ZGI built-in social MCP bridge

A local stdio MCP adapter for the native `x-social`, `linkedin-social`, and
`facebook-social` connectors. Node.js 22 or later is required. Install the locked
runtime dependencies with `npm ci --ignore-scripts`; run fixtures and the real
stdio subprocess handshake with `npm test`. No build is required.

The backend launches `node /absolute/path/server.mjs` with these variables only:

- `ZGI_SOCIAL_PROVIDER`: one of the three connector IDs.
- `ZGI_SOCIAL_TOKEN`: a single vendor user-context token (X/LinkedIn) or Facebook
  **Page** token. Do not provide passwords, browser cookies or application-only X
  bearer tokens.
- `ZGI_SOCIAL_CONTRACT`: JSON array of native `{name, description, inputSchema}`
  definitions. The bridge advertises their exact schemas for implemented tools.

No environment file or user credential cache is loaded. Native connection binding,
confirmation, receipt tracking and unknown-outcome handling remain the caller's
responsibility. This server does not grant autonomous publishing permission.

## Operations and limitations

`account_get`, `account_list`, `post_list`, `post_get`, `post_create`,
`comment_list`, `comment_reply`, and `metrics_get` are implemented for all three
providers. X and Facebook additionally expose `conversation_list`, `message_list`,
and `message_send`. LinkedIn company Page private messaging is **unavailable**:
its message tools are omitted, and direct calls fail before network access.
There is no guessed partner endpoint or browser automation workaround.

List operations request one bounded page (default 20, maximum 50), with platform
pagination cursors retained. X timeline/search API minimum page sizes may be 5/10.
X `conversation_list` returns native DM events containing `dm_conversation_id`,
not an invented conversation summary. LinkedIn cursors are numeric offsets. Its
account list paginates only the first 50 verified, approved ADMINISTRATOR ACL
entries; organizations outside that verified set are denied. Accounts require
`account_id` for LinkedIn content/comment/statistics operations. X/Facebook may
omit it, but an explicit value must match the token's verified identity. LinkedIn
identity validation uses OpenID userinfo; obtain `openid profile` along with the
necessary Community Management access.

Posting accepts `text`, with optional `image_data` (base64, at most 196608
characters) and `mime_type` (`image/png`, `image/jpeg`, `image/webp`) on all providers.
The signature is checked; no arbitrary image URL is fetched. LinkedIn initializes
an official upload, only sends credentials to LinkedIn/LinkedIn media domains,
and waits for AVAILABLE before posting. Facebook accepts either inline bytes sent as multipart `source` to its photos endpoint, or `image_url`: HTTPS,
without credentials, IP literals or local hostnames; the URL is passed to Meta
for remote ingestion, never fetched by the bridge. Platform image restrictions
and processing errors can still reject the image.

Replies require `post_id`, `comment_id`, `text`. The bridge verifies the X thread,
Facebook comment root object, or LinkedIn organization author and scoped comment
before writing. LinkedIn comment IDs may be numeric or a full native comment URN.
Private-message sending requires an existing `conversation_id`; there is no cold
recipient/bulk-message input. Messenger verifies Page participation and a received
message in the standard 24-hour response window from the most recent 50 messages;
if the window cannot be verified, it refuses. X resolves an existing conversation
before sending.

Metrics retain native field names and missing values, never fabricated zeros:

- X: current account `public_metrics`, or owned post public/non-public/organic/
  promoted metrics. Non-public fields depend on scopes, tier and post age. Since/
  until filters are rejected because these endpoints return current snapshots.
- LinkedIn: organization share statistics, optionally restricted to an owned post. A complete `since`/`until` RFC3339
  interval is supported only for organization aggregates; per-post intervals
  are rejected. This is not follower-demographic
  reporting or trial/conversion attribution.
- Facebook: Current Page/post snapshots by default; explicit `metrics` (up to ten native
  metric names) opt into Insights. Date filters require explicit metrics; Page
  Insights use day intervals, while post Insights omit period and reject date
  filters because they return lifetime values. Availability
  and metric deprecations depend on Graph version and granted Page access; API
  errors are reported rather than replaced with zeros. No deprecated metric names
  are guessed by default. Meta insights
  and real-account grants require acceptance testing.

There are 15-second request deadlines, a 1 MiB response read cap, a 60 KiB tool
result cap, HTTPS host allowlists, redirect refusal and no write retries.
Credential-bearing fields/URLs are stripped. Error responses carry only stable
safe codes and static text; uncertain writes return `outcome_unknown` with
`unknown: true`. Oversized reads fail and request a smaller page. Native-process
lifetime bounds the overall call, including LinkedIn image processing.

## Actual reuse and provenance

- **Facebook**: `@thenavidm/facebook-mcp-cli` **0.3.0**, MIT, locked by npm integrity.
  [Source](https://github.com/thenavidm/facebook-mcp-cli). The bridge imports its
  `registerAll` and `Graph`, and invokes registered `create_post`,
  `create_photo_post`, `reply_to_comment` handlers with a single verified Page.
  Its Graph request transport is replaced to use header authentication, bounded
  reads, sanitized errors, and the same pinned Graph v23.0 as bounded reads,
  Messenger and Insights supplements. Review this version before platform sunset. No upstream CLI
  login/config-cache code is executed.
- **LinkedIn**: [2060-io/linkedin-mcp](https://github.com/2060-io/linkedin-mcp), pinned
  commit **c1cddcfaf98bf9129fc5f87d21d181f68c00a9b7**. `upstream/linkedin/` preserves
  its repository Apache-2.0 `LICENSE` and original `linkedin-api.ts`; package
  metadata inconsistently states MIT, so repository LICENSE is retained. The
  checked-in JS is a TypeScript-transformed copy, with injected bounded transport
  optional `object`/`parentComment` fields and native comment receipt fields added
  for official nested replies.
  `helpers.js` extracts upstream `encodeUrn`. Its API client supplies publishing,
  Little Text escaping, comments and the image-upload/processing flow. Managed
  organization ACLs, bounded reads and statistics supplement that client. API
  version is pinned to `202609`; review before LinkedIn version expiration.
- **X**: fixed [official X API](https://docs.x.com/x-api) REST operations, including
  [media upload](https://docs.x.com/x-api/media/upload-media). **No X MCP reuse is
  claimed.** Hosted/open-source xMCP integration is deferred until its authenticated
  tool catalog, schema and token compatibility can be verified. There is no
  speculative MCP-to-REST fallback after dispatch.
- MCP transport: `@modelcontextprotocol/sdk` **1.32.1**, MIT, locked by npm.

Fixture tests and a stdio handshake do not certify actual vendor approvals,
scopes, paid access tiers or production delivery. No live writes or credentials
are used by the test suite. Real-account acceptance must verify each advertised
operation with operator-provided developer applications and tokens.

Official supplement contracts:
[LinkedIn comments](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/comments-api),
[LinkedIn organization statistics](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/organizations/share-statistics),
[Meta Messenger Send API](https://www.postman.com/meta/messenger-platform-api/folder/vilwbh4/send-api).

## Completion operations (plugin 1.1.0)

- X: `post_search` requires `query` (512 characters); `mention_list` reads the connected user's mentions. `post_repost` requires `post_id`; `post_quote` also requires `text` and creates an official quote post.
- LinkedIn: `notification_list` uses organization-scoped `organizationalEntityNotifications` with a fixed action set. `post_repost` uses the reused upstream Posts API reshare implementation; `text` is optional commentary. The target organization must appear in verified administrator ACLs; the shared source post can belong to another author.
- Facebook: `mention_list` reads the verified Page's tagged posts, not a global search. `webhook_subscribe` posts the fixed `feed,messages,messaging_postbacks` subscription to that Page's `subscribed_apps`. The application callback must already be configured. Its fields, callback and URL cannot be overridden by tool arguments.

X timeline, recent search, mention and comment reads accept RFC3339 `since`/`until` mapped to `start_time`/`end_time`, plus decimal `since_id`/`until_id`. X recent search remains limited to the vendor's recent window and access tier. LinkedIn notifications support `since`/`until` through native millisecond `timeRange` (inclusive start, exclusive end). Other list endpoints reject date/ID filters rather than silently dropping them; this includes LinkedIn posts/comments, Facebook lists and private messages. No client-side filtering pretends to provide durable incremental history.

Native payloads and pagination fields remain intact. Non-account list/search results additionally contain `sync.source_platform`, stable native `source_ids`, `next_cursor` (or null), the original supplied `filters`, and checkpoint guidance. Keep the same query, account and filters across pages; persist each page before advancing. Advance a task-owned watermark only after every page completes; a page's newest item is not a completed sync watermark. Offsets/cursors can expire or shift as vendor data changes; this bridge neither stores checkpoints nor runs a background sync. Vendor pagination URLs are never followed.

Reposts/quotes are non-idempotent publishing actions. X repost success requires explicit `data.retweeted === true`; Meta subscription requires `success === true`. These acknowledge the operation without inventing a post receipt ID. Quote and LinkedIn reshare success require the native post ID. Missing acknowledgments and uncertain writes remain `outcome_unknown`; no blind retry occurs.

Permissions supplement existing grants: X `tweet.read users.read tweet.write` (user context); LinkedIn notifications `rw_organization_admin`, comments `r_organization_social_feed w_organization_social_feed` under the official [Community Management scope migration](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/community-management-api-migration-guide?view=li-lms-2026-09), posts/reshare `r_organization_social w_organization_social`; Facebook tagged reads require Page read-user-content access and Page selection permissions, subscription `pages_manage_metadata`, Messenger fields `pages_messaging`. Existing grants must be reauthorized for newly requested scopes. Native permission errors remain failures.

References: [X mentions](https://docs.x.com/x-api/users/get-mentions), [X recent search](https://docs.x.com/x-api/posts/search-recent-posts), [LinkedIn notifications](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/organizations/organization-social-action-notifications?view=li-lms-2026-09), [LinkedIn reshares](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/posts-api?view=li-lms-2026-09), [Meta tagged Page posts](https://developers.facebook.com/docs/graph-api/reference/page/tagged/), [Meta subscriptions](https://developers.facebook.com/docs/graph-api/reference/page/subscribed_apps/), [Meta Page webhooks](https://developers.facebook.com/docs/pages-api/webhooks-for-pages/). Meta reference pages were rate-limited during implementation; routing, payloads and safety behavior are verified with fixtures, with live app/access acceptance still required.

X date and ID bounds are mutually exclusive per side: use either `since` or `since_id`, and either `until` or `until_id`. IDs are bounded to 19 decimal digits. Invalid combinations fail before network access.

Top-level comments are available as `comment.create` (`comment_create` inside MCP) on all three platforms. Inputs require `post_id` and `text`, plus the usual LinkedIn organization `account_id`; no existing `comment_id` is needed. The operation is a non-idempotent external send with normal confirmation and native receipt checks. X verifies the requested post exists and replies as the authenticated user, including to visible posts by other authors; any explicit acting `account_id` must match the token. Facebook restricts the post ID to the verified Page, and LinkedIn verifies the post author against the selected managed organization. Commenting on external discussions through Facebook/LinkedIn is outside this contract. LinkedIn sends the managed organization actor and source object without a `parentComment`; nested replies continue using `comment.reply`.
