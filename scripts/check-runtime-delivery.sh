#!/usr/bin/env bash
set -euo pipefail
set +x

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-}"
parallelism="${ZGI_TEST_PARALLELISM:-2}"

fail() { echo "runtime-delivery: $*" >&2; exit 2; }
require_env() {
  local name
  for name in "$@"; do
    [[ -n "${!name:-}" ]] || fail "set ${name} explicitly for this gate"
  done
}
check_go() {
  local expected sandbox_version actual
  expected="$(awk '$1 == "go" {sub(/\r$/, "", $2); print $2}' "${repo_root}/api/go.mod")"
  sandbox_version="$(awk '$1 == "go" {sub(/\r$/, "", $2); print $2}' "${repo_root}/sandbox/go.mod")"
  [[ "$expected" == "$sandbox_version" ]] || fail "API and Sandbox Go versions differ"
  actual="$(go env GOVERSION)"
  [[ "$actual" == "go${expected}" ]] || fail "use Go ${expected}; found ${actual}"
  [[ "$(go env CGO_ENABLED)" == 1 ]] || fail "CGO must be enabled for SQLite tests"
}
check_postgres() {
  command -v psql >/dev/null || fail "install the PostgreSQL client for the database preflight"
  # Connection values stay in the environment, never command arguments or output.
  psql -X -qAt -v ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null 2>&1 || fail "dedicated test PostgreSQL is unavailable"
}

[[ "$parallelism" =~ ^[1-9][0-9]*$ ]] || fail "ZGI_TEST_PARALLELISM must be a positive integer"
case "$mode" in
  --api)
    check_go
    require_env TEST_POSTGRES_DSN SQLMETA_TEST_DB_HOST SQLMETA_TEST_DB_PORT \
      SQLMETA_TEST_DB_USER SQLMETA_TEST_DB_PASS SQLMETA_TEST_DB_NAME
    export PGHOST="$SQLMETA_TEST_DB_HOST" PGPORT="$SQLMETA_TEST_DB_PORT"
    export PGUSER="$SQLMETA_TEST_DB_USER" PGPASSWORD="$SQLMETA_TEST_DB_PASS" PGDATABASE="$SQLMETA_TEST_DB_NAME"
    export PGCONNECT_TIMEOUT=10
    check_postgres
    [[ "$(psql -X -qAt -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM pg_extension WHERE extname = 'uuid-ossp'" 2>/dev/null)" == 1 ]] || fail "enable uuid-ossp in the dedicated API test database"
    # This key is only an isolated-test fixture, never an application credential.
    export CONNECTOR_CREDENTIAL_ACTIVE_KEY_ID=runtime-delivery-test
    export CONNECTOR_CREDENTIAL_KEYS_JSON='{"runtime-delivery-test":"12345678901234567890123456789012"}'
    cd "${repo_root}/api"
    go test -p="$parallelism" -count=1 -timeout=2m ./...
    go vet -p="$parallelism" ./...
    ;;
  --sandbox)
    check_go
    require_env ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN PGHOST PGPORT PGUSER PGPASSWORD PGDATABASE
    export PGCONNECT_TIMEOUT=10
    check_postgres
    cd "${repo_root}/sandbox"
    go test -p="$parallelism" -count=1 -timeout=3m ./...
    go vet -p="$parallelism" ./...
    ;;
  --web)
    [[ "$(node -p 'process.versions.node.split(".")[0]')" == 22 ]] || fail "use Node.js 22"
    [[ "$(pnpm --version)" == 10.12.1 ]] || fail "use pnpm 10.12.1"
    cd "${repo_root}/web"
    pnpm lint
    pnpm type-check
    pnpm test:agent-execution-job-presentation
    pnpm test:webapp-tool-detail-visibility
    pnpm test:protected-execution-files
    pnpm test:chat-runtime-stream-recovery
    pnpm test:chat-runtime-status
    pnpm test:chat-stream-decoder
    pnpm test:chat-surface-profile
    pnpm test:chat-pending-interaction
    pnpm test:chat-runtime-replay
    pnpm test:user-input-response-history
    pnpm build
    ;;
  --real-local|--real-e2b|--real-worker|--real-documents)
    [[ "${ZGI_RUNTIME_REAL_TESTS:-}" == 1 ]] || fail "real contracts require ZGI_RUNTIME_REAL_TESTS=1 and dedicated test resources"
    check_go
    case "$mode" in
      --real-local)
        [[ "$(uname -s)" == Linux && "$(id -u)" == 0 ]] || fail "use a root-owned native Linux gVisor test node"
        require_env ZGI_AGENT_RUNTIME_TEST_IMAGE ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS
        [[ "$ZGI_AGENT_RUNTIME_TEST_IMAGE" =~ @sha256:[a-f0-9]{64}$ ]] || fail "pin the local Profile to an OCI image digest"
        cd "${repo_root}/sandbox"
        go test -p=1 -tags=agent_runtime_integration -count=1 -v -timeout=25m \
          ./internal/localruntime/containerd \
          -run '^(TestContainerdGVisorContract|TestContainerdGVisorResourceLimits|TestGVisorPackageInstallRecovery)$'
        ;;
      --real-e2b)
        require_env ZGI_AGENT_RUNTIME_E2B_API_KEY ZGI_AGENT_RUNTIME_E2B_TEST_PACKAGE_TEMPLATE
        cd "${repo_root}/sandbox"
        go test -p=1 -tags=e2b_integration -count=1 -v -timeout=20m \
          ./internal/hostedruntime/e2b -run '^TestE2BPackageInstallContract$'
        ;;
      --real-worker|--real-documents)
        require_env ZGI_STANDARD_SKILL_TEST_ROOT ZGI_AGENT_RUNTIME_TEST_URL \
          ZGI_AGENT_RUNTIME_TEST_API_KEY ZGI_AGENT_RUNTIME_TEST_PROFILE ZGI_AGENT_RUNTIME_TEST_BACKEND
        case "$ZGI_AGENT_RUNTIME_TEST_BACKEND" in local-gvisor|e2b) ;; *) fail "unsupported real Worker backend" ;; esac
        skills=(xlsx pptx slack-gif-creator)
        test_name=TestAgentExecutionStandardSkillMatrix
        if [[ "$mode" == --real-documents ]]; then
          skills=(docx pdf)
          test_name=TestAgentExecutionDocumentSkills
        fi
        for skill in "${skills[@]}"; do
          [[ -f "${ZGI_STANDARD_SKILL_TEST_ROOT}/${skill}/SKILL.md" ]] || fail "missing external ${skill} Skill"
        done
        export CONNECTOR_CREDENTIAL_ACTIVE_KEY_ID=runtime-delivery-test
        export CONNECTOR_CREDENTIAL_KEYS_JSON='{"runtime-delivery-test":"12345678901234567890123456789012"}'
        cd "${repo_root}/api"
        go test -p=1 -tags=agent_runtime_integration -count=1 -v -timeout=15m \
          ./internal/capabilities/agentexecution/runtime -run "^${test_name}$"
        ;;
    esac
    ;;
  *) fail "usage: $0 --api|--sandbox|--web|--real-local|--real-e2b|--real-worker|--real-documents" ;;
esac
