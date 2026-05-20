param(
  [switch]$china
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

Write-Host "[start-docker] bootstrap repository"
& (Join-Path $Root 'dev/bootstrap.ps1')

Push-Location (Join-Path $Root 'docker')
try {
  docker compose --env-file .env up -d --build
}
finally {
  Pop-Location
}
