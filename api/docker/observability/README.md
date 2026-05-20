# Observability Recipes

These Docker recipes are optional observability platforms for local and self-hosted deployments. They are not part of the ZGI API runtime.

The default topology is:

```text
zgi-api -> OpenTelemetry Collector -> Jaeger and/or Langfuse
```

Full setup guide:

```text
docker/observability/OTEL_COLLECTOR_JAEGER_LANGFUSE.md
```

## Recipes

| Recipe | Purpose |
| --- | --- |
| `jaeger/all-in-one` | Fast local validation. Uses transient in-memory storage. |
| `jaeger/single-node-persistent` | Single-node Jaeger with Badger-backed persistent trace storage. |
| `jaeger/opensearch` | Longer-lived Jaeger setup backed by OpenSearch. |
| `langfuse` | Self-hosted Langfuse guide using the official Langfuse Docker Compose. |

## Collector Endpoint Defaults

The ZGI Collector exposes OTLP on host ports `4317` and `4318` by default. To avoid host port conflicts, Jaeger recipes expose Jaeger's OTLP ports as:

```env
JAEGER_OTLP_GRPC_PORT=14317
JAEGER_OTLP_HTTP_PORT=14318
```

Use this in the ZGI Collector environment:

```env
JAEGER_OTLP_ENDPOINT=host.docker.internal:14317
```
