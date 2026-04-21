@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "ROOT=%~dp0"
call "%ROOT%load-env.cmd" "%ROOT%project.env.development"
if errorlevel 1 (
  pause
  exit /b %ERRORLEVEL%
)

set "SERVER_DIR=%ROOT%server"
set "CLIENT_DIR=%ROOT%client"
set "CACHE_DIR=%ROOT%.cache"
set "GOCACHE_DIR=%CACHE_DIR%\go-build"
set "BIN_DIR=%CACHE_DIR%\bin"
set "SERVER_EXE=%BIN_DIR%\server.exe"
set "PICKER_EXE=%BIN_DIR%\region-picker.exe"
set "BUILDER_EXE=%BIN_DIR%\map-builder.exe"
set "CLIENT_URL=http://localhost:3000/public/index.html"

if not defined DEV_ROUTING_MODE set "DEV_ROUTING_MODE=hierarchical"

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

if defined DEV_REGION_DIR (
  set "DATA_DIR=%DEV_REGION_DIR%"
  call :ready "%DATA_DIR%"
  if errorlevel 1 (
    echo Development region files not found under: %DATA_DIR%
    echo Update DEV_REGION_DIR in project.env.development or leave it blank to use the picker.
    pause
    exit /b 1
  )
) else (
  set "DATA_DIR="
  echo.
  echo Selecting map region...
  for /f "usebackq delims=" %%D in (`""%PICKER_EXE%" --prompt --map-root "%SERVER_DIR%\data\map" --cache "%SERVER_DIR%\data\geofabrik-index.json""`) do set "DATA_DIR=%%D"

  if "!DATA_DIR!"=="" (
    echo.
    echo Region picker did not return a data directory.
    pause
    exit /b 1
  )
)

echo.
echo Using region: %DATA_DIR%

echo Starting API server in a new window...
start "Navigation Server" cmd /k "cd /d "%SERVER_DIR%" && set GOCACHE=%GOCACHE_DIR% && set CGO_ENABLED=0 && "%SERVER_EXE%" --addr :8080 --data "%DATA_DIR%" --routing-mode "%DEV_ROUTING_MODE%""

echo Starting static client in a new window...
start "Navigation Client" cmd /k "cd /d "%CLIENT_DIR%" && python -m http.server 3000"

echo Waiting for the app to come up...
timeout /t 3 /nobreak >nul

echo Opening browser...
start "" "%CLIENT_URL%"

echo My job here is done.
exit

:ready
set "CHECK_DIR=%~1"
if not exist "%CHECK_DIR%\nodes.bin" exit /b 1
if not exist "%CHECK_DIR%\edges.bin" exit /b 1
if not exist "%CHECK_DIR%\base_adj.bin" exit /b 1
if not exist "%CHECK_DIR%\cells.bin" exit /b 1
if not exist "%CHECK_DIR%\boundary.bin" exit /b 1
if not exist "%CHECK_DIR%\overlay_adj.bin" exit /b 1
exit /b 0
