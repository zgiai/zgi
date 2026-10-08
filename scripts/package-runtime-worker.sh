#!/usr/bin/env bash
set -euo pipefail

[[ $# == 3 ]] || { echo 'usage: package-runtime-worker.sh binary archive source-revision' >&2; exit 2; }
binary="$1"
archive="$2"
revision="$3"
[[ -f "$binary" && ! -L "$binary" && "$revision" =~ ^[a-f0-9]{40}$ ]] || {
  echo 'runtime-package: regular binary and full source revision required' >&2; exit 2;
}
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stage="$(mktemp -d /tmp/zgi-runtime-package.XXXXXXXX)"
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$stage/docs" "$stage/sandbox/docs" "$stage/sandbox/deploy" "$stage/sandbox/scripts"
cp "$binary" "$stage/zgi-agent-runtime-worker"
chmod 755 "$stage/zgi-agent-runtime-worker"
cp "$root/sandbox/scripts/zgi-runtime-server" "$root/sandbox/scripts/verify-agent-runtime-release" "$stage/"
cp "$stage/zgi-runtime-server" "$stage/verify-agent-runtime-release" "$stage/sandbox/scripts/"
for guide in agent-runtime-quickstart portable-agent-skills agent-runtime-platform-cli agent-runtime-m6b-dependency-preparation; do
  cp "$root/docs/$guide.md" "$stage/docs/"
done
cp "$root"/sandbox/docs/agent-runtime-*.md "$stage/sandbox/docs/"
# Only maintained public examples, never adjacent operator .env/settings files.
cp "$root"/sandbox/deploy/*.example* "$root/sandbox/deploy/compose.e2b.yaml" "$stage/sandbox/deploy/"
printf '%s\n' '# Agent Runtime Worker' '' 'Start with [setup and use](docs/agent-runtime-quickstart.md).' > "$stage/README.md"
printf '%s\n' "$revision" > "$stage/SOURCE_REVISION"
# The generated manifest is explicitly excluded from the input file list.
# shellcheck disable=SC2094
(cd "$stage" && find . -type f ! -name SHA256SUMS -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > SHA256SUMS)
tar -czf "$archive" -C "$stage" .
echo 'runtime-package: native Worker, public examples, guides and checksums packaged'
