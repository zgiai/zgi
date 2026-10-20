# Social connector capability completion

## Scope

Extend the existing built-in social connectors for the social-operations SOP. The user authorized development on `feat/builtin-social-mcp`, with existing Feishu, DingTalk and email connectors used for feedback handoff. This change adds connector capabilities rather than content scheduling, lead management, autonomous authorization or a complete operations inbox. No development web server is started.

## Design

Keep the fixed native operation registry and isolated MCP bridge. Add only documented platform operations: X recent search, mentions, repost/quote; LinkedIn organization notifications and reshare; Facebook Page tagged posts and app event subscription. List operations gain supported date/ID watermarks with bounded cursor pagination. Unsupported combinations fail explicitly rather than silently ignoring filters. Output includes stable source IDs and pagination/checkpoint information; callers persist checkpoints in their task state. Existing write approval, no blind retries and unknown-outcome semantics remain.

Normalize larger submitted image bytes in the Go adapter before invoking MCP: validate real image content, bound encoded size and decoded dimensions, resize and compress to a bridge-compatible image. Never fetch arbitrary image URLs with server credentials. Existing public Facebook image URLs remain supported. Keep all confirmation and payload limits explicit; add a separate image preparation endpoint if larger sources cannot safely fit existing runtime continuation limits.

Add a Facebook Page webhook receiver with challenge verification and HMAC-SHA256 signature verification. Map signed Page IDs to existing verified Page connections, encrypt received payloads with the Connector keyring, and deduplicate per connection. Expose owner-scoped paginated event reads and acknowledgements; no webhook directly triggers posting or sends private data to another service. Subscription uses a fixed native write operation requiring its usual confirmation. X and LinkedIn retain documented pull paths; webhook enrollment requiring separate app registration/access is documented rather than guessed.

Feedback handoff reuses existing Feishu/DingTalk/email operations. Social events carry stable source IDs and source platform fields for correlation. Destinations and sensitive content remain subject to each target connector's authorization and operation confirmation.

## Implementation plan

1. Extend native operation contracts and bridge provider implementations together. Start with failing contract and provider fixture tests for new operations, filter rejection, account ownership and uncertain writes. Update official API versions/scopes where required.
2. Add image preparation and adapter normalization with tests for invalid bytes, excessive dimensions, compression bounds, MIME mismatch and unchanged caller-owned input. Preserve native confirmation body limits.
3. Implement encrypted Facebook event persistence, migration and signature-verified receiver; owner reads/acknowledgements recheck connection ownership and active state. Test replay, malformed bodies, wrong signatures, duplicate deliveries, fan-out only to authorized Page connections, and cross-owner denial.
4. Wire deployment configuration, service and authenticated/public HTTP boundaries; add fixed callback documentation and feedback-handoff examples using existing target connectors.
5. Run focused Go/Node tests, migration and safety checks, review changed contracts and security boundaries, then commit. Actual platform access and real-account acceptance still require registered apps and granted permissions.

## Sources

- [X mentions](https://docs.x.com/x-api/users/get-mentions)
- [X recent search](https://docs.x.com/x-api/posts/search-recent-posts)
- [LinkedIn organization notifications](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/organizations/organization-social-action-notifications?view=li-lms-2026-09)
- [LinkedIn posts and reshares](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/posts-api?view=li-lms-2026-09)
- [Meta Page webhooks](https://developers.facebook.com/docs/pages-api/webhooks-for-pages/)

## Built-in operations

New operations are platform-specific, not a promise of platform parity:

| Platform | Added operations |
| --- | --- |
| X | `post.search`, `mention.list`, `post.repost`, `post.quote`, `comment.create` |
| LinkedIn | `notification.list`, `post.repost`, `comment.create` |
| Facebook Page | `mention.list`, `webhook.subscribe`, `comment.create`, base64 image publishing |

X can create a comment on a visible post by another author while using the verified acting account. Facebook and LinkedIn comment creation is restricted to the managed Page/organization posts supported by this integration. Existing operations remain available. Reconnect existing accounts after deploying to verify the expanded operation set and consent to added permissions. LinkedIn requests `r_organization_social_feed`, `w_organization_social_feed`, and existing organization/OIDC scopes. Facebook event subscription requires `pages_manage_metadata` in addition to existing Page/Messenger grants. The bridge uses LinkedIn version 202609 and Graph v23.0. Check application access and account roles before production use.

List results retain platform pagination fields and include `sync` information with source platform, stable source IDs, next cursor and original filters. Preserve the same lower watermark across all pages; update a task's checkpoint only after processing succeeds. The connector does not run a background poller. Unsupported filter combinations fail rather than disappearing. Unsafe integer literals in platform JSON are preserved as strings to avoid loss of 64-bit identifiers. Unsafe numeric source IDs in exponential notation fail explicitly rather than yielding colliding correlation IDs.

LinkedIn notification pull history is 60 days; the platform recommends it for bootstrap/error recovery and recommends registered webhooks for ongoing notifications. This change adds its pull API, not automatic LinkedIn webhook registration. LinkedIn company messaging remains unavailable. X and LinkedIn notification/webhook entitlements still require their respective platform applications; this release implements the Facebook Page webhook receiver.

## Image preparation API

Authenticated `POST /console/api/connectors/social-media/prepare-image` accepts raw JPEG/PNG/WebP bytes with `Content-Type: application/octet-stream`. Maximum source is 8 MiB, edge 8192 and 24 million pixels. The response envelope's `data` contains `image_data`, `mime_type`, `width`, `height` and decoded `bytes`; pass the first two fields to `post.create` alongside text/account. Output is at most 1600 pixels on either edge and 144 KiB decoded, fitting the existing native confirmation/MCP limits. No remote URLs are fetched. Metadata is removed; large transparent images may be flattened onto white and compressed to JPEG. Review the prepared image before publication if visual fidelity is important.

Facebook accepts the prepared base64 data as a fixed-origin multipart photo upload as well as its existing public HTTPS `image_url`. These inputs are mutually exclusive. JPEG/PNG/WebP data submitted directly to native operations is validated and normalized before any bridge startup.

## Facebook Page events

Configure `CONNECTOR_FACEBOOK_WEBHOOK_APP_SECRET` and a random `CONNECTOR_FACEBOOK_WEBHOOK_VERIFY_TOKEN` of at least 16 characters. Enter the same verification token in the Meta app dashboard and register:

`https://API_HOST/console/api/connectors/social-webhooks/facebook`

The GET challenge and POST receiver are public provider callbacks; signatures are required on every delivery. Use the same Meta application that subscribes to the Page. Invoke the native `facebook-social.webhook.subscribe` operation to subscribe the verified Page to fixed `feed,messages,messaging_postbacks` fields. Normal operation authorization/confirmation applies. Meta app review, a reachable HTTPS callback and actual Page permissions are deployment prerequisites.

Deploy the new database migration before enabling events. Valid unknown-Page deliveries receive 200 without exposing connection membership. Persisted event payloads are encrypted separately for each authorized connection. The receiver never dispatches an agent or posts a reply.

Authenticated queue endpoints:

- `GET /console/api/connector-connections/{connectionId}/social-events?after_id=0&limit=20` returns unread `items` and `next_after_id`. Each item includes the database event `id`, `provider`, `source_id`, `external_account_id`, `received_at` and external `payload`.
- `POST /console/api/connector-connections/{connectionId}/social-events/ack` with `{"through_id":42}` marks entries through that position within the selected connection as acknowledged. Acknowledge only after downstream processing or handoff succeeds.

`source_id` deduplicates a canonical Page entry batch, not each message within a differently grouped/redelivered batch. Consumers can additionally deduplicate contained message IDs or post/change IDs. List responses are bounded to 512 KiB, allowing a single event up to 1 MiB; pagination does not advance past an omitted event. Signed deliveries accept at most 100 entries, 100 matched connections and 200 persisted records / 8 MiB fan-out. Exceeding fan-out bounds rolls back the entire receipt and returns a retryable failure rather than silently dropping recipients.

## Feedback handoff

Reuse separately authorized target connections: `feishu.message.send`, `dingtalk.messages.send_direct` / `dingtalk.messages.send_group`, or mail `message.send`. A workflow or digital employee can read an event, extract the problem/need, include platform+Page+source_id+contained message/post ID, and submit the structured summary through the selected target connector. Keep a stable handoff correlation key such as `social:{connectionId}:{source_id}`. Resolve a post URL through the platform's post read result where available; do not invent public URLs for private conversations.

Social account authorization does not grant access to a team chat or mailbox. Bind the desired Feishu/DingTalk/mail connection and its allowed destination independently. Normal target operation confirmation applies. Do not acknowledge the source until confirmed downstream delivery; an unknown send result needs verification before retry. This release provides the source and destination connector interfaces, not an automatic forwarding rule or lead-management application.

## Validation

Use `npm test` in `api/social-mcp`; Go tests for `connectors/catalog`, `connectors/providers/socialmcp`, `connectors/socialevents`, `config`, the new routes and migration; and `go test -race ./internal/modules/connectors/socialevents`. In `web/`, run `pnpm test:connector-social-capabilities`, `pnpm type-check` and targeted lint. No real platform messages or posts are sent by fixture tests. Real-account acceptance remains necessary; Meta documentation fetches were rate limited during development, so the live app entitlement and tagged/subscription behavior are not certified by these fixtures.


### Native event operations

`facebook-social.event.list` and `facebook-social.event.ack` expose the same owner-scoped queue through native Connector execution. Connection identity comes from the authorized runtime scope, never tool arguments. They retain normal binding/operation authorization. The local event operations do not start an MCP subprocess or transmit the social access token to the event store.

Native `event.list` accepts `after_id` and `limit` (up to 20). `event.ack` accepts `through_id`; use it after confirmed downstream handling. Native responses must fit the existing 60 KiB output budget. Request a smaller page when needed; a single oversized event requires the authenticated HTTP API/manual review rather than silent truncation or acknowledgement. No forwarding rule or automatic agent trigger is installed.
