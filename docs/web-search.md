# Web Search with Exa

ZGI's Web Search integration lets AIChat and Agent runtimes search the public
web and read selected public webpages through Exa. It uses the shared external
integration runtime for Connections, encrypted BYOK credentials, grants,
health, per-Action policies, AIChat preferences, Agent connection bindings, and
execution logs. Exa credentials must come from an explicit organization or
personal Connection; the API process does not provide a provider-credential
fallback. The integration is disabled by default. See
[External integrations](external-integrations.md) for the common connection
model and credential-key rotation procedure.

## Requirements

Before enabling Web Search, make sure the deployment has:

- PostgreSQL for integration execution audit records.
- Redis for per-organization daily limits and durable audit-completion recovery.
- An Exa API key stored in an organization or personal Integration Connection.
- A unique, exactly 32-byte `API_KEY_ENCRYPTION_KEY` for audit fingerprints.
- A named Integration credential keyring, or the legacy encryption key as a
  compatibility fallback.
- The database migrations that create `integration_executions`,
  `integration_connections`, `integration_action_policies`, and Agent
  Integration Connection binding metadata.

`API_KEY_ENCRYPTION_KEY` derives a purpose-separated HMAC key for Web Search
audit fingerprints. Set it even in a local environment when Web Search is
enabled; otherwise external calls fail closed because they cannot be audited.
Generate a 32-character value with:

```bash
openssl rand -hex 16
```

An Exa credential is submitted once over the authenticated Connection API,
encrypted before storage, and never returned to the browser. Do not put
provider credentials in the API process environment, commit them, or expose
them through browser environment variables.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `WEB_SEARCH_PROVIDER` | `exa` | Provider selection. The current native adapter supports only `exa`. |
| `EXA_TIMEOUT_SECONDS` | `20` | Total timeout for an Exa operation, including retries. Must be positive. |
| `EXA_MAX_RESULTS` | `10` | Search-result limit. The phase-one hard maximum is 10. |
| `EXA_DEFAULT_SEARCH_TYPE` | `auto` | Search mode used when the tool call omits one: `auto`, `fast`, or `instant`. |
| `EXA_MAX_FETCH_URLS` | `5` | URL limit for one webpage-fetch call. The phase-one hard maximum is 5. |
| `EXA_MAX_CONTENT_CHARACTERS` | `20000` | Maximum retained content per fetched page. The phase-one hard maximum is 20,000. |
| `API_KEY_ENCRYPTION_KEY` | empty | Legacy 32-byte Connection-key fallback. New deployments should use the named Integration keyring. |
| `EXTERNAL_INTEGRATIONS_ENABLED` | `false` | Enables the shared Connection catalog and all built-in providers, including Web Search. |
| `INTEGRATION_ORG_DAILY_LIMIT` | `1000` | Maximum external-application calls per organization per UTC day, shared by Web Search and other providers. |
| `INTEGRATION_TIMEOUT_SECONDS` | `20` | Shared execution timeout for external actions. |
| `INTEGRATION_CREDENTIAL_ACTIVE_KEY_ID` | empty | Key ID used for newly encrypted Connection credentials. |
| `INTEGRATION_CREDENTIAL_KEYS_JSON` | empty | JSON object from key ID to exactly 32-byte key. Retain old entries while stored envelopes use them. |

All numeric limits must be greater than zero and must not exceed their stated
hard maxima. An enabled deployment refuses to start when the provider,
encryption key, or limits are invalid. Provider registration is independent of
provider credentials; calls remain unavailable until a tested Connection is
selected or bound.

`WEB_SEARCH_ENABLED` is a retired compatibility variable and does not control
provider registration. Use `EXTERNAL_INTEGRATIONS_ENABLED` as the single
runtime switch.

Example API environment:

```env
EXTERNAL_INTEGRATIONS_ENABLED=true
WEB_SEARCH_PROVIDER=exa
INTEGRATION_ORG_DAILY_LIMIT=1000
INTEGRATION_TIMEOUT_SECONDS=20
EXA_TIMEOUT_SECONDS=20
EXA_MAX_RESULTS=10
EXA_DEFAULT_SEARCH_TYPE=auto
EXA_MAX_FETCH_URLS=5
EXA_MAX_CONTENT_CHARACTERS=20000
API_KEY_ENCRYPTION_KEY=<exactly-32-bytes>
INTEGRATION_CREDENTIAL_ACTIVE_KEY_ID=2026-07
INTEGRATION_CREDENTIAL_KEYS_JSON={"2026-07":"<another-exactly-32-byte-secret>"}
```

### Docker deployment

For the repository Docker stack, put the `EXTERNAL_INTEGRATIONS_*`,
`INTEGRATION_*`, `WEB_SEARCH_PROVIDER`, and `EXA_*` values in `docker/.env`.
The API reads `API_KEY_ENCRYPTION_KEY` from
`api/.env.docker`. A fresh `make bootstrap` or the platform-specific bootstrap
script generates that 32-byte key automatically; replace it with a
deployment-specific secret for production.

The `EXA_*` environment values configure request limits and behavior only.
Create Exa credentials through the authenticated Connection UI or API.

Existing checkouts can append newly introduced environment keys from their
templates with `make env-sync`, then edit the resulting local environment
files. Review the backup produced by that command before deleting it.

### Source deployment

When running the API directly, put all values in `api/.env` or inject them into
the API process environment.

## Apply the Migration and Restart

Web Search writes execution metadata and Connection policy state to PostgreSQL.
Apply the public migration chain before serving traffic:

```bash
cd api
go run ./cmd/migrate up
go run ./cmd/migrate status
```

The Docker image applies migrations during API startup when
`MIGRATION_ENABLED=true`, which is the default in `api/.env.docker`. After
updating the environment or application image, recreate or restart the API
service so the Connector is registered:

```bash
cd docker
docker compose --env-file .env up -d api
docker compose --env-file .env logs -f api
```

For a source deployment, restart the API process after the migration and
configuration changes. Changing `EXTERNAL_INTEGRATIONS_ENABLED` without
restarting does not add or remove providers from a running process.

## Configure and Select a Connection

Deployment configuration makes the Web Search provider and its Actions
available in Connection Center. Web Search is not a Skill and has no separate
organization or user Skill switch.

1. Sign in as an organization administrator.
2. Open `/console/integrations` from **Authoring tools → Connection Center**.
3. Create an organization API-key Connection. A user who should not share a
   credential can instead create a personal Exa Connection from the same
   Connection Center. Testing a Connection performs one minimal Exa search, consumes one
   organization daily-quota unit, and can incur the provider's normal cost.
4. Grant a shared Connection to the required organization, workspace, or
   member scope.
5. AIChat users explicitly select the Exa Connection under **Connected Apps**.
   The hidden external-apps runtime discovers `web.search` and `web.fetch` and
   obtains their current Action guides before execution.
6. To use it in an Agent, select an Integration Connection plus the permitted
   `web.search` and/or `web.fetch` Action IDs in the Agent editor, then save and
   publish. No Web Search Skill needs to be added.

AIChat resolves only a Connection explicitly selected in Connected Apps. Agent
execution requires an explicit organization Connection binding and at least
one permitted Action ID. Invocation is limited to that exact Connection and
its Action allowlist; a missing, invalid, expired, or unauthorized Connection
never falls back to another credential.

The current Actions support only `aichat` and `agent` callers. They are not
Workflow tools.

## Capabilities and Current Boundaries

Web Search exposes two governed external Actions:

- `search_web` searches public sources with `auto`, `fast`, or `instant`
  search and returns bounded titles, URLs, publication dates, authors, and
  relevant highlights.
- `fetch_webpage` reads bounded text or highlights from up to five selected
  public HTTP or HTTPS URLs.

The Action guides require source links, distinguish publication dates from
retrieval time, report conflicting sources, keep queries public and minimal,
and treat all webpage content as untrusted data. Results are normalized and
truncated before they enter model context; the raw Exa response is not
forwarded as a tool result. Failed
requests retain only Exa's bounded safe error tag, request ID, HTTP status, and
retry time. Batch webpage reads attach a safe error code to each failed URL
without exposing the provider response body.

This release intentionally does not include:

- OAuth.
- Workflow Connector nodes.
- MCP transport or dynamic MCP Actions.
- OpenAPI dynamic import.

Calls accrue to the Exa account represented by the selected organization or
personal Connection.

## Security, Quotas, and Audit

Search queries, URL lists, and related fetch parameters are data egress to
Exa. ZGI applies the following controls before making an external request:

- JSON Schema validation for every action.
- Detection and blocking of API keys, bearer tokens, private keys,
  credential-bearing URLs, connection strings, and similar secrets.
- Rejection of localhost, private-address, local-domain, non-HTTP, and
  credential-bearing fetch URLs.
- A Redis-backed daily request limit isolated by organization and UTC day.
- A required audit-record creation step. If quota or audit infrastructure is
  unavailable, the external request fails closed.
- Request-scoped credential resolution. Organization and personal credentials
  are encrypted with AES-256-GCM under a purpose-separated key; authenticated
  additional data binds the ciphertext to organization, Connection,
  Integration, and credential version. Decrypted values are cleared after the
  single provider call.
- Organization Action policies can disable an Action, block data egress, or
  require interactive approval. They cannot lower provider-defined effect,
  risk, or external destination. An Action configured as `always_ask` requires
  approval for every invocation: the approval resumes only that frozen call
  and is never retained as a conversation-level grant.
- Agent executions recheck the current draft or published binding and Action
  allowlist immediately before using an explicitly selected Connection.
- A Redis-backed completion outbox. If the database update after an Exa call
  fails, bounded request ID, cost, duration, result count, and status metadata
  are persisted without an application TTL and replayed without repeating the
  external request. Every enabled API instance runs a startup recovery worker;
  atomic Redis processing leases prevent instances from replaying the same
  completion concurrently, and expired leases are retried. Malformed recovery
  records are moved to a Redis dead-letter hash and reported in server logs.

Each tool invocation records bounded operational metadata such as organization,
caller, action, status, provider request ID, duration, result count, retry
count, returned cost, and an HMAC fingerprint of the input. The audit record
does not store the Exa API key, raw search query, webpage body, or raw upstream
response. The encrypted Connection secret remains server-side and is not
included in normalized tool output. The Redis recovery record contains only the same bounded
operational metadata, never the query or webpage body.

Webpage content remains untrusted after retrieval. The `web.fetch` Action guide
instructs the model to ignore embedded commands, credential requests,
policy-bypass text, and other prompt-injection attempts.

## Troubleshooting

- **Web Search is missing from Connection Center:** confirm that
  `EXTERNAL_INTEGRATIONS_ENABLED=true` and that the API restarted successfully.
- **Web Search is absent from AIChat:** create and test an Exa Connection, grant
  access if it is shared, and select it under **Connected Apps**. Web Search is
  intentionally absent from every Skill picker.
- **The API fails during startup:** check `WEB_SEARCH_PROVIDER`, positive
  numeric limits, and the exact length of `API_KEY_ENCRYPTION_KEY`.
- **Calls report that a Connection is required:** create and test an
  organization or personal Exa Connection, then select it under Connected Apps
  or bind it explicitly to the Agent.
- **An Agent call is denied while AIChat works:** verify the Agent's selected
  Connection is active and its `allowed_action_ids` includes the requested
  Action; explicit Agent bindings do not fall back.
- **Calls report an audit failure:** apply the database migration and confirm
  that PostgreSQL and `API_KEY_ENCRYPTION_KEY` are available to the API.
- **Calls report a quota-service failure:** confirm Redis connectivity.
- **Calls report the daily limit was reached:** increase
  `INTEGRATION_ORG_DAILY_LIMIT` deliberately or wait for the next UTC day.

## Migrating from the legacy platform credential

`EXA_API_KEY` is no longer read by the API. Existing environment values can be
removed after their secret has been copied into a new organization or personal
Connection through the authenticated UI. Legacy database rows with
`credential_source=platform` are hidden from catalogs and management lists and
fail closed if referenced.

ZGI does not convert those rows automatically: a legacy row contains no
encrypted provider secret, so converting it without an authenticated
credential submission would create an unusable or misleading Connection.
Create and test a replacement Connection, update AIChat selections and Agent
bindings, then retire the old environment secret.
