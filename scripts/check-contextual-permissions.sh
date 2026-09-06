#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Synthetic configuration for isolated tests; no external provider calls.
export CONNECTOR_CREDENTIAL_ACTIVE_KEY_ID="contextual-permissions-ci"
export CONNECTOR_CREDENTIAL_KEYS_JSON='{"contextual-permissions-ci":"contextual-test-key-000000000001"}'
cd "${repo_root}/api"
go test ./internal/modules/tools/builtin/agentmanagement \
  ./internal/modules/tools/builtin/files -count=1
go test ./internal/modules/app/workflowtest ./internal/modules/dataset/handler -count=1
go test ./internal/capabilities/chatruntime/service \
  -run 'Test(TrustedContextual|ContextualAgentSkillDiscovery|ContextualFileSkillDiscovery|AddContextualAIChatSkillIDsDoesNotUseText|ContextualAIChatSkillIDsUseModelIntent)' \
  -count=1
