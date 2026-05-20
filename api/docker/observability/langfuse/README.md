# Langfuse Self-Hosted Recipe

This recipe does not copy Langfuse's full Docker Compose file into this repository. Langfuse v3 self-hosting uses multiple services including web, worker, Postgres, ClickHouse, Redis, and object storage. The official Compose file is the source of truth and should be used directly.

Official guide:

```text
https://langfuse.com/self-hosting/deployment/docker-compose
```

Official repository:

```text
https://github.com/langfuse/langfuse
```

## Start Langfuse

```bash
git clone https://github.com/langfuse/langfuse.git
cd langfuse
docker compose up -d
```

Open the UI:

```text
http://localhost:3000
```

Create or initialize a project and get the project's public key and secret key.

## Connect ZGI Directly

For most deployments, export ZGI API traces directly to Langfuse over OTLP HTTP. If these variables are not configured, tracing remains disabled by default and the API still starts normally.

```env
OTEL_ENABLED=true
OTEL_TRACES_SAMPLE_RATE=1.0
LANGFUSE_ENABLED=true
LANGFUSE_PUBLIC_KEY=pk-lf-...
LANGFUSE_SECRET_KEY=sk-lf-...
LANGFUSE_BASE_URL=https://cloud.langfuse.com
OTEL_LLM_LANGFUSE_ATTRIBUTES=true
OTEL_LLM_CAPTURE_CONTENT=summary
OTEL_LLM_CAPTURE_MAX_CHARS=65536
```

Set `LANGFUSE_ENABLED=false` only when keys are present but Langfuse export should be explicitly ignored. `LANGFUSE_AUTH_STRING` by itself is reserved for Collector mode and does not redirect API traces away from `OTEL_EXPORTER_OTLP_ENDPOINT`.

Use `LANGFUSE_OTEL_ENDPOINT=https://jp.cloud.langfuse.com/api/public/otel` or a self-hosted `/api/public/otel` URL when you need a specific region or deployment.

## Connect ZGI Collector

Set these variables in the ZGI API environment:

```env
OTEL_COLLECTOR_CONFIG=./docker/otel-collector.external-langfuse.yaml
LANGFUSE_OTEL_ENDPOINT=http://host.docker.internal:3000/api/public/otel
LANGFUSE_AUTH_STRING=<base64-public-key-colon-secret-key>
OTEL_LLM_LANGFUSE_ATTRIBUTES=true
OTEL_LLM_CAPTURE_CONTENT=summary
OTEL_LLM_CAPTURE_MAX_CHARS=65536
```

Use `docker/otel-collector.external-jaeger-langfuse.yaml` instead when exporting to both Jaeger and Langfuse.

Set `OTEL_LLM_CAPTURE_CONTENT=full` only when you intentionally want prompt and output content sent to Langfuse. Keep `summary` for shared or production environments unless data policy allows full content capture.

Generate the auth value:

```bash
printf '%s' '<public_key>:<secret_key>' | base64
```

## Production Notes

The official Docker Compose setup is suitable for local and low-scale VM deployments. For high availability or high throughput, follow the official Kubernetes or Terraform deployment guides.
