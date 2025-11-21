@echo off
REM Build script for Windows GUI application

echo Building CV Review Agent GUI for Windows...

REM Set environment for Windows build
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=1

REM Build the GUI application
cd ..
go build -ldflags="-H windowsgui" -o cmd/gui/cv-review-agent-gui.exe cmd/gui/main.go

if %ERRORLEVEL% EQU 0 (
    echo Build successful! Executable created at: cmd\gui\cv-review-agent-gui.exe
) else (
    echo Build failed with error code %ERRORLEVEL%
    exit /b %ERRORLEVEL%
)

echo.
echo To create an installer, run the installer script in installer\windows\
pause
