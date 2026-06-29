# run.ps1 — build to a stable, execution-allowed path and launch the game.
# Use this instead of `go run .`: on this machine the OS blocks executing
# binaries from %TEMP% (AppLocker / SRP), which is where `go run` puts its
# temp exe — hence "fork/exec ... Access is denied". Building into the project
# dir sidesteps that entirely.
$ErrorActionPreference = "Stop"
$proj = $PSScriptRoot
$exe  = Join-Path $proj ".build\crawler-3d.exe"

# A still-running instance locks the exe and makes the rebuild fail — kill it.
Get-Process -Name crawler-3d -ErrorAction SilentlyContinue | Stop-Process -Force

Write-Host "Building -> $exe"
& go build -o $exe $proj
if ($LASTEXITCODE -ne 0) { Write-Host "build failed"; exit 1 }

Write-Host "Launching..."
& $exe
