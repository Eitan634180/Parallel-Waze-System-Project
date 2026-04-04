@echo off
setlocal

set "ROOT=%~dp0"
set "SERVER_DIR=%ROOT%server"
set "CLIENT_DIR=%ROOT%client"
set "DATA_DIR=%SERVER_DIR%\data\map"
set "CACHE_DIR=%ROOT%.cache"
set "GOCACHE_DIR=%CACHE_DIR%\go-build"
set "BIN_DIR=%CACHE_DIR%\bin"
set "SERVER_EXE=%BIN_DIR%\server.exe"
set "PBF_FILE=%DATA_DIR%\israel-latest.osm.pbf"
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

set "MAP_READY=1"
if not exist "%DATA_DIR%\nodes.bin" set "MAP_READY=0"
if not exist "%DATA_DIR%\edges.bin" set "MAP_READY=0"
if not exist "%DATA_DIR%\base_adj.bin" set "MAP_READY=0"
if not exist "%DATA_DIR%\cells.bin" set "MAP_READY=0"
if not exist "%DATA_DIR%\boundary.bin" set "MAP_READY=0"
if not exist "%DATA_DIR%\overlay_adj.bin" set "MAP_READY=0"

if "%MAP_READY%"=="0" (
  if not exist "%PBF_FILE%" (
    echo Graph data is missing and the source PBF was not found:
    echo   %PBF_FILE%
    echo.
    echo Put the PBF in server\data\map or build the graph manually.
    pause
    exit /b 1
  )

  echo Graph data was not found. Building map files...
  pushd "%SERVER_DIR%"
  set "GOCACHE=%GOCACHE_DIR%"
  set "CGO_ENABLED=0"
  go run .\cmd\map-builder --pbf .\data\map\israel-latest.osm.pbf --out .\data\map
  if errorlevel 1 (
    echo.
    echo Map build failed.
    popd
    pause
    exit /b 1
  )
  popd
)

echo Building Go server...
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
go build -o "%SERVER_EXE%" .\cmd\server
if errorlevel 1 (
  echo.
  echo Server build failed.
  popd
  pause
  exit /b 1
)
popd

echo Starting API server in a new window...
start "Navigation Server" cmd /k "cd /d "%SERVER_DIR%" && set GOCACHE=%GOCACHE_DIR% && set CGO_ENABLED=0 && "%SERVER_EXE%" --addr :8080"

echo Starting static client in a new window...
start "Navigation Client" cmd /k "cd /d "%CLIENT_DIR%" && python -m http.server 3000"

echo Waiting for the app to come up...
timeout /t 3 /nobreak >nul

echo Opening browser...
start "" "%CLIENT_URL%"

echo My job here is done.
exit
