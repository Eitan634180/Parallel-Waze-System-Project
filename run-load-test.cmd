@echo off
setlocal

set "ROOT=%~dp0"
set "TESTS_DIR=%ROOT%test"
set "LOADBOT_DIR=%TESTS_DIR%\performance\loadbot"
set "CLIENTS=20"

if not exist "%TESTS_DIR%\package.json" (
  echo Loadbot was not found in "%LOADBOT_DIR%".
  pause
  exit /b 1
)

where node >nul 2>nul
if errorlevel 1 (
  echo Node.js was not found in PATH.
  pause
  exit /b 1
)

where npm.cmd >nul 2>nul
if errorlevel 1 (
  echo npm.cmd was not found in PATH.
  pause
  exit /b 1
)

echo.
set /p CLIENTS=How many browser clients should run? [default: %CLIENTS%]: 
if "%CLIENTS%"=="" set "CLIENTS=%CLIENTS%"

echo.
echo Starting browser load test with %CLIENTS% clients
echo.

pushd "%TESTS_DIR%"

if not exist "%TESTS_DIR%\node_modules\playwright" (
  echo Installing loadbot dependencies...
  call npm.cmd install
  if errorlevel 1 (
    echo npm install failed.
    popd
    pause
    exit /b 1
  )
)

call node performance\loadbot\index.mjs --clients %CLIENTS%
set "EXIT_CODE=%ERRORLEVEL%"

popd

if not "%EXIT_CODE%"=="0" (
  echo.
  echo Load test exited with code %EXIT_CODE%.
  pause
  exit /b %EXIT_CODE%
)

echo.
echo Load test finished.
