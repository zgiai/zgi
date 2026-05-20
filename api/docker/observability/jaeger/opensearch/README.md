# Jaeger with OpenSearch

This recipe runs Jaeger v2 with OpenSearch as the trace storage backend. Use this as the recommended long-lived Docker recipe when data should survive restarts and the storage service should be separable from the Jaeger process.

This is still a single-node Docker Compose recipe. For high availability, run OpenSearch and Jaeger as managed infrastructure or on Kubernetes.

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

## Storage

OpenSearch data is stored in the `jaeger-opensearch-data` Docker volume.

The recipe disables OpenSearch's security plugin to keep local setup simple. Do not expose this service directly to the internet without adding authentication, TLS, backups, and resource limits.
