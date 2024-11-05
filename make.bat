@echo off
setlocal

if "%~1"=="linux" (
    set GOOS=linux
    set GOARCH=amd64
    go clean && go build -o dist/apixia-linux-amd64
) else (
     set GOOS=windows
     set GOARCH=amd64
     go clean
     go build  -ldflags -H=windowsgui -o dist/apixia-windows-amd64.exe
)

endlocal
