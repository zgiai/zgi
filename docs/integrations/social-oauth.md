# Built-in social OAuth

This extension implements the approved connector flow: connect, authorize at the platform, return to ZGI, select a Facebook Page if necessary, and save the encrypted credential. It preserves the existing manual-token flow and runtime confirmation policy.

## Design

The API owns OAuth client secrets, exact registered callback URLs, fixed provider endpoints and scopes. X uses Authorization Code with S256 PKCE and offline access. LinkedIn uses its confidential authorization-code flow and refreshes only when a refresh token was issued. Facebook exchanges a short-lived user token for a long-lived token, lists authorized Pages, and stores the selected Page token. Facebook and LinkedIn never simulate unsupported refresh grants.

A persisted, encrypted ten-minute authorization flow binds random one-time state to organization, account, connector, optional reconnect connection, and PKCE verifier. A public API callback consumes state and redirects to a fixed frontend completion route with a flow UUID only. The authenticated completion screen reads safe status and Page choices. No code, token or upstream error is put into the frontend URL. Denial, replay, expiry and cross-owner access fail closed.

Credentials reuse the Connector keyring. OAuth grant metadata and refresh tokens are encrypted separately from the existing connection token. Resolution renews expiring credentials under a database lock before provider dispatch. This also renews idle X connections on their next use. Non-refreshable expired or revoked grants require reauthorization. Transient failures preserve credentials and surface a safe retryable error. A reconnect replaces a connection only after verifying the same external identity; changed identity creates a separate unbound connection.

The connector UI uses the same OAuth panel for creation in Connector Center and embedded pickers. It supports reconnect, Facebook Page selection, success/error status, and manual-token fallback. Platform applications and permission approval remain deployment prerequisites. Next.js is not started as part of implementation validation.

## Implementation and validation plan

1. Implement the isolated `socialoauth` provider/session/grant module and database migration. Start with failing tests for authorization state, exchange, encryption, scope, Page selection, expiry, owner isolation, replay and refresh concurrency.
2. Wire deployment configuration and a singleton service into the composition root. Add authenticated start/status/select routes plus an unauthenticated state-bound callback. Cover safe redirect and identity boundaries with handler tests.
3. Connect the OAuth resolver to native execution and maintenance without changing other connector credentials or binding authorization.
4. Add the shared frontend OAuth panel, completion route and localized messages. Reuse existing API client, dialogs and cache invalidation. Validate TypeScript, lint and focused behavior tests without launching Next.js.
5. Review implementation against this design and run focused Go, Node and frontend checks. Document exact callback URLs, permissions, expiry behavior, and real-account testing prerequisites. Actual production OAuth verification requires platform client credentials and account authorization.
