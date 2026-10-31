#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_root="$(mktemp -d /tmp/zgi-runtime-gate-test.XXXXXXXX)"
[[ "$test_root" == /tmp/zgi-runtime-gate-test.* && -d "$test_root" && ! -L "$test_root" ]]
trap 'rm -rf -- "$test_root"' EXIT
mkdir "$test_root/bin"
export TEST_CALLS="$test_root/calls"
export PATH="$test_root/bin:$PATH"

cat >"$test_root/bin/go" <<'MOCK'
#!/usr/bin/env bash
case "$*" in
  'env GOVERSION') echo "${MOCK_GO_VERSION:-go1.26.2}" ;;
  'env CGO_ENABLED') echo "${MOCK_CGO:-1}" ;;
  *) echo "go $*" >>"$TEST_CALLS" ;;
esac
MOCK
cat >"$test_root/bin/psql" <<'MOCK'
#!/usr/bin/env bash
[[ "${MOCK_POSTGRES_FAIL:-0}" == 0 ]] || exit 1
echo "${MOCK_EXTENSION_COUNT:-1}"
MOCK
cat >"$test_root/bin/node" <<'MOCK'
#!/usr/bin/env bash
echo "${MOCK_NODE_MAJOR:-22}"
MOCK
cat >"$test_root/bin/pnpm" <<'MOCK'
#!/usr/bin/env bash
if [[ "$*" == --version ]]; then echo "${MOCK_PNPM_VERSION:-10.12.1}"; exit; fi
echo "pnpm $*" >>"$TEST_CALLS"
[[ "$*" != "${MOCK_PNPM_FAIL:-}" ]]
MOCK
cat >"$test_root/bin/id" <<'MOCK'
#!/usr/bin/env bash
echo "${MOCK_UID:-0}"
MOCK
cat >"$test_root/bin/uname" <<'MOCK'
#!/usr/bin/env bash
echo Linux
MOCK
chmod +x "$test_root/bin/"*

expect_failure() {
  : >"$TEST_CALLS"
  if "$@" >"$test_root/output" 2>&1; then echo "expected gate failure" >&2; exit 1; fi
  [[ ! -s "$TEST_CALLS" ]] || { echo "failed preflight ran a test" >&2; exit 1; }
}
gate=(bash "$repo_root/scripts/check-runtime-delivery.sh")
# Clear inherited opt-in flags and production connection settings.
unset ZGI_RUNTIME_REAL_TESTS TEST_POSTGRES_DSN ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN
unset ZGI_AGENT_RUNTIME_E2B_API_KEY ZGI_AGENT_RUNTIME_E2B_TEST_PACKAGE_TEMPLATE
expect_failure "${gate[@]}" --real-e2b
expect_failure "${gate[@]}" --api
expect_failure "${gate[@]}" --sandbox
expect_failure env MOCK_GO_VERSION=go1.25.0 "${gate[@]}" --api
expect_failure env MOCK_CGO=0 "${gate[@]}" --api
expect_failure env ZGI_TEST_PARALLELISM=0 "${gate[@]}" --web
expect_failure env MOCK_NODE_MAJOR=20 "${gate[@]}" --web
expect_failure env MOCK_PNPM_VERSION=10.27.0 "${gate[@]}" --web
expect_failure env ZGI_RUNTIME_REAL_TESTS=1 "${gate[@]}" --real-e2b

export TEST_POSTGRES_DSN='postgres://postgres@test/db'
export SQLMETA_TEST_DB_HOST=test SQLMETA_TEST_DB_PORT=5432 SQLMETA_TEST_DB_USER=postgres
export SQLMETA_TEST_DB_PASS=test-only SQLMETA_TEST_DB_NAME=db
expect_failure env MOCK_POSTGRES_FAIL=1 "${gate[@]}" --api
expect_failure env MOCK_EXTENSION_COUNT=0 "${gate[@]}" --api
"${gate[@]}" --api
grep -Fx 'go test -p=2 -count=1 -timeout=2m ./...' "$TEST_CALLS" >/dev/null
grep -Fx 'go vet -p=2 ./...' "$TEST_CALLS" >/dev/null

export ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN="$TEST_POSTGRES_DSN"
export PGHOST=test PGPORT=5432 PGUSER=postgres PGPASSWORD=test-only PGDATABASE=db
"${gate[@]}" --sandbox
grep -Fx 'go test -p=2 -count=1 -timeout=3m ./...' "$TEST_CALLS" >/dev/null
export ZGI_RUNTIME_REAL_TESTS=1 ZGI_AGENT_RUNTIME_E2B_API_KEY=test-only ZGI_AGENT_RUNTIME_E2B_TEST_PACKAGE_TEMPLATE=approved-test
"${gate[@]}" --real-e2b
grep -F 'TestE2BPackageInstallContract' "$TEST_CALLS" >/dev/null
export ZGI_AGENT_RUNTIME_TEST_IMAGE=not-pinned ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS=/run/test-containerd.sock
expect_failure env MOCK_UID=1000 "${gate[@]}" --real-local
expect_failure "${gate[@]}" --real-local
ZGI_AGENT_RUNTIME_TEST_IMAGE="test/image@sha256:$(printf '%064d' 0)"
export ZGI_AGENT_RUNTIME_TEST_IMAGE
"${gate[@]}" --real-local
grep -F 'TestGVisorPackageInstallRecovery' "$TEST_CALLS" >/dev/null

export ZGI_STANDARD_SKILL_TEST_ROOT="$test_root/skills" ZGI_AGENT_RUNTIME_TEST_URL=http://test-worker
export ZGI_AGENT_RUNTIME_TEST_API_KEY=test-only ZGI_AGENT_RUNTIME_TEST_PROFILE=test-profile
export ZGI_AGENT_RUNTIME_TEST_BACKEND=unsupported
expect_failure "${gate[@]}" --real-worker
export ZGI_AGENT_RUNTIME_TEST_BACKEND=e2b
expect_failure "${gate[@]}" --real-worker
for skill in xlsx pptx slack-gif-creator; do
  mkdir -p "$test_root/skills/$skill"
  touch "$test_root/skills/$skill/SKILL.md"
done
"${gate[@]}" --real-worker
grep -F 'TestAgentExecutionStandardSkillMatrix' "$TEST_CALLS" >/dev/null
grep -F './internal/capabilities/agentexecution/runtime' "$TEST_CALLS" >/dev/null

expect_failure "${gate[@]}" --real-documents
for skill in docx pdf; do
  mkdir -p "$test_root/skills/$skill"
  touch "$test_root/skills/$skill/SKILL.md"
done
"${gate[@]}" --real-documents
grep -F 'TestAgentExecutionDocumentSkills' "$TEST_CALLS" >/dev/null

: >"$TEST_CALLS"
if MOCK_PNPM_FAIL=lint "${gate[@]}" --web; then echo "lint failure did not stop gate" >&2; exit 1; fi
[[ "$(wc -l <"$TEST_CALLS")" == 1 ]]
: >"$TEST_CALLS"
"${gate[@]}" --web
[[ "$(tail -1 "$TEST_CALLS")" == 'pnpm build' ]]
# Every command the --web gate lists runs, however many tests it gains.
web_commands="$(awk '/^  --web\)/,/;;/' "$repo_root/scripts/check-runtime-delivery.sh" | grep -c '^ *pnpm ')"
[[ "$web_commands" -gt 1 && "$(wc -l <"$TEST_CALLS")" == "$web_commands" ]]
echo 'runtime-delivery preflight and fail-fast tests passed'
