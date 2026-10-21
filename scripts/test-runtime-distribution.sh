#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d /tmp/zgi-runtime-distribution.XXXXXX)"
trap 'rm -rf "$tmp"' EXIT
cp "$root/sandbox/deploy/compose.e2b.yaml" "$tmp/compose.yaml"
cp "$root/sandbox/deploy/e2b.env.example" "$tmp/e2b.env"
export ZGI_AGENT_RUNTIME_ENV_FILE="$tmp/e2b.env"
# Render only. Never start services, contact a provider, or print credentials.
ZGI_AGENT_RUNTIME_WORKER_IMAGE='registry.example/worker@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
ZGI_AGENT_RUNTIME_DOCKER_NETWORK='runtime-distribution-test' \
  docker compose -f "$tmp/compose.yaml" config --format json > "$tmp/config.json"
jq -e '
  .services.agentworker as $w |
  ($w.environment.ZGI_AGENT_RUNTIME_BACKEND == "e2b") and
  ($w.environment.ZGI_AGENT_RUNTIME_CHECKPOINT_DIR == "/var/lib/zgi-ar/checkpoints") and
  (($w.ports // []) | length == 0) and
  ($w.volumes | length == 1) and
  ($w.volumes[0].target == "/var/lib/zgi-ar") and
  (.networks.zgi.external == true)
' "$tmp/config.json" >/dev/null
if env -u ZGI_AGENT_RUNTIME_WORKER_IMAGE -u ZGI_AGENT_RUNTIME_DOCKER_NETWORK \
    docker compose -f "$tmp/compose.yaml" config --quiet >/dev/null 2>&1; then
  echo 'runtime-distribution: missing operator image/network accepted' >&2
  exit 1
fi
echo 'runtime-distribution: private E2B compose contract passed'

# A fixture binary tests packaging, not Linux architecture or backend isolation.
bash "$root/scripts/package-runtime-worker.sh" "$(readlink -f /bin/true)" "$tmp/worker.tar.gz" aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
mkdir "$tmp/extracted"
tar -xzf "$tmp/worker.tar.gz" -C "$tmp/extracted"
(cd "$tmp/extracted" && sha256sum -c SHA256SUMS >/dev/null)
[[ -x "$tmp/extracted/zgi-agent-runtime-worker" ]]
[[ -f "$tmp/extracted/sandbox/scripts/zgi-runtime-server" ]]
[[ -f "$tmp/extracted/docs/agent-runtime-m6b-dependency-preparation.md" ]]
[[ ! -f "$tmp/extracted/sandbox/deploy/e2b.env" && ! -f "$tmp/extracted/sandbox/deploy/settings.json" ]]
echo 'runtime-distribution: native archive contract passed'
