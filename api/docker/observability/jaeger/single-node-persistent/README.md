# Jaeger Single-Node Persistent

This recipe runs Jaeger v2 with Badger storage. It is suitable for a single-node, low-maintenance self-hosted setup where trace data should survive container restarts.

Badger is not a horizontal scaling backend. Use the OpenSearch recipe when you need a longer-lived setup with a storage service that can grow beyond one process.

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

Trace data is stored in the `jaeger-badger-data` Docker volume.

Default retention:

```env
JAEGER_BADGER_TTL=168h
JAEGER_BADGER_ARCHIVE_TTL=720h
```
