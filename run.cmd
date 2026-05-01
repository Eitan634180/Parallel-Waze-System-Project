@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "ROOT=%~dp0"
set "USE_ISRAEL_DEMO="

:parse_args
if "%~1"=="" goto :args_done
if /I "%~1"=="--israel" (
  set "USE_ISRAEL_DEMO=1"
) else (
  echo Unknown argument: %~1
  echo Usage: run.cmd [--israel]
  pause
  exit /b 1
)
shift
goto :parse_args

:args_done
set "ENV_FILE=%ROOT%scripts\project.env.development"
if not exist "%ENV_FILE%" (echo Env file not found: %ENV_FILE% & pause & exit /b 1)
for /f "usebackq eol=# tokens=1* delims==" %%A in ("%ENV_FILE%") do if not "%%~A"=="" set "%%~A=%%~B"

set "SERVER_DIR=%ROOT%server"
set "CLIENT_DIR=%ROOT%client"
set "BIN_DIR=%ROOT%bin"
set "SERVER_EXE=%BIN_DIR%\server.exe"
set "PICKER_EXE=%BIN_DIR%\region-picker.exe"

for %%V in (NAV_SERVER_ADDR NAV_MAP_ROOT NAV_GEOFABRIK_CACHE_FILE NAV_CLIENT_SERVER_URL DEV_CLIENT_HOST DEV_CLIENT_PORT DEV_CLIENT_ENTRY_PATH DEV_ROUTING_MODE) do (
  if not defined %%V echo %%V must be set in project.env.development. & pause & exit /b 1
)
set "CLIENT_URL=http://%DEV_CLIENT_HOST%:%DEV_CLIENT_PORT%%DEV_CLIENT_ENTRY_PATH%"

for %%X in (go python node) do where %%X >nul 2>nul || (echo %%X was not found in PATH. & pause & exit /b 1)

for %%D in ("%BIN_DIR%") do if not exist "%%~D" mkdir "%%~D"

echo Building Go binaries...
pushd "%SERVER_DIR%"
set "CGO_ENABLED=1"
for %%B in (map-builder region-picker server) do (
  go build -o "%BIN_DIR%\%%B.exe" .\cmd\%%B || (echo %%B build failed. & popd & pause & exit /b 1)
)
popd

echo Writing client runtime config...
node "%CLIENT_DIR%\scripts\runtime-config.mjs" || (echo Client runtime config generation failed. & pause & exit /b 1)

if defined USE_ISRAEL_DEMO (
  echo.
  echo Preparing Israel demo map...
  set "DATA_DIR="
  for /f "delims=" %%D in ('"%PICKER_EXE%" --region israel-and-palestine --keep-pbf=false') do set "DATA_DIR=%%D"
  
  if "!DATA_DIR!"=="" (
    echo.
    echo Israel demo map setup failed.
    pause
    exit /b 1
  )
  set "NAV_SERVER_DATA_DIR=!DATA_DIR!"
)

if not defined NAV_SERVER_DATA_DIR if not defined DEV_REGION_DIR (
  echo.
  echo Selecting map region...
  set "DATA_DIR="
  for /f "delims=" %%D in ('"%PICKER_EXE%" --prompt') do set "DATA_DIR=%%D"
  
  if "!DATA_DIR!"=="" (
    echo.
    echo Region picker did not return a data directory.
    pause
    exit /b 1
  )
  set "NAV_SERVER_DATA_DIR=!DATA_DIR!"
)

echo Starting API server in a new window...
start "Navigation Server" cmd /k "cd /d "%SERVER_DIR%" && set CGO_ENABLED=1 && "%SERVER_EXE%""

echo Starting static client in a new window...
start "Navigation Client" cmd /k "cd /d "%CLIENT_DIR%" && python -m http.server %DEV_CLIENT_PORT%"

echo Waiting for the app to come up...
timeout /t 3 /nobreak >nul

echo Opening browser...
start "" "%CLIENT_URL%"

echo My job here is done.
exit

:ready
if "%~1"=="" exit /b 1
for %%F in (nodes edges base_adj cells overlay_adj) do if not exist "%~1\%%F.bin" exit /b 1
exit /b 0
