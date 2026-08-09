# LLM invocation content audit

ZGI can optionally keep a short-lived, business-facing copy of LLM inputs and
outputs for troubleshooting agents and workflows. This store is independent
from usage billing and from OpenTelemetry/Langfuse.

Content capture is disabled for every organization by default. An organization
owner or administrator can enable it directly from the LLM invocation log page;
no deployment-level feature flag or application restart is required.

Deployments may optionally tune the storage limits:

```env
LLM_INVOCATION_CONTENT_MAX_BYTES=65536
LLM_INVOCATION_CONTENT_RETENTION_DAYS=14
LLM_INVOCATION_CONTENT_QUEUE_SIZE=1000
LLM_INVOCATION_CONTENT_BATCH_SIZE=50
```

Changing the organization switch takes effect without an application restart.

## Safety and performance

- Content is stored separately from `llm_usage_bills`.
- When an organization enables capture, Gateway requests create a redacted, size-bounded,
  immutable snapshot and attempt a non-blocking enqueue. Database writes are
  batched in the background. Queue pressure or a database outage drops the
  optional content copy without affecting the model response, billing, or
  invocation metadata.
- Known credential fields and common bearer/API-key patterns are redacted
  before content enters the asynchronous queue.
- Input and output are bounded independently. The default maximum is 64 KiB.
- Content expires after 14 days by default. The deployment retention value is
  constrained to 1–30 days. Expired rows continue to be cleaned if capture is
  later disabled.
- Only organization owners and administrators can request content. Every
  successful sensitive read is written to `llm_invocation_content_views` in
  the same transaction; if the audit write fails, content is not returned.

The initial implementation captures Chat Completions, including streaming and
non-streaming calls. Invocation metadata remains available for all Gateway
protocols even when content capture is disabled or unsupported.
