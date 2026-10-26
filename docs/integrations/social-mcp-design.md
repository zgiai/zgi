# Built-in social MCP integration

Date: 2026-10-19. Base: `feat/zgi-market` at `e831acaf5`.

## Goal and scope

Add X, LinkedIn and Facebook to the existing native Connector Center. Reuse open-source MCP implementations where their identity and credential model fits. Include publishing, comments, private conversations and platform metrics. This is a connector integration, not the complete autonomous social-operations SOP.

The first release uses the existing encrypted personal access-token setup. Operators obtain vendor tokens through their developer applications; no vendor password or browser cookie is accepted. Native browser OAuth onboarding is a subsequent, separate change. A connection remains personally owned inside its organization; no new shared-credential ownership model is introduced.

## Architecture

Existing Connector setup → encrypted connection → explicit resource binding → native invocation/confirmation → built-in Go social MCP driver → isolated Node MCP process → approved platform APIs.

Three stable IDs: `x-social`, `linkedin-social`, `facebook-social`. Each operation has a fixed schema and native operation ID. There is no member-controlled command, URL, tool name or shell execution. The trusted deployment config selects Node and the bridge script. The bridge runs once per invocation with only this connection's token and the platform contract; it inherits no home-directory credential cache. Processes and temporary resources are closed after every call. Writes are submitted once; ambiguous results are recorded as unknown, never blindly repeated.

Reuse evaluation: Facebook uses `@thenavidm/facebook-mcp-cli` 0.3.0; LinkedIn reuses the API client from `2060-io/linkedin-mcp`, pinned revision with its Apache-2.0 LICENSE. Preserve upstream notices. Supplement missing operations with fixed official API calls. X's official `xmcp` requires a different startup OAuth/credential lifecycle; its authenticated hosted tool schemas have not been verified. This release therefore wraps fixed official X REST calls in the same isolated MCP bridge, without claiming to reuse the official X MCP or silently switching transports after submission. Facebook's reused tools are bound to one verified Page per process. LinkedIn organization authors must be among the token's approved managed organizations.

## Public behavior

Catalog entries expose account identity/selection, list/read/create posts, list/reply comments, list/read/reply private conversations, and read platform metrics. LinkedIn company-page private messaging is not asserted available: ordinary Community Management access does not provide it. Its message operations stay unavailable and fail before network dispatch; a partner implementation requires a documented, approved API contract. API scopes/tiers still govern every operation. Available operation discovery describes verified bridge support, not proof that every upstream write permission has been granted.

Output retains platform-native IDs, pagination cursors and metric names in a bounded `data` object, plus the connector ID and transport. Unavailable metrics are missing rather than fabricated as zero. Metrics are platform statistics, not ZGI trial/conversion attribution. Public content and messages are untrusted data.

Use existing per-invocation confirmation, tenant/owner checks, connection revisions, receipts, unknown-outcome handling and audit records. Include DigitalHuman bindings through the existing V4 surface contract. This release does not weaken its confirmation requirement or claim unattended publishing is enabled.

## Delivery and verification

1. Register localized schemas and add provider bootstrap/configuration plus visible brand/category metadata.
2. Add the isolated MCP driver and Node bridge under `api/social-mcp` with reuse/license provenance and bounded official API supplements. Include Node and pinned production dependencies in the API Docker image.
3. Verify catalog contracts, provider dispatch, schema mismatch rejection, token redaction, account separation, absent deployment configuration, denied permissions and ambiguous writes.
4. Test Node handlers against injected HTTP fixtures and one real stdio MCP subprocess. Run relevant Go connector tests, frontend checks and server compilation.
5. Real-account acceptance is a separate test requiring operator-supplied developer apps/tokens. Never imply that mock tests certify vendor approval, scopes or production delivery.

## Acceptance

All three connectors appear in Connector Center and can use the existing token setup, bindings and confirmed invocation flow. Missing bridge configuration or platform access produces a safe failure. The bridge supports X and Facebook private-message operations and platform statistics; LinkedIn statistics use official organization endpoints while unsupported private messages are explicitly unavailable. Existing connector contracts and unrelated digital-human/Market governance remain compatible.
