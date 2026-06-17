@echo off
REM run.cmd - build the game to a stable, execution-allowed path and launch it.
REM Type "run" in cmd.exe. Use this instead of "go run .": this machine blocks
REM executing binaries from %TEMP% (where go run builds), so go run fails with
REM "fork/exec ... Access is denied". Building into the project dir works.
setlocal
taskkill /IM crawler-3d.exe /F >nul 2>&1
echo Building...
go build -o "%~dp0.codex-build\crawler-3d.exe" "%~dp0."
if errorlevel 1 ( echo BUILD FAILED & exit /b 1 )
echo Launching...
"%~dp0.codex-build\crawler-3d.exe" %*
