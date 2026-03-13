# run_and_validate.ps1
# Compila, roda testes e gera dashboard.html a partir dos logs em logs/ (dados reais dos testes).
# Uso: .\scripts\run_and_validate.ps1 [-OpenDashboard]

param(
    [switch]$OpenDashboard  # Abre dashboard.html no navegador ao final
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $Root

Write-Host "=== 1. Go build ===" -ForegroundColor Cyan
go build ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Build OK." -ForegroundColor Green

Write-Host "`n=== 2. Go test (server) ===" -ForegroundColor Cyan
go test ./src/server/... -count=1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Tests OK." -ForegroundColor Green

Write-Host "`n=== 3. Gerar dashboard (dados em logs/) ===" -ForegroundColor Cyan
python dashboard.py --base-dir (Join-Path $Root "logs") --output (Join-Path $Root "dashboard.html")
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "dashboard.html gerado em $Root" -ForegroundColor Green

if ($OpenDashboard) {
    Start-Process (Join-Path $Root "dashboard.html")
}

Write-Host "`n=== Concluido ===" -ForegroundColor Green
