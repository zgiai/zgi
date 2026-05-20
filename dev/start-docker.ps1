param(
  [switch]$china,
  [switch]$runtime,
  [switch]$knowledge,
  [switch]$full
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Split-Path -Parent $ScriptDir

if ($china) {
  if (-not $env:BUILD_REGION) { $env:BUILD_REGION = 'cn' }
  if (-not $env:APT_MIRROR) { $env:APT_MIRROR = 'mirrors.aliyun.com' }
  if (-not $env:APT_SECURITY_MIRROR) { $env:APT_SECURITY_MIRROR = 'mirrors.aliyun.com' }
  if (-not $env:APK_MIRROR) { $env:APK_MIRROR = 'mirrors.aliyun.com' }
  if (-not $env:NPM_REGISTRY) { $env:NPM_REGISTRY = 'https://registry.npmmirror.com' }
  if (-not $env:PIP_INDEX_URL) { $env:PIP_INDEX_URL = 'https://pypi.tuna.tsinghua.edu.cn/simple' }
  if (-not $env:UV_INDEX_URL) { $env:UV_INDEX_URL = 'https://pypi.tuna.tsinghua.edu.cn/simple' }
  if (-not $env:GOPROXY) { $env:GOPROXY = 'https://goproxy.cn,direct' }
  Write-Host "[start-docker] build region: china"
}

function Add-ComposeProfile {
  param([string]$Profile)

  $profiles = @()
  if ($env:COMPOSE_PROFILES) {
    $profiles = $env:COMPOSE_PROFILES.Split(',') | Where-Object { $_ }
  }
  if ($profiles -notcontains $Profile) {
    $profiles += $Profile
  }
  $env:COMPOSE_PROFILES = ($profiles -join ',')
}

function Enable-RuntimeProfile {
  Add-ComposeProfile 'runtime'
  if (-not $env:CODE_EXECUTION_ENDPOINT) { $env:CODE_EXECUTION_ENDPOINT = 'http://zgi-sandbox:2660' }
  if (-not $env:PLUGIN_RUNNER_ENABLED) { $env:PLUGIN_RUNNER_ENABLED = 'true' }
  if (-not $env:PLUGIN_RUNNER_URL) { $env:PLUGIN_RUNNER_URL = 'http://runner:2665' }
  Write-Host "[start-docker] enable runtime profile"
}

function Enable-KnowledgeProfile {
  Add-ComposeProfile 'knowledge'
  if (-not $env:VECTOR_STORE) { $env:VECTOR_STORE = 'weaviate' }
  if (-not $env:WEAVIATE_ENDPOINT) { $env:WEAVIATE_ENDPOINT = 'http://weaviate:8080' }
  if (-not $env:NEO4J_URI) { $env:NEO4J_URI = 'bolt://neo4j:7687' }
  Write-Host "[start-docker] enable knowledge profile"
}

if ($runtime) {
  Enable-RuntimeProfile
}

if ($knowledge) {
  Enable-KnowledgeProfile
}

if ($full) {
  Enable-RuntimeProfile
  Enable-KnowledgeProfile
}

Write-Host "[start-docker] bootstrap repository"
& (Join-Path $Root 'dev/bootstrap.ps1')

Push-Location (Join-Path $Root 'docker')
try {
  docker compose --env-file .env up -d --build
}
finally {
  Pop-Location
}
