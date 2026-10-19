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
characters) and `mime_type` (`image/png`, `image/jpeg`, `image/webp`) on X/LinkedIn.
The signature is checked; no arbitrary image URL is fetched. LinkedIn initializes
an official upload, only sends credentials to LinkedIn/LinkedIn media domains,
and waits for AVAILABLE before posting. Facebook accepts `image_url`: HTTPS,
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
  version is pinned to `202610`; review before LinkedIn version expiration.
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
