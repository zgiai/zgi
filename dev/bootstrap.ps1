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

Write-Host "[bootstrap] sync submodules"
Set-Location $Root
git submodule sync
git submodule update --init --remote --merge

Copy-IfMissing -SourcePath (Join-Path $Root 'docker/.env.example') -TargetPath (Join-Path $Root 'docker/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'api/.env.example') -TargetPath (Join-Path $Root 'api/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'api/.env.docker.example') -TargetPath (Join-Path $Root 'api/.env.docker')
Copy-IfMissing -SourcePath (Join-Path $Root 'web/.env.example') -TargetPath (Join-Path $Root 'web/.env.local')
Copy-IfMissing -SourcePath (Join-Path $Root 'sandbox/.env.example') -TargetPath (Join-Path $Root 'sandbox/.env')
Copy-IfMissing -SourcePath (Join-Path $Root 'plugin-runner/.env.example') -TargetPath (Join-Path $Root 'plugin-runner/.env')

Write-Host "[bootstrap] generate docker compose"
& (Join-Path $Root 'docker/generate_docker_compose.ps1')

Write-Host "[bootstrap] done"
