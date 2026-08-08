# LLM invocation content audit

ZGI can optionally keep a short-lived, business-facing copy of LLM inputs and
outputs for troubleshooting agents and workflows. This store is independent
from usage billing and from OpenTelemetry/Langfuse.

Content capture is disabled by default. A deployment operator must first set:

```env
LLM_INVOCATION_CONTENT_AVAILABLE=true
LLM_INVOCATION_CONTENT_MAX_BYTES=65536
LLM_INVOCATION_CONTENT_RETENTION_DAYS=14
LLM_INVOCATION_CONTENT_QUEUE_SIZE=1000
LLM_INVOCATION_CONTENT_BATCH_SIZE=50
```

An organization owner or administrator can then enable capture from the LLM
invocation log page. Both switches must be enabled. Changing the organization
switch does not require an application restart.

## Safety and performance

- Content is stored separately from `llm_usage_bills`.
- When capture is enabled, Gateway requests create a redacted, size-bounded,
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
