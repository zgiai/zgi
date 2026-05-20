# Jaeger All-in-One

This recipe runs the official Jaeger v2 all-in-one image for quick local validation.

It uses transient in-memory storage. Trace data is lost when the container restarts.

## Start

```bash
cp .env.example .env
docker compose --env-file .env up -d
```

Open the UI:

```text
http://localhost:16686
```

## Connect ZGI Collector

Set these variables in the ZGI API environment:

```env
OTEL_COLLECTOR_CONFIG=./docker/otel-collector.external-jaeger.yaml
JAEGER_OTLP_ENDPOINT=host.docker.internal:14317
```

Use `docker/otel-collector.external-jaeger-langfuse.yaml` instead when exporting to both Jaeger and Langfuse.

## Notes

Jaeger listens on standard OTLP ports inside the container. The host ports default to `14317` and `14318` to avoid conflicts with the ZGI OpenTelemetry Collector.
