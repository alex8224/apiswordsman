@echo off
setlocal

if "%~1"=="linux" (
    set GOOS=linux
    set GOARCH=amd64
    go clean && go build
) else (
     set GOOS=windows
     set GOARCH=amd64
     go clean
     go build  -ldflags -H=windowsgui
)

endlocal
