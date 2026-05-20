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
    'change-me-in-production'
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

Set-Location $Root

Copy-IfMissing -SourcePath (Join-Path $Root 'docker/.env.example') -TargetPath (Join-Path $Root 'docker/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'api/.env.example') -TargetPath (Join-Path $Root 'api/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'api/.env.docker.example') -TargetPath (Join-Path $Root 'api/.env.docker')
Copy-IfMissing -SourcePath (Join-Path $Root 'web/.env.example') -TargetPath (Join-Path $Root 'web/.env.local')
Copy-IfMissing -SourcePath (Join-Path $Root 'sandbox/.env.example') -TargetPath (Join-Path $Root 'sandbox/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'runner/.env.example') -TargetPath (Join-Path $Root 'runner/.env')

$apiEnv = Join-Path $Root 'api/.env'
$apiDockerEnv = Join-Path $Root 'api/.env.docker'
Ensure-EnvValue -TargetPath $apiEnv -Key 'SECRET_KEY' -Value (New-Secret32)
Ensure-EnvValue -TargetPath $apiEnv -Key 'API_KEY_ENCRYPTION_KEY' -Value (New-Secret32)
Ensure-EnvValue -TargetPath $apiDockerEnv -Key 'SECRET_KEY' -Value (New-Secret32)
Ensure-EnvValue -TargetPath $apiDockerEnv -Key 'API_KEY_ENCRYPTION_KEY' -Value (New-Secret32)
Replace-EnvValueIfCurrent -TargetPath $apiDockerEnv -Key 'SQL_BASE_INTERNAL_DB' -OldValue 'zgi' -NewValue 'zgi_sql_base'
Replace-EnvValueIfCurrent -TargetPath (Join-Path $Root 'docker/.env') -Key 'HOST_WEAVIATE_PORT' -OldValue '18080' -NewValue '18081'

Write-Host "[bootstrap] generate docker compose"
& (Join-Path $Root 'docker/generate_docker_compose.ps1')

Write-Host "[bootstrap] done"
