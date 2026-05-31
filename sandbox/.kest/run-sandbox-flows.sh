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

cleanup() {
  if [[ "${START_LOCAL_SANDBOX}" = "1" && -n "${SERVER_PID:-}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${DATA_DIR}" "${ARCHIVE_VARS}"
}
trap cleanup EXIT

if ! command -v "${KEST_BIN}" >/dev/null 2>&1; then
  echo "kest binary not found. Install kest or set KEST_BIN=/path/to/kest." >&2
  exit 127
fi

cd "${SANDBOX_DIR}"
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
strip_root_archive_base64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["strip_root_archive_base64"])' "${ARCHIVE_VARS}")"
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

"${KEST_BIN}" run .kest/sandbox-lifecycle-files-command.flow.md \
  --var base_url="${BASE_URL}" \
  --fail-fast

"${KEST_BIN}" run .kest/sandbox-short-code-contract.flow.md \
  --var base_url="${BASE_URL}" \
  --fail-fast

"${KEST_BIN}" run .kest/sandbox-template-runtime.flow.md \
  --var base_url="${BASE_URL}" \
  --var template_render="${template_render}" \
  --var template_missing="${template_missing}" \
  --var template_unsafe_helper="${template_unsafe_helper}" \
  --var template_builtin_helper="${template_builtin_helper}" \
  --var template_value="${template_value}" \
  --var template_output_value="${template_output_value}" \
  --var oversized_template_value="${oversized_template_value}" \
  --fail-fast

"${KEST_BIN}" run .kest/sandbox-execution-timeouts.flow.md \
  --var base_url="${BASE_URL}" \
  --fail-fast

"${KEST_BIN}" run .kest/sandbox-archive-skill-artifacts.flow.md \
  --var base_url="${BASE_URL}" \
  --var skill_archive_base64="${skill_archive_base64}" \
  --var valid_skill_manifest_archive_base64="${valid_skill_manifest_archive_base64}" \
  --var invalid_skill_manifest_archive_base64="${invalid_skill_manifest_archive_base64}" \
  --fail-fast

"${KEST_BIN}" run .kest/sandbox-archive-strip-root.flow.md \
  --var base_url="${BASE_URL}" \
  --var strip_root_archive_base64="${strip_root_archive_base64}" \
  --fail-fast

"${KEST_BIN}" run .kest/sandbox-security-limits.flow.md \
  --var base_url="${BASE_URL}" \
  --var zip_slip_archive_base64="${zip_slip_archive_base64}" \
  --var symlink_archive_base64="${symlink_archive_base64}" \
  --fail-fast

if [[ "${START_LOCAL_SANDBOX}" = "1" ]]; then
  "${KEST_BIN}" run .kest/sandbox-resource-limits.flow.md \
    --var base_url="${BASE_URL}" \
    --fail-fast
else
  echo "Skipping resource limit saturation flow against external sandbox: ${BASE_URL}"
fi
