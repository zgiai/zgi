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

## Deployment

Install the [social MCP bridge](social-mcp.md) first and run the API migrations before enabling OAuth. Configure the optional provider application in `api/.env.example`: `CONNECTOR_X_OAUTH_*`, `CONNECTOR_LINKEDIN_OAUTH_*`, or `CONNECTOR_FACEBOOK_OAUTH_*`. Each enabled application needs `CLIENT_ID`, `CLIENT_SECRET`, `REDIRECT_URL`, and permitted `SCOPES`. Set `CONNECTOR_SOCIAL_OAUTH_FRONTEND_URL` to the public console origin. Register the exact callback URLs at the platform:

| Provider | Callback path on the API origin |
| --- | --- |
| X | `/console/api/connectors/x-social/oauth/callback` |
| LinkedIn | `/console/api/connectors/linkedin-social/oauth/callback` |
| Facebook | `/console/api/connectors/facebook-social/oauth/callback` |

Use HTTPS in deployment; loopback HTTP is supported for development. The API and console must share a site so that the initiating browser can receive and return its HttpOnly, SameSite=Lax callback cookie. Keep local hostnames consistent. The configured console origin must also be allowed by the existing API CORS configuration, with credentials permitted for OAuth start. Credentials stay in encrypted database envelopes; reverse proxies must avoid logging callback query strings, authorization headers, and response bodies containing authorization URLs.

X requires user authorization, including `offline.access` for automatic renewal, `dm.read`/`dm.write` for private messages, and `media.write` for uploads. App-only bearer tokens cannot perform this flow. API entitlements still apply.

LinkedIn identity verification uses `/v2/userinfo` and requires `openid profile` plus the Sign In with LinkedIn using OpenID Connect product. LinkedIn company operations require Community Management / organization permissions approved for the application's access tier. Request only approved scopes through `CONNECTOR_LINKEDIN_OAUTH_SCOPES`. Refresh is available only if LinkedIn issues a refresh token to the approved app; otherwise the member must reauthorize when access expires. LinkedIn company messaging remains unavailable in the MCP integration.

Facebook requires Facebook Login and permissions for authorized Pages, publishing, interactions, messaging and Insights. Non-role users generally require the appropriate app review/access level. The adapter exchanges the user token for a long-lived token and obtains the selected Page token from `/me/accounts`. The current adapter conservatively uses the long-lived user authorization expiry as the selected Page credential expiry, even if the Page token could remain valid longer. Facebook does not provide the OAuth refresh-token grant used by X; expired/revoked authorization must be connected again. A long-lived Page credential is not a guarantee against revocation. Set `CONNECTOR_FACEBOOK_OAUTH_API_VERSION` consistently with the platform application (default `v23.0`).

Renewal occurs before an expiring OAuth connection is used, under a database lock. Idle connections are renewed when next used, rather than through a separate scheduler. A manual-token connection has no refresh token and retains its existing behavior. Reauthorization never silently changes the external identity of an existing bound connection.

## Official references

- [X authorization code flow, scopes, expiry and refresh](https://docs.x.com/fundamentals/authentication/oauth-2-0/authorization-code)
- [LinkedIn authorization code flow](https://learn.microsoft.com/en-us/linkedin/shared/authentication/authorization-code-flow)
- [LinkedIn partner refresh-token eligibility](https://learn.microsoft.com/en-us/linkedin/shared/authentication/programmatic-refresh-tokens)
- [Facebook long-lived access tokens](https://developers.facebook.com/docs/facebook-login/guides/access-tokens/get-long-lived/)
- [Meta Page access token example](https://www.postman.com/meta/facebook/request/bqfxwbp/get-access-tokens-of-pages-you-manage)

## Verification

Backend checks (from `api/`):

```sh
go test -p 1 ./internal/modules/connectors/... ./config
go test -race ./internal/modules/connectors/socialoauth
go test ./routes/v1 -run TestSocialOAuth
go test ./internal/migrations -run TestConnectorSocialOAuthMigrationSchemaAndRollback
go build ./cmd/server
```

Frontend checks (from `web/`, without starting a development server):

```sh
pnpm type-check
pnpm test:connector-social-oauth
pnpm test:connector-api-contract
pnpm test:connector-connection-management
pnpm test:connector-key-setup-lifecycle
pnpm test:connector-i18n
```

The tests use mocked provider responses, real native connection lifecycle persistence for atomic rollback, and SQL assertions for PostgreSQL locking. They cover denial, one-use callback state, initiating-browser cookies, actor boundaries, Page selection races, interrupted flow expiry, encrypted grants, rotated refresh tokens, manual replacement and unchanged binding authorization on renewal. They do not establish platform application approval or replace a real-account authorization test. Before enabling for users, verify connect, reconnect, selected Page identity, access expiry and revoked authorization against the configured platform applications.
