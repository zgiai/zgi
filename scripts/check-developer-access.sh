#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Synthetic credentials satisfy connector initialization in isolated tests only.
export CONNECTOR_CREDENTIAL_ACTIVE_KEY_ID="developer-access-ci"
export CONNECTOR_CREDENTIAL_KEYS_JSON='{"developer-access-ci":"developer-access-test-key-000001"}'
cd "${repo_root}/api"
go test ./internal/modules/llm/developeraccess \
  ./internal/modules/llm/apikey/repository \
  ./internal/modules/llm/apikey/service \
  ./internal/modules/llm/gateway/... -count=1
go test ./internal/bootstrap/fxapp -run '^TestProvideGinEngine_' -count=1
