#!/usr/bin/env bash
set -euo pipefail

SANDBOX_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${ZGI_SANDBOX_KEST_PORT:-$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)}"
START_LOCAL_SANDBOX=1
if [[ -n "${ZGI_SANDBOX_KEST_BASE_URL:-}" ]]; then
  BASE_URL="${ZGI_SANDBOX_KEST_BASE_URL}"
  START_LOCAL_SANDBOX=0
else
  BASE_URL="http://127.0.0.1:${PORT}"
fi
KEST_BIN="${KEST_BIN:-kest}"
DATA_DIR="$(mktemp -d /tmp/zgi-sandbox-kest.XXXXXX)"
ARCHIVE_VARS="$(mktemp /tmp/zgi-sandbox-kest-vars.XXXXXX)"
SERVER_LOG="${DATA_DIR}/sandbox.log"
KEST_HOME="${DATA_DIR}/kest-home"
KEST_CONFIG_HOME="${DATA_DIR}/kest-config"
FLOW_ROOT="${DATA_DIR}/flow-root"
FLOW_DIR="${FLOW_ROOT}/.kest"

cleanup() {
  if [[ "${START_LOCAL_SANDBOX}" = "1" && -n "${SERVER_PID:-}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${RATE_SERVER_PID:-}" ]] && kill -0 "${RATE_SERVER_PID}" 2>/dev/null; then
    kill "${RATE_SERVER_PID}" 2>/dev/null || true
    wait "${RATE_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${WORKSPACE_SERVER_PID:-}" ]] && kill -0 "${WORKSPACE_SERVER_PID}" 2>/dev/null; then
    kill "${WORKSPACE_SERVER_PID}" 2>/dev/null || true
    wait "${WORKSPACE_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${FILE_COUNT_SERVER_PID:-}" ]] && kill -0 "${FILE_COUNT_SERVER_PID}" 2>/dev/null; then
    kill "${FILE_COUNT_SERVER_PID}" 2>/dev/null || true
    wait "${FILE_COUNT_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${ARTIFACT_LIMIT_SERVER_PID:-}" ]] && kill -0 "${ARTIFACT_LIMIT_SERVER_PID}" 2>/dev/null; then
    kill "${ARTIFACT_LIMIT_SERVER_PID}" 2>/dev/null || true
    wait "${ARTIFACT_LIMIT_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${DEPENDENCY_PROFILE_LIMIT_SERVER_PID:-}" ]] && kill -0 "${DEPENDENCY_PROFILE_LIMIT_SERVER_PID}" 2>/dev/null; then
    kill "${DEPENDENCY_PROFILE_LIMIT_SERVER_PID}" 2>/dev/null || true
    wait "${DEPENDENCY_PROFILE_LIMIT_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${DEPENDENCY_PROFILE_BUILD_SERVER_PID:-}" ]] && kill -0 "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null; then
    kill "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null || true
    wait "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID:-}" ]] && kill -0 "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID}" 2>/dev/null; then
    kill "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID}" 2>/dev/null || true
    wait "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${CONCURRENT_SERVER_PID:-}" ]] && kill -0 "${CONCURRENT_SERVER_PID}" 2>/dev/null; then
    kill "${CONCURRENT_SERVER_PID}" 2>/dev/null || true
    wait "${CONCURRENT_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${QUEUE_TIMEOUT_SERVER_PID:-}" ]] && kill -0 "${QUEUE_TIMEOUT_SERVER_PID}" 2>/dev/null; then
    kill "${QUEUE_TIMEOUT_SERVER_PID}" 2>/dev/null || true
    wait "${QUEUE_TIMEOUT_SERVER_PID}" 2>/dev/null || true
  fi
  if [[ -n "${SHUTDOWN_DRAIN_SERVER_PID:-}" ]] && kill -0 "${SHUTDOWN_DRAIN_SERVER_PID}" 2>/dev/null; then
    kill "${SHUTDOWN_DRAIN_SERVER_PID}" 2>/dev/null || true
    wait "${SHUTDOWN_DRAIN_SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${DATA_DIR}" "${ARCHIVE_VARS}"
}
trap cleanup EXIT

if ! command -v "${KEST_BIN}" >/dev/null 2>&1; then
  echo "kest binary not found. Install kest or set KEST_BIN=/path/to/kest." >&2
  exit 127
fi

cd "${SANDBOX_DIR}"

./scripts/build-profile --profile skill-office --dry-run >/dev/null
./scripts/build-profile --profile stdlib --output-dir "${DATA_DIR}/profile-build" --force >/dev/null

mkdir -p "${FLOW_DIR}" "${KEST_HOME}" "${KEST_CONFIG_HOME}"
for flow in .kest/*.flow.md; do
  sed 's#{{base_url}}##g' "${flow}" >"${FLOW_DIR}/$(basename "${flow}")"
done
write_kest_config() {
  local base_url="$1"
  cat >"${FLOW_DIR}/config.yaml" <<EOF
version: 1
environments:
  local:
    base_url: ${base_url}
active_env: local
log_enabled: true
EOF
  cat >"${FLOW_DIR}/flow.config.yaml" <<EOF
version: 1
profiles:
  local:
    include: [".kest/*.flow.md"]
    env: local
    base_url: "${base_url}"
    strict: true
    fail_fast: false
    sync: false
EOF
}
write_kest_config "${BASE_URL}"

run_kest() {
  (
    cd "${FLOW_ROOT}"
    HOME="${KEST_HOME}" XDG_CONFIG_HOME="${KEST_CONFIG_HOME}" "${KEST_BIN}" run "$@"
  )
}

if [[ "${START_LOCAL_SANDBOX}" = "1" ]]; then
  env \
    ZGI_SANDBOX_SERVER_PORT="${PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-kest-${PORT}" \
    ZGI_SANDBOX_MAX_ACTIVE_PER_ORGANIZATION="2" \
    go run cmd/server/main.go >"${SERVER_LOG}" 2>&1 &
  SERVER_PID="$!"
fi

for _ in {1..80}; do
  if curl -fsS "${BASE_URL}/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done

if ! curl -fsS "${BASE_URL}/health" >/dev/null 2>&1; then
  if [[ "${START_LOCAL_SANDBOX}" = "1" ]]; then
    cat "${SERVER_LOG}" >&2
  fi
  echo "sandbox did not become ready at ${BASE_URL}" >&2
  exit 1
fi

python3 - "${ARCHIVE_VARS}" <<'PY'
import base64
import io
import json
import stat
import sys
import zipfile

out = sys.argv[1]

def zip_b64(entries):
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        for name, content in entries:
            zf.writestr(name, content)
    return base64.b64encode(buf.getvalue()).decode()

def symlink_zip_b64():
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("ok.txt", "changed")
        info = zipfile.ZipInfo("link.txt")
        info.create_system = 3
        info.external_attr = (stat.S_IFLNK | 0o777) << 16
        zf.writestr(info, "../outside.txt")
    return base64.b64encode(buf.getvalue()).decode()

values = {
    "skill_archive_base64": zip_b64([
        ("SKILL.md", "---\nname: kest-skill\ndescription: Kest skill\nruntime_type: prompt\n---\nUse scripts/run.py to echo input.\n"),
        ("references/schema.md", "Input JSON contains input.\n"),
        ("scripts/run.py", "import json, os, sys\nargs = json.loads(sys.stdin.read() or '{}')\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/report.txt', 'w').write('kest artifact\\n')\nprint(json.dumps({'echo': args.get('input', ''), 'ok': True}))\n"),
    ]),
    "valid_skill_manifest_archive_base64": zip_b64([
        ("SKILL.md", "---\nname: manifest-skill\ndescription: Manifest skill\nruntime_type: prompt\n---\n"),
        ("scripts/run.py", "import json, os, sys\npayload = json.loads(sys.stdin.read() or '{}')\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/manifest-report.txt', 'w').write('manifest artifact\\n')\nprint(json.dumps({'input': payload.get('input'), 'ok': True}))\n"),
        ("references/schema.md", "schema\n"),
        ("skill.manifest.json", json.dumps({
            "entrypoint": "scripts/run.py",
            "language": "python3",
            "dependency_profile": "stdlib",
            "timeout_ms": 30000,
            "allowed_artifact_paths": ["artifacts"],
            "max_artifact_count": 10,
            "max_artifact_bytes": 32768,
            "result_mode": "mixed",
        })),
    ]),
    "mismatched_skill_manifest_archive_base64": zip_b64([
        ("SKILL.md", "---\nname: mismatched-manifest-skill\ndescription: Mismatched manifest skill\nruntime_type: prompt\n---\n"),
        ("scripts/run.py", "print('ok')\n"),
        ("skill.manifest.json", json.dumps({
            "entrypoint": "scripts/run.py",
            "language": "python3",
            "dependency_profile": "workflow-safe",
            "timeout_ms": 30000,
            "allowed_artifact_paths": ["artifacts"],
            "max_artifact_count": 10,
            "max_artifact_bytes": 32768,
            "result_mode": "mixed",
        })),
    ]),
    "invalid_skill_manifest_archive_base64": zip_b64([
        ("SKILL.md", "---\nname: invalid-manifest-skill\ndescription: Invalid manifest skill\nruntime_type: prompt\n---\n"),
        ("scripts/run.py", "print('ok')\n"),
        ("skill.manifest.json", json.dumps({
            "entrypoint": "scripts/missing.py",
            "language": "python3",
            "timeout_ms": 30000,
            "allowed_artifact_paths": ["artifacts"],
            "max_artifact_count": 10,
            "max_artifact_bytes": 32768,
            "result_mode": "mixed",
        })),
    ]),
    "strip_root_archive_base64": zip_b64([
        ("weather/SKILL.md", "# Weather\n"),
        ("weather/references/schema.md", "schema\n"),
        ("weather/scripts/run.py", "print('ok')\n"),
    ]),
    "dependency_prepare_archive_base64": zip_b64([
        ("SKILL.md", "---\nname: dependency-prepare-skill\ndescription: Dependency prepare skill\nruntime_type: prompt\n---\n"),
        ("skill.manifest.json", json.dumps({
            "entrypoint": "scripts/run.py",
            "language": "python3",
            "dependencies": {
                "python": ["pydantic==2.7.4"],
            },
        })),
        ("requirements.txt", "pandas==2.2.3\n-r nested.txt\n# ignored\n"),
        ("package.json", json.dumps({
            "dependencies": {
                "pdf-lib": "1.17.1",
                "local-only": "file:../local",
            },
        })),
        ("scripts/run.py", "import json\nfrom PIL import Image\nprint(json.dumps({'ok': True}))\n"),
        ("scripts/run.js", "import tool from '@org/tool/path';\n"),
    ]),
    "zip_slip_archive_base64": zip_b64([
        ("../escape.txt", "nope"),
    ]),
    "symlink_archive_base64": symlink_zip_b64(),
}

with open(out, "w", encoding="utf-8") as f:
    json.dump(values, f)
PY

skill_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["skill_archive_base64"])' "${ARCHIVE_VARS}")"
valid_skill_manifest_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["valid_skill_manifest_archive_base64"])' "${ARCHIVE_VARS}")"
invalid_skill_manifest_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["invalid_skill_manifest_archive_base64"])' "${ARCHIVE_VARS}")"
mismatched_skill_manifest_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["mismatched_skill_manifest_archive_base64"])' "${ARCHIVE_VARS}")"
strip_root_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["strip_root_archive_base64"])' "${ARCHIVE_VARS}")"
dependency_prepare_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["dependency_prepare_archive_base64"])' "${ARCHIVE_VARS}")"
zip_slip_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["zip_slip_archive_base64"])' "${ARCHIVE_VARS}")"
symlink_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["symlink_archive_base64"])' "${ARCHIVE_VARS}")"
template_render="Hello {{ upper .name }}"
template_missing="Hello {{ .missing }}"
template_unsafe_helper="{{ env .name }}"
template_builtin_helper="{{ len .name }}"
template_value="{{ .value }}"
template_output_value="$(python3 - <<'PY'
print("x" * 2048)
PY
)"
oversized_template_value="$(python3 - <<'PY'
print("x" * 17408)
PY
)"

cd "${SANDBOX_DIR}"

run_kest .kest/sandbox-lifecycle-files-command.flow.md \
  --fail-fast

run_kest .kest/sandbox-ttl-limits.flow.md \
  --fail-fast

run_kest .kest/sandbox-dependency-profile-catalog.flow.md \
  --fail-fast

run_kest .kest/sandbox-dependency-prepare.flow.md \
  --var dependency_prepare_archive_base64="${dependency_prepare_archive_base64}" \
  --fail-fast

if [[ "${START_LOCAL_SANDBOX}" = "1" ]]; then
  DEPENDENCY_PROFILE_BUILD_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  DEPENDENCY_PROFILE_BUILD_BASE_URL="http://127.0.0.1:${DEPENDENCY_PROFILE_BUILD_PORT}"
  DEPENDENCY_PROFILE_BUILD_SERVER_LOG="${DATA_DIR}/sandbox-dependency-profile-build.log"
  DEPENDENCY_PROFILE_BUILD_API_KEY="kest-admin-key"
  DEPENDENCY_PROFILE_BUILD_NAME="office-safe-${DEPENDENCY_PROFILE_BUILD_PORT}-${RANDOM}"
  env \
    ZGI_SANDBOX_SERVER_PORT="${DEPENDENCY_PROFILE_BUILD_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/dependency-profile-build-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-dependency-profile-build-kest-${DEPENDENCY_PROFILE_BUILD_PORT}" \
    ZGI_SANDBOX_API_KEY="${DEPENDENCY_PROFILE_BUILD_API_KEY}" \
    go run cmd/server/main.go >"${DEPENDENCY_PROFILE_BUILD_SERVER_LOG}" 2>&1 &
  DEPENDENCY_PROFILE_BUILD_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${DEPENDENCY_PROFILE_BUILD_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${DEPENDENCY_PROFILE_BUILD_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${DEPENDENCY_PROFILE_BUILD_SERVER_LOG}" >&2
    echo "dependency profile build sandbox did not become ready at ${DEPENDENCY_PROFILE_BUILD_BASE_URL}" >&2
    exit 1
  fi

  write_kest_config "${DEPENDENCY_PROFILE_BUILD_BASE_URL}"
  run_kest .kest/sandbox-dependency-profile-build.flow.md \
    --var admin_api_key="${DEPENDENCY_PROFILE_BUILD_API_KEY}" \
    --var dependency_profile_name="${DEPENDENCY_PROFILE_BUILD_NAME}" \
    --fail-fast

  if kill -0 "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null; then
    kill "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null || true
    wait "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null || true
  fi

  DEPENDENCY_PROFILE_CACHE_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  DEPENDENCY_PROFILE_BUILD_BASE_URL="http://127.0.0.1:${DEPENDENCY_PROFILE_CACHE_PORT}"
  DEPENDENCY_PROFILE_BUILD_SERVER_LOG="${DATA_DIR}/sandbox-dependency-profile-cache.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${DEPENDENCY_PROFILE_CACHE_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/dependency-profile-build-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-dependency-profile-cache-kest-${DEPENDENCY_PROFILE_CACHE_PORT}" \
    ZGI_SANDBOX_API_KEY="${DEPENDENCY_PROFILE_BUILD_API_KEY}" \
    go run cmd/server/main.go >"${DEPENDENCY_PROFILE_BUILD_SERVER_LOG}" 2>&1 &
  DEPENDENCY_PROFILE_BUILD_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${DEPENDENCY_PROFILE_BUILD_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${DEPENDENCY_PROFILE_BUILD_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${DEPENDENCY_PROFILE_BUILD_SERVER_LOG}" >&2
    echo "dependency profile cache sandbox did not become ready at ${DEPENDENCY_PROFILE_BUILD_BASE_URL}" >&2
    exit 1
  fi

  write_kest_config "${DEPENDENCY_PROFILE_BUILD_BASE_URL}"
  run_kest .kest/sandbox-dependency-profile-cache.flow.md \
    --var admin_api_key="${DEPENDENCY_PROFILE_BUILD_API_KEY}" \
    --var dependency_profile_name="${DEPENDENCY_PROFILE_BUILD_NAME}" \
    --fail-fast

  if kill -0 "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null; then
    kill "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null || true
    wait "${DEPENDENCY_PROFILE_BUILD_SERVER_PID}" 2>/dev/null || true
  fi

  DEPENDENCY_PROFILE_ARTIFACT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  DEPENDENCY_PROFILE_ARTIFACT_BASE_URL="http://127.0.0.1:${DEPENDENCY_PROFILE_ARTIFACT_PORT}"
  DEPENDENCY_PROFILE_ARTIFACT_SERVER_LOG="${DATA_DIR}/sandbox-dependency-profile-artifact.log"
  DEPENDENCY_PROFILE_ARTIFACT_ROOT="${DATA_DIR}/dependency-profile-artifacts"
  DEPENDENCY_PROFILE_ARTIFACT_API_KEY="kest-artifact-admin-key"
  DEPENDENCY_PROFILE_ARTIFACT_CHECKSUM="$(
    DEPENDENCY_PROFILE_ARTIFACT_ROOT="${DEPENDENCY_PROFILE_ARTIFACT_ROOT}" python3 - <<'PY'
import hashlib
import json
import os
from pathlib import Path

root = Path(os.environ["DEPENDENCY_PROFILE_ARTIFACT_ROOT"])
profile_dir = root / "skill-office" / "opt" / "zgi" / "profiles" / "skill-office"
files = {
    "venv/bin/python": b"#!/usr/bin/env python3\n",
    "venv/pyvenv.cfg": b"home = /usr/bin\ninclude-system-site-packages = false\n",
    "node_modules/.bin/tool": b"#!/usr/bin/env node\n",
    "node_modules/office-tools/package.json": b'{"name":"office-tools","version":"managed"}\n',
    "bin/profile-ready": b"ready\n",
}

for rel, raw in files.items():
    path = profile_dir / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(raw)

digest = hashlib.sha256()
size = 0
for rel in sorted(files):
    raw = files[rel]
    digest.update(rel.encode("utf-8"))
    digest.update(b"\0")
    digest.update(raw)
    digest.update(b"\0")
    size += len(raw)
checksum = "sha256:" + digest.hexdigest()
manifest = {
    "name": "skill-office",
    "version": "2026.05.31-artifact",
    "status": "disabled",
    "enabled": False,
    "owner_scope": "global",
    "languages": ["python3", "nodejs"],
    "base_runtime": "linux-secure",
    "description": "Managed office automation profile artifact.",
    "packages": [
        {"ecosystem": "python3", "name": "office-tools", "version": "managed"},
        {"ecosystem": "nodejs", "name": "office-tools", "version": "managed"},
    ],
    "build": {
        "checksum": checksum,
        "size_bytes": size,
        "verification_passed": True,
    },
}
(profile_dir / "manifest.json").write_text(json.dumps(manifest, sort_keys=True), encoding="utf-8")
print(checksum)
PY
  )"
  env \
    ZGI_SANDBOX_SERVER_PORT="${DEPENDENCY_PROFILE_ARTIFACT_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/dependency-profile-artifact-data" \
    ZGI_SANDBOX_DEPENDENCY_ROOTFS_DIR="${DEPENDENCY_PROFILE_ARTIFACT_ROOT}" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-dependency-profile-artifact-kest-${DEPENDENCY_PROFILE_ARTIFACT_PORT}" \
    ZGI_SANDBOX_API_KEY="${DEPENDENCY_PROFILE_ARTIFACT_API_KEY}" \
    go run cmd/server/main.go >"${DEPENDENCY_PROFILE_ARTIFACT_SERVER_LOG}" 2>&1 &
  DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${DEPENDENCY_PROFILE_ARTIFACT_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${DEPENDENCY_PROFILE_ARTIFACT_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_LOG}" >&2
    echo "dependency profile artifact sandbox did not become ready at ${DEPENDENCY_PROFILE_ARTIFACT_BASE_URL}" >&2
    exit 1
  fi

  write_kest_config "${DEPENDENCY_PROFILE_ARTIFACT_BASE_URL}"
  run_kest .kest/sandbox-dependency-profile-artifact-autoload.flow.md \
    --var admin_api_key="${DEPENDENCY_PROFILE_ARTIFACT_API_KEY}" \
    --var skill_office_checksum="${DEPENDENCY_PROFILE_ARTIFACT_CHECKSUM}" \
    --fail-fast

  if kill -0 "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID}" 2>/dev/null; then
    kill "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID}" 2>/dev/null || true
    wait "${DEPENDENCY_PROFILE_ARTIFACT_SERVER_PID}" 2>/dev/null || true
  fi
  write_kest_config "${BASE_URL}"
fi

run_kest .kest/sandbox-organization-access-scope.flow.md \
  --fail-fast

run_kest .kest/sandbox-policy-deny-audit.flow.md \
  --fail-fast

run_kest .kest/sandbox-egress-policy-decision.flow.md \
  --fail-fast

run_kest .kest/sandbox-short-code-contract.flow.md \
  --fail-fast

run_kest .kest/sandbox-template-runtime.flow.md \
  --var template_render="${template_render}" \
  --var template_missing="${template_missing}" \
  --var template_unsafe_helper="${template_unsafe_helper}" \
  --var template_builtin_helper="${template_builtin_helper}" \
  --var template_value="${template_value}" \
  --var template_output_value="${template_output_value}" \
  --var oversized_template_value="${oversized_template_value}" \
  --fail-fast

run_kest .kest/sandbox-execution-timeouts.flow.md \
  --fail-fast

if [[ "${START_LOCAL_SANDBOX}" = "1" ]]; then
  CANCELLATION_REQUEST_ID="req_kest_canceled_command"
  CANCELLATION_SANDBOX_ID="$(curl -fsS \
    -H "Content-Type: application/json" \
    -d '{"runtime_profile":"session","ttl_seconds":60,"dependency_profile":"stdlib","network_enabled":false,"network_policy":"deny-by-default"}' \
    "${BASE_URL}/v1/sandboxes" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')"

  set +e
  curl -fsS --max-time 0.2 \
    -H "Content-Type: application/json" \
    -H "X-Request-ID: ${CANCELLATION_REQUEST_ID}" \
    -d "{\"sandbox_id\":\"${CANCELLATION_SANDBOX_ID}\",\"command\":\"python3\",\"args\":[\"-c\",\"import time; time.sleep(5)\"],\"profile\":\"code-short\",\"timeout_ms\":10000}" \
    "${BASE_URL}/v1/exec/command" >/dev/null
  CANCELLATION_CURL_STATUS="$?"
  set -e
  if [[ "${CANCELLATION_CURL_STATUS}" -eq 0 ]]; then
    echo "cancellation request unexpectedly completed before client timeout" >&2
    exit 1
  fi

  for _ in {1..120}; do
    active_workers="$(curl -fsS "${BASE_URL}/v1/metrics" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["runner"]["active_workers"])')"
    queued_executions="$(curl -fsS "${BASE_URL}/v1/metrics" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["runner"]["queued_executions"])')"
    cancellation_events="$(curl -fsS "${BASE_URL}/v1/observer/events?sandbox_id=${CANCELLATION_SANDBOX_ID}&type=exec.command.failed&request_id=${CANCELLATION_REQUEST_ID}&limit=1" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["data"]["events"]))')"
    if [[ "${active_workers}" = "0" && "${queued_executions}" = "0" && "${cancellation_events}" = "1" ]]; then
      break
    fi
    sleep 0.05
  done

  run_kest .kest/sandbox-execution-cancellation.flow.md \
    --var cancellation_sandbox_id="${CANCELLATION_SANDBOX_ID}" \
    --var cancellation_request_id="${CANCELLATION_REQUEST_ID}" \
    --fail-fast
else
  echo "Skipping cancellation cleanup flow against external sandbox: ${BASE_URL}"
fi

run_kest .kest/sandbox-archive-skill-artifacts.flow.md \
  --var skill_archive_base64="${skill_archive_base64}" \
  --var valid_skill_manifest_archive_base64="${valid_skill_manifest_archive_base64}" \
  --var invalid_skill_manifest_archive_base64="${invalid_skill_manifest_archive_base64}" \
  --var mismatched_skill_manifest_archive_base64="${mismatched_skill_manifest_archive_base64}" \
  --fail-fast

run_kest .kest/sandbox-archive-strip-root.flow.md \
  --var strip_root_archive_base64="${strip_root_archive_base64}" \
  --fail-fast

run_kest .kest/sandbox-security-limits.flow.md \
  --var zip_slip_archive_base64="${zip_slip_archive_base64}" \
  --var symlink_archive_base64="${symlink_archive_base64}" \
  --fail-fast

if [[ "${START_LOCAL_SANDBOX}" = "1" ]]; then
  run_kest .kest/sandbox-resource-limits.flow.md \
    --fail-fast

  RATE_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  RATE_BASE_URL="http://127.0.0.1:${RATE_PORT}"
  RATE_SERVER_LOG="${DATA_DIR}/sandbox-rate.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${RATE_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/rate-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-rate-kest-${RATE_PORT}" \
    ZGI_SANDBOX_MAX_EXECUTIONS_PER_MINUTE_PER_ORGANIZATION="1" \
    ZGI_SANDBOX_MAX_NETWORK_REQUESTS_PER_MINUTE_PER_ORGANIZATION="1" \
    go run cmd/server/main.go >"${RATE_SERVER_LOG}" 2>&1 &
  RATE_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${RATE_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${RATE_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${RATE_SERVER_LOG}" >&2
    echo "rate-limit sandbox did not become ready at ${RATE_BASE_URL}" >&2
    exit 1
  fi

  local_config="${FLOW_DIR}/config.yaml"
  local_flow_config="${FLOW_DIR}/flow.config.yaml"
  cat >"${local_config}" <<EOF
version: 1
environments:
  local:
    base_url: ${RATE_BASE_URL}
active_env: local
log_enabled: true
EOF
  cat >"${local_flow_config}" <<EOF
version: 1
profiles:
  local:
    include: [".kest/*.flow.md"]
    env: local
    base_url: "${RATE_BASE_URL}"
    strict: true
    fail_fast: false
    sync: false
EOF
  run_kest .kest/sandbox-organization-execution-rate.flow.md \
    --var rate_organization_id="organization_rate_kest_${RATE_PORT}" \
    --fail-fast

  WORKSPACE_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  WORKSPACE_BASE_URL="http://127.0.0.1:${WORKSPACE_PORT}"
  WORKSPACE_SERVER_LOG="${DATA_DIR}/sandbox-workspace.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${WORKSPACE_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/workspace-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-workspace-kest-${WORKSPACE_PORT}" \
    ZGI_SANDBOX_MAX_WORKSPACE_BYTES="16" \
    ZGI_SANDBOX_MAX_WORKSPACE_BYTES_PER_ORGANIZATION="16" \
    go run cmd/server/main.go >"${WORKSPACE_SERVER_LOG}" 2>&1 &
  WORKSPACE_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${WORKSPACE_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${WORKSPACE_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${WORKSPACE_SERVER_LOG}" >&2
    echo "workspace-limit sandbox did not become ready at ${WORKSPACE_BASE_URL}" >&2
    exit 1
  fi

  cat >"${local_config}" <<EOF
version: 1
environments:
  local:
    base_url: ${WORKSPACE_BASE_URL}
active_env: local
log_enabled: true
EOF
  cat >"${local_flow_config}" <<EOF
version: 1
profiles:
  local:
    include: [".kest/*.flow.md"]
    env: local
    base_url: "${WORKSPACE_BASE_URL}"
    strict: true
    fail_fast: false
    sync: false
EOF
  run_kest .kest/sandbox-workspace-byte-limit.flow.md \
    --var workspace_organization_id="organization_workspace_kest_${WORKSPACE_PORT}" \
    --fail-fast

  FILE_COUNT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  FILE_COUNT_BASE_URL="http://127.0.0.1:${FILE_COUNT_PORT}"
  FILE_COUNT_SERVER_LOG="${DATA_DIR}/sandbox-file-count.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${FILE_COUNT_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/file-count-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-file-count-kest-${FILE_COUNT_PORT}" \
    ZGI_SANDBOX_MAX_WORKSPACE_FILES="1" \
    go run cmd/server/main.go >"${FILE_COUNT_SERVER_LOG}" 2>&1 &
  FILE_COUNT_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${FILE_COUNT_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${FILE_COUNT_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${FILE_COUNT_SERVER_LOG}" >&2
    echo "workspace-file-limit sandbox did not become ready at ${FILE_COUNT_BASE_URL}" >&2
    exit 1
  fi

  cat >"${local_config}" <<EOF
version: 1
environments:
  local:
    base_url: ${FILE_COUNT_BASE_URL}
active_env: local
log_enabled: true
EOF
  cat >"${local_flow_config}" <<EOF
version: 1
profiles:
  local:
    include: [".kest/*.flow.md"]
    env: local
    base_url: "${FILE_COUNT_BASE_URL}"
    strict: true
    fail_fast: false
    sync: false
EOF
  run_kest .kest/sandbox-workspace-file-limit.flow.md \
    --var file_count_organization_id="organization_file_count_kest_${FILE_COUNT_PORT}" \
    --fail-fast

  ARTIFACT_LIMIT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  ARTIFACT_LIMIT_BASE_URL="http://127.0.0.1:${ARTIFACT_LIMIT_PORT}"
  ARTIFACT_LIMIT_SERVER_LOG="${DATA_DIR}/sandbox-artifact-limit.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${ARTIFACT_LIMIT_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/artifact-limit-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-artifact-limit-kest-${ARTIFACT_LIMIT_PORT}" \
    ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_FILES="10" \
    ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_BYTES="8" \
    ZGI_SANDBOX_MAX_ARTIFACT_BYTES_PER_ORGANIZATION="10" \
    go run cmd/server/main.go >"${ARTIFACT_LIMIT_SERVER_LOG}" 2>&1 &
  ARTIFACT_LIMIT_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${ARTIFACT_LIMIT_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${ARTIFACT_LIMIT_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${ARTIFACT_LIMIT_SERVER_LOG}" >&2
    echo "artifact-manifest-limit sandbox did not become ready at ${ARTIFACT_LIMIT_BASE_URL}" >&2
    exit 1
  fi

  cat >"${local_config}" <<EOF
version: 1
environments:
  local:
    base_url: ${ARTIFACT_LIMIT_BASE_URL}
active_env: local
log_enabled: true
EOF
  cat >"${local_flow_config}" <<EOF
version: 1
profiles:
  local:
    include: [".kest/*.flow.md"]
    env: local
    base_url: "${ARTIFACT_LIMIT_BASE_URL}"
    strict: true
    fail_fast: false
    sync: false
EOF
  run_kest .kest/sandbox-artifact-manifest-config-limit.flow.md \
    --var artifact_limit_organization_id="organization_artifact_limit_kest_${ARTIFACT_LIMIT_PORT}" \
    --fail-fast

  DEPENDENCY_PROFILE_LIMIT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  DEPENDENCY_PROFILE_LIMIT_BASE_URL="http://127.0.0.1:${DEPENDENCY_PROFILE_LIMIT_PORT}"
  DEPENDENCY_PROFILE_LIMIT_SERVER_LOG="${DATA_DIR}/sandbox-dependency-profile-limit.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${DEPENDENCY_PROFILE_LIMIT_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/dependency-profile-limit-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-dependency-profile-limit-kest-${DEPENDENCY_PROFILE_LIMIT_PORT}" \
    ZGI_SANDBOX_MAX_DEPENDENCY_PROFILES_PER_ORGANIZATION="1" \
    go run cmd/server/main.go >"${DEPENDENCY_PROFILE_LIMIT_SERVER_LOG}" 2>&1 &
  DEPENDENCY_PROFILE_LIMIT_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${DEPENDENCY_PROFILE_LIMIT_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${DEPENDENCY_PROFILE_LIMIT_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${DEPENDENCY_PROFILE_LIMIT_SERVER_LOG}" >&2
    echo "dependency-profile-limit sandbox did not become ready at ${DEPENDENCY_PROFILE_LIMIT_BASE_URL}" >&2
    exit 1
  fi

  cat >"${local_config}" <<EOF
version: 1
environments:
  local:
    base_url: ${DEPENDENCY_PROFILE_LIMIT_BASE_URL}
active_env: local
log_enabled: true
EOF
  cat >"${local_flow_config}" <<EOF
version: 1
profiles:
  local:
    include: [".kest/*.flow.md"]
    env: local
    base_url: "${DEPENDENCY_PROFILE_LIMIT_BASE_URL}"
    strict: true
    fail_fast: false
    sync: false
EOF
  run_kest .kest/sandbox-organization-dependency-profile-limit.flow.md \
    --var dependency_profile_limit_organization_id="organization_dependency_profile_limit_kest_${DEPENDENCY_PROFILE_LIMIT_PORT}" \
    --fail-fast

  CONCURRENT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  CONCURRENT_BASE_URL="http://127.0.0.1:${CONCURRENT_PORT}"
  CONCURRENT_SERVER_LOG="${DATA_DIR}/sandbox-concurrent-execution.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${CONCURRENT_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/concurrent-execution-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-concurrent-kest-${CONCURRENT_PORT}" \
    ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS="1" \
    ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_PROFILE="1" \
    ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_ORGANIZATION="1" \
    ZGI_SANDBOX_MAX_QUEUED_EXECUTIONS_PER_ORGANIZATION="1" \
    go run cmd/server/main.go >"${CONCURRENT_SERVER_LOG}" 2>&1 &
  CONCURRENT_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${CONCURRENT_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${CONCURRENT_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${CONCURRENT_SERVER_LOG}" >&2
    echo "organization-concurrent-execution-limit sandbox did not become ready at ${CONCURRENT_BASE_URL}" >&2
    exit 1
  fi

  cat >"${local_config}" <<EOF
version: 1
environments:
  local:
    base_url: ${CONCURRENT_BASE_URL}
active_env: local
log_enabled: true
EOF
  cat >"${local_flow_config}" <<EOF
version: 1
profiles:
  local:
    include: [".kest/*.flow.md"]
    env: local
    base_url: "${CONCURRENT_BASE_URL}"
    strict: true
    fail_fast: false
    sync: false
EOF
  run_kest .kest/sandbox-organization-concurrent-execution-limit.flow.md \
    --var concurrent_organization_id="organization_concurrent_kest_${CONCURRENT_PORT}" \
    --fail-fast
  run_kest .kest/sandbox-service-concurrent-execution-limit.flow.md \
    --var service_organization_id="organization_service_kest_${CONCURRENT_PORT}" \
    --fail-fast
  run_kest .kest/sandbox-organization-queued-execution-limit.flow.md \
    --var queued_organization_id="organization_queued_kest_${CONCURRENT_PORT}" \
    --fail-fast
  run_kest .kest/sandbox-profile-concurrent-execution-limit.flow.md \
    --var profile_organization_id="organization_profile_kest_${CONCURRENT_PORT}" \
    --fail-fast

  QUEUE_TIMEOUT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  QUEUE_TIMEOUT_BASE_URL="http://127.0.0.1:${QUEUE_TIMEOUT_PORT}"
  QUEUE_TIMEOUT_SERVER_LOG="${DATA_DIR}/sandbox-queue-timeout.log"
  env \
    ZGI_SANDBOX_SERVER_PORT="${QUEUE_TIMEOUT_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/queue-timeout-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-queue-timeout-kest-${QUEUE_TIMEOUT_PORT}" \
    ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_ORGANIZATION="1" \
    ZGI_SANDBOX_MAX_QUEUED_EXECUTIONS_PER_ORGANIZATION="1" \
    ZGI_SANDBOX_QUEUE_TIMEOUT_MS="100" \
    go run cmd/server/main.go >"${QUEUE_TIMEOUT_SERVER_LOG}" 2>&1 &
  QUEUE_TIMEOUT_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${QUEUE_TIMEOUT_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${QUEUE_TIMEOUT_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${QUEUE_TIMEOUT_SERVER_LOG}" >&2
    echo "queue-timeout sandbox did not become ready at ${QUEUE_TIMEOUT_BASE_URL}" >&2
    exit 1
  fi

  write_kest_config "${QUEUE_TIMEOUT_BASE_URL}"
  QUEUE_TIMEOUT_ORGANIZATION_ID="organization_queue_timeout_kest_${QUEUE_TIMEOUT_PORT}"
  QUEUE_TIMEOUT_SANDBOX_ID="$(curl -fsS \
    -H "Content-Type: application/json" \
    -d "{\"runtime_profile\":\"session\",\"ttl_seconds\":60,\"organization_id\":\"${QUEUE_TIMEOUT_ORGANIZATION_ID}\"}" \
    "${QUEUE_TIMEOUT_BASE_URL}/v1/sandboxes" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')"
  QUEUE_TIMEOUT_HOLDER_RESPONSE="${DATA_DIR}/queue-timeout-holder-response.json"
  curl -fsS \
    -H "Content-Type: application/json" \
    -H "X-Request-ID: req_kest_queue_timeout_holder" \
    -d "{\"sandbox_id\":\"${QUEUE_TIMEOUT_SANDBOX_ID}\",\"command\":\"python3\",\"args\":[\"-c\",\"import time; time.sleep(2); print('holder')\"],\"profile\":\"code-short\",\"timeout_ms\":5000}" \
    "${QUEUE_TIMEOUT_BASE_URL}/v1/exec/command" >"${QUEUE_TIMEOUT_HOLDER_RESPONSE}" &
  QUEUE_TIMEOUT_HOLDER_PID="$!"

  for _ in {1..80}; do
    active_workers="$(curl -fsS "${QUEUE_TIMEOUT_BASE_URL}/v1/metrics" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["runner"]["active_workers"])')"
    if [[ "${active_workers}" != "0" ]]; then
      break
    fi
    sleep 0.025
  done
  if [[ "$(curl -fsS "${QUEUE_TIMEOUT_BASE_URL}/v1/metrics" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["runner"]["active_workers"])')" = "0" ]]; then
    echo "queue-timeout holder did not occupy a worker" >&2
    exit 1
  fi

  run_kest .kest/sandbox-execution-queue-timeout.flow.md \
    --var queue_timeout_sandbox_id="${QUEUE_TIMEOUT_SANDBOX_ID}" \
    --var queue_timeout_organization_id="${QUEUE_TIMEOUT_ORGANIZATION_ID}" \
    --fail-fast

  wait "${QUEUE_TIMEOUT_HOLDER_PID}"
  curl -fsS -X DELETE "${QUEUE_TIMEOUT_BASE_URL}/v1/sandboxes/${QUEUE_TIMEOUT_SANDBOX_ID}" >/dev/null
  write_kest_config "${CONCURRENT_BASE_URL}"
else
  echo "Skipping resource limit saturation flow against external sandbox: ${BASE_URL}"
fi

if [[ "${START_LOCAL_SANDBOX}" = "1" ]]; then
  SHUTDOWN_DRAIN_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    print(s.getsockname()[1])
PY
)"
  SHUTDOWN_DRAIN_BASE_URL="http://127.0.0.1:${SHUTDOWN_DRAIN_PORT}"
  SHUTDOWN_DRAIN_SERVER_LOG="${DATA_DIR}/sandbox-shutdown-drain.log"
  SHUTDOWN_DRAIN_BINARY="${DATA_DIR}/zgi-sandbox-server"
  go build -o "${SHUTDOWN_DRAIN_BINARY}" ./cmd/server
  env \
    ZGI_SANDBOX_SERVER_PORT="${SHUTDOWN_DRAIN_PORT}" \
    ZGI_SANDBOX_DATA_DIR="${DATA_DIR}/shutdown-drain-data" \
    ZGI_SANDBOX_WORKER_ID="zgi-sandbox-shutdown-drain-kest-${SHUTDOWN_DRAIN_PORT}" \
    "${SHUTDOWN_DRAIN_BINARY}" >"${SHUTDOWN_DRAIN_SERVER_LOG}" 2>&1 &
  SHUTDOWN_DRAIN_SERVER_PID="$!"

  for _ in {1..80}; do
    if curl -fsS "${SHUTDOWN_DRAIN_BASE_URL}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -fsS "${SHUTDOWN_DRAIN_BASE_URL}/health" >/dev/null 2>&1; then
    cat "${SHUTDOWN_DRAIN_SERVER_LOG}" >&2
    echo "shutdown-drain sandbox did not become ready at ${SHUTDOWN_DRAIN_BASE_URL}" >&2
    exit 1
  fi

  SHUTDOWN_DRAIN_SANDBOX_ID="$(curl -fsS \
    -H "Content-Type: application/json" \
    -d '{"runtime_profile":"session","ttl_seconds":60,"dependency_profile":"stdlib","network_enabled":false,"network_policy":"deny-by-default"}' \
    "${SHUTDOWN_DRAIN_BASE_URL}/v1/sandboxes" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')"
  SHUTDOWN_DRAIN_RESPONSE="${DATA_DIR}/shutdown-drain-response.json"
  curl -fsS \
    -H "Content-Type: application/json" \
    -H "X-Request-ID: req_kest_shutdown_drain" \
    -d "{\"sandbox_id\":\"${SHUTDOWN_DRAIN_SANDBOX_ID}\",\"command\":\"python3\",\"args\":[\"-c\",\"import time; time.sleep(0.5); print('shutdown-drain-ok')\"],\"profile\":\"code-short\",\"timeout_ms\":5000}" \
    "${SHUTDOWN_DRAIN_BASE_URL}/v1/exec/command" >"${SHUTDOWN_DRAIN_RESPONSE}" &
  SHUTDOWN_DRAIN_CURL_PID="$!"

  for _ in {1..80}; do
    active_workers="$(curl -fsS "${SHUTDOWN_DRAIN_BASE_URL}/v1/metrics" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["runner"]["active_workers"])')"
    if [[ "${active_workers}" != "0" ]]; then
      break
    fi
    sleep 0.025
  done
  if [[ "$(curl -fsS "${SHUTDOWN_DRAIN_BASE_URL}/v1/metrics" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["runner"]["active_workers"])')" = "0" ]]; then
    echo "shutdown-drain command did not occupy a worker" >&2
    exit 1
  fi

  kill -TERM "${SHUTDOWN_DRAIN_SERVER_PID}"
  wait "${SHUTDOWN_DRAIN_CURL_PID}"
  python3 - "${SHUTDOWN_DRAIN_RESPONSE}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    payload = json.load(f)
data = payload["data"]
if payload["code"] != 0 or data["exit_code"] != 0 or data["stdout"] != "shutdown-drain-ok\n":
    raise SystemExit(f"unexpected shutdown drain response: {payload}")
PY
  wait "${SHUTDOWN_DRAIN_SERVER_PID}"
  SHUTDOWN_DRAIN_SERVER_PID=""
else
  echo "Skipping shutdown drain flow against external sandbox: ${BASE_URL}"
fi
