$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Split-Path -Parent $ScriptDir

function Get-DisplayPath {
  param([string]$TargetPath)

  $normalizedRoot = $Root.TrimEnd('\', '/')
  if ($TargetPath.StartsWith($normalizedRoot)) {
    return $TargetPath.Substring($normalizedRoot.Length).TrimStart('\', '/')
  }

  return $TargetPath
}

function Copy-IfMissing {
  param(
    [string]$SourcePath,
    [string]$TargetPath
  )

  if (-not (Test-Path -LiteralPath $SourcePath)) {
    throw "[bootstrap] missing template: $SourcePath"
  }

  if (Test-Path -LiteralPath $TargetPath) {
    Write-Host "[bootstrap] keep existing $(Get-DisplayPath -TargetPath $TargetPath)"
    return
  }

  $parent = Split-Path -Parent $TargetPath
  if (-not (Test-Path -LiteralPath $parent)) {
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
  }

  Copy-Item -LiteralPath $SourcePath -Destination $TargetPath
  Write-Host "[bootstrap] created $(Get-DisplayPath -TargetPath $TargetPath)"
}

function New-Secret32 {
  return ([guid]::NewGuid().ToString('N'))
}

function Test-UnsetOrPlaceholder {
  param([string]$Value)

  return [string]::IsNullOrWhiteSpace($Value) -or @(
    'replace-with-strong-random-secret',
    'replace-with-32-byte-random-key',
    'change-me-in-production',
    'change-me'
  ) -contains $Value.Trim()
}

function Ensure-EnvValue {
  param(
    [string]$TargetPath,
    [string]$Key,
    [string]$Value
  )

  if (-not (Test-Path -LiteralPath $TargetPath)) {
    throw "[bootstrap] missing env file: $TargetPath"
  }

  $content = Get-Content -LiteralPath $TargetPath -Raw
  $pattern = "(?m)^$([regex]::Escape($Key))=(.*)$"
  $match = [regex]::Match($content, $pattern)
  if ($match.Success) {
    $current = $match.Groups[1].Value
    if (Test-UnsetOrPlaceholder -Value $current) {
      $content = [regex]::Replace($content, $pattern, "$Key=$Value", 1)
      Set-Content -LiteralPath $TargetPath -Value $content -NoNewline
      Write-Host "[bootstrap] updated $(Get-DisplayPath -TargetPath $TargetPath) $Key"
    }
    return
  }

  Add-Content -LiteralPath $TargetPath -Value "`n$Key=$Value"
  Write-Host "[bootstrap] added $(Get-DisplayPath -TargetPath $TargetPath) $Key"
}

function Replace-EnvValueIfCurrent {
  param(
    [string]$TargetPath,
    [string]$Key,
    [string]$OldValue,
    [string]$NewValue
  )

  if (-not (Test-Path -LiteralPath $TargetPath)) {
    return
  }

  $content = Get-Content -LiteralPath $TargetPath -Raw
  $pattern = "(?m)^$([regex]::Escape($Key))=(.*)$"
  $match = [regex]::Match($content, $pattern)
  if ($match.Success -and $match.Groups[1].Value -eq $OldValue) {
    $content = [regex]::Replace($content, $pattern, "$Key=$NewValue", 1)
    Set-Content -LiteralPath $TargetPath -Value $content -NoNewline
    Write-Host "[bootstrap] updated $(Get-DisplayPath -TargetPath $TargetPath) $Key"
  }
}

function Remove-EnvKey {
  param(
    [string]$TargetPath,
    [string]$Key
  )

  if (-not (Test-Path -LiteralPath $TargetPath)) {
    return
  }

  $content = Get-Content -LiteralPath $TargetPath -Raw
  $pattern = "(?m)^$([regex]::Escape($Key))=.*(?:\r?\n)?"
  if ([regex]::IsMatch($content, $pattern)) {
    $content = [regex]::Replace($content, $pattern, '', 1)
    Set-Content -LiteralPath $TargetPath -Value $content -NoNewline
    Write-Host "[bootstrap] removed $(Get-DisplayPath -TargetPath $TargetPath) $Key"
  }
}

function Get-EnvValue {
  param(
    [string]$TargetPath,
    [string]$Key
  )

  if (-not (Test-Path -LiteralPath $TargetPath)) {
    return $null
  }

  $content = Get-Content -LiteralPath $TargetPath -Raw
  $pattern = "(?m)^$([regex]::Escape($Key))=(.*)$"
  $match = [regex]::Match($content, $pattern)
  if ($match.Success) {
    return $match.Groups[1].Value
  }

  return $null
}

Set-Location $Root

Copy-IfMissing -SourcePath (Join-Path $Root 'docker/.env.example') -TargetPath (Join-Path $Root 'docker/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'api/.env.example') -TargetPath (Join-Path $Root 'api/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'api/.env.docker.example') -TargetPath (Join-Path $Root 'api/.env.docker')
Copy-IfMissing -SourcePath (Join-Path $Root 'web/.env.example') -TargetPath (Join-Path $Root 'web/.env.local')
Copy-IfMissing -SourcePath (Join-Path $Root 'sandbox/.env.example') -TargetPath (Join-Path $Root 'sandbox/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'runner/.env.example') -TargetPath (Join-Path $Root 'runner/.env')

$apiEnv = Join-Path $Root 'api/.env'
$apiDockerEnv = Join-Path $Root 'api/.env.docker'
$dockerEnv = Join-Path $Root 'docker/.env'
$postgresPassword = New-Secret32
$redisPassword = New-Secret32
$runnerApiKey = New-Secret32
$sandboxPostgresPassword = New-Secret32
Ensure-EnvValue -TargetPath $apiEnv -Key 'SECRET_KEY' -Value (New-Secret32)
Ensure-EnvValue -TargetPath $apiEnv -Key 'API_KEY_ENCRYPTION_KEY' -Value (New-Secret32)
Ensure-EnvValue -TargetPath $apiDockerEnv -Key 'SECRET_KEY' -Value (New-Secret32)
Ensure-EnvValue -TargetPath $apiDockerEnv -Key 'API_KEY_ENCRYPTION_KEY' -Value (New-Secret32)
Replace-EnvValueIfCurrent -TargetPath $apiDockerEnv -Key 'SQL_BASE_INTERNAL_DB' -OldValue 'zgi' -NewValue 'zgi_sql_base'
Replace-EnvValueIfCurrent -TargetPath $apiEnv -Key 'NEO4J_URI' -OldValue 'bolt://localhost:7687' -NewValue ''
Replace-EnvValueIfCurrent -TargetPath $apiDockerEnv -Key 'NEO4J_URI' -OldValue 'bolt://neo4j:7687' -NewValue ''
Replace-EnvValueIfCurrent -TargetPath $dockerEnv -Key 'NEO4J_URI' -OldValue 'bolt://neo4j:7687' -NewValue ''
Remove-EnvKey -TargetPath $apiEnv -Key 'NEO4J_URI'
Remove-EnvKey -TargetPath $apiEnv -Key 'NEO4J_USERNAME'
Remove-EnvKey -TargetPath $apiEnv -Key 'NEO4J_PASSWORD'
Remove-EnvKey -TargetPath $apiEnv -Key 'NEO4J_DATABASE'
Remove-EnvKey -TargetPath $apiDockerEnv -Key 'NEO4J_URI'
Remove-EnvKey -TargetPath $apiDockerEnv -Key 'NEO4J_USERNAME'
Remove-EnvKey -TargetPath $apiDockerEnv -Key 'NEO4J_PASSWORD'
Remove-EnvKey -TargetPath $apiDockerEnv -Key 'NEO4J_DATABASE'
Remove-EnvKey -TargetPath $dockerEnv -Key 'NEO4J_URI'
Remove-EnvKey -TargetPath $dockerEnv -Key 'NEO4J_USERNAME'
Remove-EnvKey -TargetPath $dockerEnv -Key 'NEO4J_PASSWORD'
Remove-EnvKey -TargetPath $dockerEnv -Key 'NEO4J_DATABASE'
Ensure-EnvValue -TargetPath $dockerEnv -Key 'PUBLIC_PORT' -Value '2679'
Ensure-EnvValue -TargetPath $dockerEnv -Key 'PUBLIC_URL' -Value 'http://localhost:2679'
Ensure-EnvValue -TargetPath $dockerEnv -Key 'POSTGRES_PASSWORD' -Value $postgresPassword
Ensure-EnvValue -TargetPath $dockerEnv -Key 'REDIS_PASSWORD' -Value $redisPassword

$currentPostgresPassword = Get-EnvValue -TargetPath $dockerEnv -Key 'POSTGRES_PASSWORD'
if (-not [string]::IsNullOrWhiteSpace($currentPostgresPassword)) {
  $postgresPassword = $currentPostgresPassword
}
$currentRedisPassword = Get-EnvValue -TargetPath $dockerEnv -Key 'REDIS_PASSWORD'
if (-not [string]::IsNullOrWhiteSpace($currentRedisPassword)) {
  $redisPassword = $currentRedisPassword
}
Ensure-EnvValue -TargetPath $apiEnv -Key 'DB_PASSWORD' -Value $postgresPassword
Ensure-EnvValue -TargetPath $apiEnv -Key 'REDIS_PASSWORD' -Value $redisPassword
Ensure-EnvValue -TargetPath $apiDockerEnv -Key 'DB_PASSWORD' -Value $postgresPassword
Ensure-EnvValue -TargetPath $apiDockerEnv -Key 'REDIS_PASSWORD' -Value $redisPassword

$runnerEnv = Join-Path $Root 'runner/.env'
Ensure-EnvValue -TargetPath $runnerEnv -Key 'EXECUTOR_API_KEY' -Value $runnerApiKey
$currentRunnerApiKey = Get-EnvValue -TargetPath $runnerEnv -Key 'EXECUTOR_API_KEY'
if (-not [string]::IsNullOrWhiteSpace($currentRunnerApiKey)) {
  $runnerApiKey = $currentRunnerApiKey
}
Ensure-EnvValue -TargetPath $apiEnv -Key 'PLUGIN_RUNNER_API_KEY' -Value $runnerApiKey
Ensure-EnvValue -TargetPath $apiDockerEnv -Key 'PLUGIN_RUNNER_API_KEY' -Value $runnerApiKey
Ensure-EnvValue -TargetPath $runnerEnv -Key 'EXECUTOR_DB_PASSWORD' -Value $postgresPassword
Ensure-EnvValue -TargetPath $runnerEnv -Key 'EXECUTOR_REDIS_PASSWORD' -Value $redisPassword

$sandboxEnv = Join-Path $Root 'sandbox/.env'
Ensure-EnvValue -TargetPath $sandboxEnv -Key 'POSTGRES_PASSWORD' -Value $sandboxPostgresPassword
$currentSandboxPostgresPassword = Get-EnvValue -TargetPath $sandboxEnv -Key 'POSTGRES_PASSWORD'
if (-not [string]::IsNullOrWhiteSpace($currentSandboxPostgresPassword)) {
  $sandboxPostgresPassword = $currentSandboxPostgresPassword
}
Ensure-EnvValue -TargetPath $sandboxEnv -Key 'ZGI_SANDBOX_DATABASE_URL' -Value "postgres://postgres:$sandboxPostgresPassword@sandbox-postgres:5432/zgi_sandbox?sslmode=disable"

Write-Host "[bootstrap] generate docker compose"
& (Join-Path $Root 'docker/generate_docker_compose.ps1')

Write-Host "[bootstrap] done"
