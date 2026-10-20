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
