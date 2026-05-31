# zgi-sandbox Configuration

This document lists the supported `ZGI_SANDBOX_` environment variables. Keep it
in sync with `sandbox/internal/config/config.go`, `sandbox/.env.example`, and
`sandbox/.env.production.example`.

## Local Defaults

Use `sandbox/.env.example` for local development. It uses the preview runtime
backend, local service URLs, short default TTLs, and conservative file and output
limits.

## Production Defaults

Use `sandbox/.env.production.example` as the starting point for production
deployments. Production deployments must set `ZGI_SANDBOX_ENV=production`, a
non-empty `ZGI_SANDBOX_API_KEY`, and a runtime backend that enforces network
policy.

The production example uses `ZGI_SANDBOX_RUNTIME_BACKEND=linux-secure`. That
backend also requires `ZGI_SANDBOX_SECURE_ROOTFS` and
`ZGI_SANDBOX_BWRAP_BINARY`. When `ZGI_SANDBOX_DEPENDENCY_ROOTFS_DIR` is set,
the secure runtime resolves dependency profiles from child rootfs directories
named after each profile.
When a sandbox selects a dependency profile, the secure Linux backend activates
profile-local Python and Node paths under `/opt/zgi/profiles/<profile>`.
Profile environment values are applied after request environment values so user
code cannot override the managed profile path contract. When profile-specific
rootfs directories are enabled, the backend also verifies the built profile
manifest and read-only binds `/opt/zgi/profiles/<profile>` before execution.

## Variables

| Variable | Purpose | Local default | Production guidance |
| --- | --- | --- | --- |
| `ZGI_SANDBOX_ADVERTISE_URL` | Internal URL advertised to peer services. | `http://localhost:2660` | Set to the internal service URL. |
| `ZGI_SANDBOX_API_KEY` | Optional API key for protected operator endpoints. | empty | Required in production. |
| `ZGI_SANDBOX_BWRAP_BINARY` | Bubblewrap binary used by the secure Linux backend. | `bwrap` | Set to the installed binary name or absolute path. |
| `ZGI_SANDBOX_CACHE_TTL_SECONDS` | Sandbox endpoint cache TTL. | `30` | Keep small enough for endpoint updates to propagate quickly. |
| `ZGI_SANDBOX_COMMAND_TIMEOUT_SECONDS` | Default command execution timeout. | `15` | Keep bounded for workflow use. |
| `ZGI_SANDBOX_DATABASE_URL` | PostgreSQL connection URL. | empty in `.env.example`; composed in Docker Compose. | Required for production metadata persistence. |
| `ZGI_SANDBOX_DATA_DIR` | Local data directory for sandbox files and metadata. | `/var/lib/zgi-sandbox` | Use persistent storage. |
| `ZGI_SANDBOX_DEPENDENCY_ROOTFS_DIR` | Optional parent directory for dependency-profile-specific secure rootfs directories. | empty | Set when dependency profiles are materialized as separate rootfs directories. |
| `ZGI_SANDBOX_DEPENDENCY_PROFILE_BUILD_TIMEOUT_SECONDS` | Maximum dependency profile build timeout. | `600` | Tune to the largest managed profile build. |
| `ZGI_SANDBOX_EGRESS_PROXY_MAX_BODY_BYTES` | Maximum request body accepted and response body returned by the egress proxy. | `1048576` | Keep bounded for workflow use and auditability. |
| `ZGI_SANDBOX_ENV` | Runtime environment name. | `local` | Use `production` or `prod` for production. |
| `ZGI_SANDBOX_INTERACTIVE_TTL_SECONDS` | Interactive sandbox TTL. | `3600` | Keep bounded and align with cleanup policy. |
| `ZGI_SANDBOX_LITE_MAX_WORKERS` | Maximum short-code worker count. | `4` | Tune to host capacity. |
| `ZGI_SANDBOX_LITE_WORKER_TIMEOUT` | Short-code worker timeout in seconds. | `5` | Keep short for stateless workflow code. |
| `ZGI_SANDBOX_MAX_ACTIVE` | Maximum active sandboxes per worker. | `6` | Tune to host capacity. |
| `ZGI_SANDBOX_MAX_ACTIVE_PER_ORGANIZATION` | Optional active sandbox quota per organization. | `0` disabled | Set for shared deployments. |
| `ZGI_SANDBOX_MAX_ARTIFACT_BYTES_PER_ORGANIZATION` | Optional artifact byte quota per organization. | `0` disabled | Set for shared deployments. |
| `ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_BYTES` | Maximum total bytes represented by one artifact manifest. | `0` disabled | Set to prevent oversized artifact responses. |
| `ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_FILES` | Maximum files represented by one artifact manifest. | `0` disabled | Set to prevent oversized artifact responses. |
| `ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS` | Optional service-wide concurrent execution limit. | `0` disabled | Set to host capacity. |
| `ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_ORGANIZATION` | Optional concurrent execution limit per organization. | `0` disabled | Set for shared deployments. |
| `ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_PROFILE` | Optional concurrent execution limit per command profile. | `0` disabled | Set when specific profiles are expensive. |
| `ZGI_SANDBOX_MAX_DEPENDENCY_PROFILES_PER_ORGANIZATION` | Optional active dependency profile limit per organization. | `0` disabled | Set for shared deployments. |
| `ZGI_SANDBOX_MAX_DEPENDENCY_PROFILE_SIZE_BYTES` | Maximum managed dependency profile size. | `536870912` | Tune to artifact storage and startup budget. |
| `ZGI_SANDBOX_MAX_EXECUTIONS_PER_MINUTE_PER_ORGANIZATION` | Optional execution rate limit per organization. | `0` disabled | Set for shared deployments. |
| `ZGI_SANDBOX_MAX_FILE_SIZE_KB` | Maximum uploaded file size. | `256` | Keep aligned with API upload policy. |
| `ZGI_SANDBOX_MAX_NETWORK_REQUESTS_PER_MINUTE_PER_ORGANIZATION` | Optional egress proxy request rate limit per organization. | `0` disabled | Set for shared deployments that enable network egress. |
| `ZGI_SANDBOX_MAX_QUEUED_EXECUTIONS_PER_ORGANIZATION` | Optional queued execution limit per organization. | `0` disabled | Set to prevent queue buildup. |
| `ZGI_SANDBOX_MAX_WORKSPACE_BYTES` | Optional workspace byte limit per sandbox. | `0` disabled | Set for disk protection. |
| `ZGI_SANDBOX_MAX_WORKSPACE_BYTES_PER_ORGANIZATION` | Optional workspace byte limit per organization. | `0` disabled | Set for shared deployments. |
| `ZGI_SANDBOX_MAX_WORKSPACE_FILES` | Optional workspace file count limit per sandbox. | `0` disabled | Set for inode protection. |
| `ZGI_SANDBOX_OBSERVER_MAX_EVENTS` | Maximum retained observer events. | `10000` | Tune to storage budget. |
| `ZGI_SANDBOX_OBSERVER_RETENTION_DAYS` | Observer event retention by age. | `7` | Tune to audit requirements. |
| `ZGI_SANDBOX_OUTPUT_LIMIT_KB` | Default stdout/stderr output limit. | `64` | Keep bounded for API responses. |
| `ZGI_SANDBOX_PROXY_TIMEOUT_SECONDS` | Interactive proxy timeout. | `20` | Tune to interactive preview needs. |
| `ZGI_SANDBOX_PUBLIC_BASE_URL` | Public base URL for routed endpoints. | `http://localhost:2660` | Set to the public sandbox URL. |
| `ZGI_SANDBOX_QUEUE_TIMEOUT_MS` | Maximum wait time for queued executions. | `5000` | Keep short for workflow responsiveness. |
| `ZGI_SANDBOX_REDIS_ADDR` | Redis address for endpoint cache coordination. | `sandbox-redis:6379` | Set when Redis is available. |
| `ZGI_SANDBOX_REDIS_DB` | Redis database number. | `0` | Use an isolated database where needed. |
| `ZGI_SANDBOX_REDIS_PASSWORD` | Redis password. | empty | Set when Redis requires authentication. |
| `ZGI_SANDBOX_RUNTIME_BACKEND` | Runtime backend selector. | `preview` | Use `linux-secure` in production. |
| `ZGI_SANDBOX_SECURE_RUNTIME_CPU_SECONDS` | Secure Linux backend CPU time limit in seconds. | `2` | Keep positive in production and tune to command profile timeouts. |
| `ZGI_SANDBOX_SECURE_RUNTIME_MEMORY_BYTES` | Secure Linux backend address-space memory limit. | `268435456` | Set according to profile memory budget. |
| `ZGI_SANDBOX_SECURE_RUNTIME_OPEN_FILE_LIMIT` | Secure Linux backend open file descriptor limit. | `128` | Keep low unless a managed profile needs more descriptors. |
| `ZGI_SANDBOX_SECURE_RUNTIME_PROCESS_LIMIT` | Secure Linux backend process count limit. | `64` | Keep low enough to contain fork-heavy workloads. |
| `ZGI_SANDBOX_SECURE_ROOTFS` | Root filesystem path for the secure Linux backend. | empty | Required when using `linux-secure`. |
| `ZGI_SANDBOX_SERVER_PORT` | HTTP server port. | `2660` | Set to the exposed service port. |
| `ZGI_SANDBOX_SESSION_TTL_SECONDS` | Session sandbox TTL. | `1800` | Keep bounded and align with cleanup policy. |
| `ZGI_SANDBOX_SHUTDOWN_TIMEOUT_SECONDS` | Graceful shutdown timeout. | `10` | Keep long enough for in-flight cleanup. |
| `ZGI_SANDBOX_WORKER_ID` | Worker identifier included in metadata and diagnostics. | `zgi-sandbox-local` | Set to a stable worker identity. |

## Validation

Run the configuration documentation gate after changing config names or
examples:

```bash
make -C sandbox check-config-docs
```
