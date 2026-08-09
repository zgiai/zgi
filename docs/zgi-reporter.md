# ZGI Reporter

ZGI Reporter is the provider-neutral error and event reporting facade used by
the API and web application. Product code reports stable ZGI events and does
not import Sentry, OpenTelemetry, or another vendor SDK directly.

## Selecting reporters

No reporting destination is configured by default. For compatibility, a
provider is auto-enabled when its existing provider-specific configuration is
present. Use `ZGI_REPORTERS` to make the selection explicit:

```env
# API/server reporters. Multiple values may be comma-separated.
ZGI_REPORTERS=sentry,otel

# Each selected provider still requires its own configuration.
SENTRY_DSN=https://public-key@sentry.example.com/project-id
SENTRY_ENVIRONMENT=production
SENTRY_TRACES_SAMPLE_RATE=0.1
# Set this to the deployed image tag or commit SHA for useful release grouping.
APP_VERSION=git-sha-or-image-tag
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=https://otel.example.com
```

Use `ZGI_REPORTERS=none` to disable all API/server reporters even when provider
credentials are present.

The browser uses the public equivalent:

```env
NEXT_PUBLIC_ZGI_REPORTERS=sentry
NEXT_PUBLIC_SENTRY_DSN=https://public-key@sentry.example.com/project-id
```

Use `NEXT_PUBLIC_ZGI_REPORTERS=none` to disable browser reporting. Server and
edge Sentry initialization also respects `ZGI_REPORTERS`.

## Reporting an event

API code uses stable, low-cardinality names and provider-neutral attributes:

```go
observability.CaptureError(ctx, "file.parse.failed", err,
    observability.Tag("file.type", fileType),
    observability.Attribute("file.size_bucket", sizeBucket),
)
```

Web code uses the matching facade:

```ts
captureError(error, 'http.request.failed', {
  tags: { endpoint: 'console-api' },
  attributes: { http: { path: '/console/api/files', status: 500 } },
});
```

Do not use unique IDs as indexed tags. Put request, account, tenant, workflow,
or document identifiers in non-indexed attributes instead.

## Adding another platform

The backend extension point is `observability.Reporter`. Register the adapter
when constructing `observability.NewZGIReporter`; the facade sends every event
to each registered adapter and isolates adapter failures.

The web extension point is `Reporter` from `@/lib/observability`:

```ts
import { registerReporter, type Reporter } from '@/lib/observability';

const customReporter: Reporter = {
  name: 'custom',
  report(event) {
    // Translate the provider-neutral event to the custom platform SDK.
  },
};

registerReporter(customReporter);
```

With no adapters, ZGI Reporter is a No-op and performs no network requests.

## Operational behavior

Sentry is for errors, performance traces, and alerting. OpenTelemetry exports
vendor-neutral traces; Langfuse can consume those traces for LLM invocations,
tokens, and cost analysis. Successful LLM calls are therefore not duplicated
as Sentry errors.

Expected caller cancellation (`context.Canceled`) is not reported. Dependency
timeouts remain reportable. When both Sentry and OpenTelemetry are enabled,
OTel exporter and queue failures are emitted to Sentry as the rate-limited
`observability.otel.runtime_failed` event. This path reports directly to Sentry
so an unavailable OTel exporter cannot recursively report through itself.

Sentry events use readable operator-facing issue titles while retaining stable
ZGI event names in the `zgi.event` tag and fingerprint for filtering and
grouping. For example, `llm.provider.stream_failed` is displayed as
`LLM provider stream failed`, not an internal wrapper type. Configure
`SENTRY_ENVIRONMENT` per deployment and set `APP_VERSION` to an immutable
release identifier so regressions can be attributed to a deployment. Keep
`SENTRY_TRACES_SAMPLE_RATE` low in production; values are clamped to the
supported `0` to `1` range.

Errors also carry a provider-neutral classification:

- `error.category` describes the failure class, such as `configuration`,
  `dependency`, `database`, or `timeout`.
- `error.source` identifies who can act: `tenant`, `provider`, `zgi`, or
  `infrastructure`.
- `error.code` is a stable, low-cardinality code such as
  `model_route_not_configured` or `postgres_42601`.
- `error.retryable` distinguishes transient failures from deterministic ones.

Request-scoped deduplication prevents an error already reported by a module
from being reported again by the HTTP fallback middleware. Provider failover
attempts are warnings while another route remains; only the final failed
attempt is an error. Internal Reporter frames are removed from Sentry error
stacks so the first useful application call site is shown instead.

## Privacy boundary

All ZGI Reporter events pass through centralized sanitization before reaching
an adapter. Known credential, prompt, request/response body, raw SQL, and
workflow input fields are removed or redacted. Error objects are replaced with
sanitized copies, URL query strings and request environment data are removed,
request headers are allowlisted, and string/collection payloads are bounded.
Browser and server Sentry clients disable default PII collection.

This sanitization is a final safety boundary, not permission to report raw
content. Call sites should send metadata such as content length, file type, or
processing stage instead of prompts, file contents, response bodies, tokens,
cookies, or credentials.
