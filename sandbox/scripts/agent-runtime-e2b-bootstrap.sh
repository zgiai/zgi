#!/bin/sh
set -eu

token_file="${ZGI_RUNTIME_AGENT_TOKEN_FILE:-/run/zgi-agent/bootstrap-token}"
while [ ! -s "$token_file" ]; do
  sleep 0.05
done

# E2B template environment can shadow create-time overrides. Read the new
# admission bit from the authenticated bootstrap payload before hardening.
if [ -f /run/zgi-agent/bootstrap-config.json ]; then
  ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED="$(python3 -c 'import json; c=json.load(open("/run/zgi-agent/bootstrap-config.json")); v=c.get("platform_cli",False); assert type(v) is bool; print("true" if v else "false")')"
  export ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED
fi

# E2B preserves a sandbox-local MMDS route outside its provider egress filter.
# Jobs may connect only to the loopback package proxy. envd/root retain
# bootstrap access; the root proxy applies HTTPS host/IP/port validation on
# every request in addition to E2B's outer egress allowlist.
if [ "${ZGI_RUNTIME_AGENT_EGRESS_MODE:-bridge}" = direct ] || [ "${ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED:-false}" = true ]; then
  job_uid="${ZGI_RUNTIME_AGENT_JOB_UID:-65532}"
  case "$job_uid" in
    ''|0|*[!0-9]*) echo 'direct egress requires a non-root Job UID' >&2; exit 1 ;;
  esac
  # Some E2B base images make the language installation writable by all
  # users. Restore the Profile boundary before starting any untrusted Job:
  # pip writes to the session venv and npm writes to the workspace, never
  # to the base interpreters, global modules or trusted agent executable.
  /usr/bin/chown -R root:root /usr/local /opt/zgi-profile /opt/corepack
  /usr/bin/chmod -R go-w /usr/local /opt/zgi-profile /opt/corepack
  # Platform Jobs use their own transient socket grant, never the root control
  # token. Keep bootstrap files and socket names outside the Job's write scope.
  if [ "${ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED:-false}" = true ]; then
    /usr/bin/chown root:65532 /run/zgi-agent
    /usr/bin/chmod 0750 /run/zgi-agent
    /usr/bin/chown root:root "$token_file"
    /usr/bin/chmod 0600 "$token_file"
    if [ -f /run/zgi-agent/bootstrap-config.json ]; then
      /usr/bin/chown root:root /run/zgi-agent/bootstrap-config.json
      /usr/bin/chmod 0600 /run/zgi-agent/bootstrap-config.json
    fi
  fi
  if [ "${ZGI_RUNTIME_AGENT_EGRESS_PROXY_ADDRESS:-127.0.0.1:3128}" != 127.0.0.1:3128 ]; then
    echo 'direct egress requires the guarded loopback proxy address' >&2
    exit 1
  fi
  # Native nft meta skuid works on E2B kernels without the xt_owner module.
  # The batch is atomic and replaces only our own chain on repeated startup.
  printf '%s\n' \
    'add table inet zgi_runtime_guard' \
    'add chain inet zgi_runtime_guard output { type filter hook output priority 0; policy accept; }' \
    'flush chain inet zgi_runtime_guard output' \
    "add rule inet zgi_runtime_guard output meta skuid $job_uid ip daddr 127.0.0.1 tcp dport 3128 accept" \
    "add rule inet zgi_runtime_guard output meta skuid $job_uid reject" \
    | /usr/sbin/nft --file -
  /usr/sbin/nft --numeric list chain inet zgi_runtime_guard output \
    | grep -F "meta skuid $job_uid reject" >/dev/null
  /usr/sbin/nft --numeric list chain inet zgi_runtime_guard output \
    | grep -F "meta skuid $job_uid ip daddr 127.0.0.1 tcp dport 3128 accept" >/dev/null
fi

exec /usr/local/bin/zgi-runtime-agent
