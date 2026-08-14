# Knowledge Graph Deployment

Neo4j is an optional deployment dependency required only when knowledge graph capability is used. The Docker configuration restores the original `knowledge` profile behavior: the default, `--knowledge`, and `--full` modes include Neo4j, while `--core` does not.

## Installation

The repository Docker stack installs Neo4j 5.15 Community Edition with APOC, persistent data and log volumes, and a `cypher-shell` health check. Copy the environment templates, set the same `NEO4J_USERNAME` and `NEO4J_PASSWORD` in `docker/.env` and `api/.env.docker`, and start a mode that includes the knowledge profile:

```bash
cp docker/.env.example docker/.env
cp api/.env.docker.example api/.env.docker
./dev/start-docker --knowledge
```

The default and `--full` modes enable both the runtime and knowledge profiles. The `--core` mode starts only Nginx, API, Web, PostgreSQL, and Redis; the API remains available, but graph operations report that the graph runtime is not configured.

For an external Neo4j installation, configure the API with `NEO4J_URI`, `NEO4J_USERNAME`, `NEO4J_PASSWORD`, and `NEO4J_DATABASE`. Use a Neo4j driver URI such as `neo4j+s://graph.example.com:7687`; do not embed credentials in the URI.

## Health Diagnostics

Use `/ping` for API liveness. The authenticated graph capability endpoint exposes only sanitized state and reason codes. Connection strings, usernames, passwords, and raw driver errors are never returned by the endpoint.

```bash
curl --fail http://localhost:2679/ping
docker compose --env-file docker/.env ps neo4j api
docker compose --env-file docker/.env logs --tail=100 neo4j api
```

If Neo4j becomes unavailable after startup, restore connectivity and verify the graph capability endpoint. The API liveness endpoint remains available. The runtime monitor updates graph capability state, while the graph outbox reconciler reclaims expired work and retries unconfirmed graph runs and visibility projections.

## Upgrade

Before changing the pinned Neo4j image version, read the Neo4j upgrade notes for every version crossed and take a verified backup. Stop API writers, back up Neo4j and PostgreSQL as one maintenance operation, update the Compose template, and start Neo4j before resuming graph writes. Confirm Neo4j health, graph capability, graph status, and a bounded graph query before reopening writes.

Never skip required Neo4j store migration steps or move a data volume directly across unsupported major versions.

## Backup and Restore

The knowledge graph is a projection of PostgreSQL evidence, but Neo4j and PostgreSQL should still be backed up together to minimize recovery work. For Community Edition, use an offline database dump during a maintenance window or take a consistent volume snapshot after stopping Neo4j. Protect backup credentials and storage with the same controls as production data.

Restore PostgreSQL first, restore the matching Neo4j snapshot, then start Neo4j and the API. If the snapshots are not from the same point, rebuild affected knowledge graphs from the application. Keep the old backup until graph status, active-source filtering, retrieval, and document replacement behavior have been verified.
