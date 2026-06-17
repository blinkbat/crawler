@echo off
REM test.cmd — run the full test suite + vet. Type `test`.
setlocal
go test ./... %*
go vet ./...
