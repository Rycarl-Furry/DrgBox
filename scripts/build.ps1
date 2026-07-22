$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Write-Host '[1/4] Go tests' -ForegroundColor Cyan
go test ./...

Write-Host '[2/4] Frontend build' -ForegroundColor Cyan
npm install --prefix frontend
npm run build --prefix frontend

Write-Host '[3/4] Wails production build' -ForegroundColor Cyan
wails build

Write-Host '[4/4] Embedded installer' -ForegroundColor Cyan
$app = Join-Path $root 'build\bin\DrgBoxDesktop.exe'
$embedded = Join-Path $root 'installer\setup\DrgBoxDesktop.exe'
$installer = Join-Path $root 'build\installer\DRGBOX-Setup.exe'
Copy-Item -LiteralPath $app -Destination $embedded -Force
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $installer) | Out-Null
Push-Location (Join-Path $root 'installer\setup')
try {
    go build -ldflags '-H=windowsgui' -o $installer .
} finally {
    Pop-Location
}

Write-Host "Build complete:`n$app`n$installer" -ForegroundColor Green

