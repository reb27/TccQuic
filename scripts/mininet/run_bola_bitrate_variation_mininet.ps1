#Requires -Version 5.1
<#
.SYNOPSIS
  Corre run_bola_bitrate_variation_mininet.sh via Git Bash (PowerShell no Windows).

.EXAMPLE
  cd scripts\mininet
  .\run_bola_bitrate_variation_mininet.ps1 192.168.56.101
  .\run_bola_bitrate_variation_mininet.ps1 --reuse 192.168.56.101
  .\run_bola_bitrate_variation_mininet.ps1 --generate-variants 192.168.56.101
#>
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ScriptArgs
)

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$sh = Join-Path $here "run_bola_bitrate_variation_mininet.sh"

if (-not (Test-Path $sh)) {
    Write-Error "Nao encontrei: $sh"
    exit 1
}

$bashExe = $null
foreach ($c in @(
        "${env:ProgramFiles}\Git\bin\bash.exe",
        "${env:ProgramFiles(x86)}\Git\bin\bash.exe"
    )) {
    if ($c -and (Test-Path $c)) {
        $bashExe = $c
        break
    }
}
if (-not $bashExe) {
    $cmd = Get-Command bash.exe -ErrorAction SilentlyContinue
    if ($cmd) { $bashExe = $cmd.Source }
}
if (-not $bashExe) {
    Write-Error "Git Bash nao encontrado. Instala Git for Windows (https://git-scm.com/download/win) ou adiciona bash ao PATH."
    exit 1
}

Set-Location $here
& $bashExe $sh @ScriptArgs
exit $LASTEXITCODE
