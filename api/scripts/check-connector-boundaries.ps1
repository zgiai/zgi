param([switch]$CoreOnly)
$ErrorActionPreference = 'Stop'
$connectorApiRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location -LiteralPath $connectorApiRoot
try {
  $connectorTargets = @('./internal/modules/connectors/...')
  if (-not $CoreOnly) { $connectorTargets += @('./cmd/server/...','./internal/container','./routes/v1') }
  $connectorDependencies = & go list -deps -test @connectorTargets
  if ($LASTEXITCODE -ne 0) { throw 'Connector dependency graph does not compile.' }
  $connectorForbidden = @($connectorDependencies | Where-Object { $_ -match '^github.com/zgiai/zgi/api/internal/modules/integrations(/|$)' })
  if ($connectorForbidden.Count) { throw ('Legacy Connector dependencies remain: ' + ($connectorForbidden -join ', ')) }
  $connectorProductionDependencies = & go list -deps './internal/modules/connectors/...'
  if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve native Connector production dependencies.' }
  $connectorHostImports = @($connectorProductionDependencies | Where-Object { $_ -match '^github.com/zgiai/zgi/api/internal/(modules/(tools|app/agents|app/workflow|aichat)|capabilities/(chatruntime|connectortools))(/|$)' })
  if ($connectorHostImports.Count) { throw ('Connector core depends on host execution: ' + ($connectorHostImports -join ', ')) }
  $connectorSQLRoots = @('internal', 'routes', 'cmd')
  if ($CoreOnly) { $connectorSQLRoots = @('internal/modules/connectors') }
  $connectorLegacySQL = & rg -n '\b(integration_action_policies|integration_connection_grants|integration_connection_health_events|integration_connections|integration_executions|integration_oauth_client_configs|integration_oauth_flows|integration_oauth_recovery_operations|integration_oauth_states|integration_operation_receipts|integration_provider_authorizations|integration_provider_install_flows|integration_provider_suite_states|aichat_integration_preferences|connector_execution_route_claims|connector_legacy_identity_mappings|connector_migration_inventory|connector_mutation_journal|connector_provider_rollouts|connector_reversal_baselines|connector_rollout_evidence|connector_shadow_observations)\b' @connectorSQLRoots -g '*.go' -g '!**/*_test.go' -g '!**/migrations/**'
  if ($LASTEXITCODE -eq 0) { throw ('Native Connector code references retired SQL: ' + ($connectorLegacySQL -join ', ')) }
  if ($LASTEXITCODE -gt 1) { throw 'Cannot scan Connector SQL dependencies.' }
  $connectorRetiredProtocol = & rg -n 'external-apps|execute_action|ArgumentEnvelope|NativeExternalAction|server_projected_ledger_epoch|server_binding_fingerprint' @connectorSQLRoots -g '*.go' -g '!**/*_test.go' -g '!**/migrations/**'
  if ($LASTEXITCODE -eq 0) { throw ('Retired Connector runtime protocol remains: ' + ($connectorRetiredProtocol -join ', ')) }
  if ($LASTEXITCODE -gt 1) { throw 'Cannot scan Connector runtime protocol.' }
  if (Test-Path -LiteralPath './internal/modules/connectors/compatibility') { throw 'Online Connector compatibility implementation remains.' }
  if (-not $CoreOnly -and (Test-Path -LiteralPath './internal/modules/integrations')) { throw 'Retired integrations implementation remains.' }
  Write-Output 'Connector production and test dependency boundary passed.'
} finally { Pop-Location }
exit 0
