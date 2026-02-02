@echo off
REM Development helper script
REM Workaround for wails dev hanging on some systems

REM Kill any running instance so the build can replace the binary
taskkill /F /IM goop2.exe >nul 2>&1
timeout /t 2 /nobreak >nul

REM Remove old binary before build to avoid directory lock issues
del /F /Q "build\bin\goop2.exe" >nul 2>&1

echo Building application...
wails build -windowsconsole
if %errorlevel% neq 0 (
    echo Build failed!
    exit /b 1
)

echo.
echo Build complete!
echo.
echo Running application...
echo    Desktop UI:  .\build\bin\goop2.exe
echo    CLI Peer:    .\build\bin\goop2.exe peer .\peers\peerA
echo    Rendezvous:  .\build\bin\goop2.exe rendezvous .\peers\peerA
echo.

REM Run the app
.\build\bin\goop2.exe
