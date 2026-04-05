@echo off
setlocal

set "ROOT=%~dp0"
set "SERVER_DIR=%ROOT%server"
set "CLIENT_DIR=%ROOT%client"
set "CACHE_DIR=%ROOT%.cache"
set "GOCACHE_DIR=%CACHE_DIR%\go-build"
set "BIN_DIR=%CACHE_DIR%\bin"
set "SERVER_EXE=%BIN_DIR%\server.exe"
set "PICKER_EXE=%BIN_DIR%\region-picker.exe"
set "BUILDER_EXE=%BIN_DIR%\map-builder.exe"
set "CLIENT_URL=http://localhost:3000/navigation.html"

where go >nul 2>nul
if errorlevel 1 (
  echo Go was not found in PATH.
  pause
  exit /b 1
)

where python >nul 2>nul
if errorlevel 1 (
  echo Python was not found in PATH.
  pause
  exit /b 1
)

if not exist "%CACHE_DIR%" mkdir "%CACHE_DIR%"
if not exist "%GOCACHE_DIR%" mkdir "%GOCACHE_DIR%"
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"

:: ─── Build binaries ───────────────────────────────────────────────────────────
echo Building Go binaries...
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"

go build -o "%BUILDER_EXE%" .\cmd\map-builder
if errorlevel 1 (
  echo Map-builder build failed.
  popd
  pause
  exit /b 1
)

go build -o "%PICKER_EXE%" .\cmd\region-picker
if errorlevel 1 (
  echo Region-picker build failed.
  popd
  pause
  exit /b 1
)

go build -o "%SERVER_EXE%" .\cmd\server
if errorlevel 1 (
  echo Server build failed.
  popd
  pause
  exit /b 1
)
popd

:: ─── Region selection ─────────────────────────────────────────────────────────
:: The picker prints the chosen data directory to stdout.
:: Passing --prompt to always show the selection menu on this run.
echo.
echo Selecting map region...
for /f "usebackq delims=" %%D in (`""%PICKER_EXE%" --prompt --map-root "%SERVER_DIR%\data\map" --cache "%SERVER_DIR%\data\geofabrik-index.json""`) do set "DATA_DIR=%%D"

if "%DATA_DIR%"=="" (
  echo.
  echo Region picker did not return a data directory.
  pause
  exit /b 1
)

echo.
echo Using region: %DATA_DIR%

:: ─── Launch server and client ─────────────────────────────────────────────────
echo Starting API server in a new window...
start "Navigation Server" cmd /k "cd /d "%SERVER_DIR%" && set GOCACHE=%GOCACHE_DIR% && set CGO_ENABLED=0 && "%SERVER_EXE%" --addr :8080 --data "%DATA_DIR%""

echo Starting static client in a new window...
start "Navigation Client" cmd /k "cd /d "%CLIENT_DIR%" && python -m http.server 3000"

echo Waiting for the app to come up...
timeout /t 3 /nobreak >nul

echo Opening browser...
start "" "%CLIENT_URL%"

echo My job here is done.
exit
