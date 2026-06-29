@echo off
REM build.cmd - compile + link to the stable exe without launching. Type "build".
setlocal
go build -o "%~dp0.build\crawler-3d.exe" "%~dp0."
if errorlevel 1 ( echo BUILD FAILED & exit /b 1 )
echo BUILD OK: .build\crawler-3d.exe
