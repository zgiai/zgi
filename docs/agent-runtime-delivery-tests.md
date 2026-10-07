# Runtime delivery test gates

Run from the repository root on Linux. The gate verifies the exact Go version in
`api/go.mod` and `sandbox/go.mod` (currently 1.26.2), with CGO enabled. Web gates
require Node.js 22, pnpm 10.12.1 and an existing frozen-lockfile installation.
On Windows, use a Linux development container with this checkout mounted; do not
lower the module version. `ZGI_TEST_PARALLELISM` defaults to 2 for smaller hosts.

```sh
bash scripts/test-runtime-delivery.sh
bash scripts/check-runtime-delivery.sh --api
bash scripts/check-runtime-delivery.sh --sandbox
bash scripts/check-runtime-delivery.sh --web
```

Each gate stops at its first failure. No runtime service or package is installed
by the script. The preflight test uses mocks only to verify command dispatch and
failure handling, not to certify execution backends.

## Isolated PostgreSQL

Never use an application database. Create dedicated disposable PostgreSQL 16
instances for API and Sandbox tests, with no shared persistent application volume.
Before the API gate, enable `uuid-ossp` in its test database. Supply connection
values securely in the environment, not command arguments:

| Gate | Required environment |
|---|---|
| API | `TEST_POSTGRES_DSN`; `SQLMETA_TEST_DB_HOST`, `SQLMETA_TEST_DB_PORT`, `SQLMETA_TEST_DB_USER`, `SQLMETA_TEST_DB_PASS`, `SQLMETA_TEST_DB_NAME` pointing to the same isolated instance |
| Sandbox | `ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN`; `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE` pointing to the same isolated instance; its test role needs database-creation permission |

Install `psql` in the development/CI environment for connection and extension
preflights. Missing settings or an unreachable test database stop the gate. The
API gate injects a fixed test-only connector-encryption key into its subprocess;
it does not read deployed application credentials. Full API tests include workflow
and SQL metadata integration; Sandbox storage tests create and remove their own
databases. Explicitly configured unavailable Sandbox PostgreSQL fails instead of
skipping its tests; cleanup errors also fail the owning test. See
[workflow fixtures](../api/tests/integration/workflow/README.md).

The ordinary CI workflow provisions separate PostgreSQL services, runs both full
Go suites and vet, and runs Web lint/type-check/build plus focused UI regressions.
Existing Kest, migration and registration gates remain separate. Runtime data,
accounts, provider keys and generated test outputs must not enter Git.

## Opt-in real backends

All real gates require `ZGI_RUNTIME_REAL_TESTS=1`, dedicated test resources and the
same Go version. They create real resources, incur provider costs and remove their
own disposable compute. Do not cancel a run without reconciling its owned resources.

| Command | Additional settings and prerequisites |
|---|---|
| `bash scripts/check-runtime-delivery.sh --real-local` | Root-owned native Linux node with containerd/runsc; `ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS`; `ZGI_AGENT_RUNTIME_TEST_IMAGE` pinned with `@sha256:` to the approved standard image |
| `bash scripts/check-runtime-delivery.sh --real-e2b` | Dedicated E2B project; secret `ZGI_AGENT_RUNTIME_E2B_API_KEY`; approved `ZGI_AGENT_RUNTIME_E2B_TEST_PACKAGE_TEMPLATE` built from the current package-enabled Runtime Agent |
| `bash scripts/check-runtime-delivery.sh --real-worker` | Authenticated dedicated Worker; `ZGI_AGENT_RUNTIME_TEST_URL`, secret `ZGI_AGENT_RUNTIME_TEST_API_KEY`, `ZGI_AGENT_RUNTIME_TEST_PROFILE`, `ZGI_AGENT_RUNTIME_TEST_BACKEND` (`local-gvisor` or `e2b`); `ZGI_STANDARD_SKILL_TEST_ROOT` containing original `xlsx`, `pptx`, `slack-gif-creator` directories |

The native and E2B contracts verify installation, native modules, network limits,
compute loss, retained versions, corrupt-artifact rejection and cleanup. The Worker
matrix adds original Skill helpers, output checks, errors, tenant rejection and
termination. Its in-memory export store is not browser download evidence.

The **Agent runtime real backend acceptance** workflow is manually dispatched,
never triggered by pull requests. Configure its protected
`agent-runtime-acceptance` environment with the E2B secret/template above. The
local option needs a dedicated runner selected by
`AGENT_RUNTIME_LOCAL_RUNNER_LABELS` (JSON array, default label
`zgi-runtime-acceptance-linux`), the image/socket variables above and permission
to run the isolated contract as root. No runner or public Worker is provisioned
automatically. These workflow gates do not replace console import/download or
upgrade/rollback drills in [the release checklist](agent-runtime-release-candidate.md).
