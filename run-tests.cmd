@echo off
setlocal EnableExtensions

set "ROOT=%~dp0"
call "%ROOT%load-env.cmd" "%ROOT%project.env.test"
if errorlevel 1 exit /b %ERRORLEVEL%

set "SERVER_DIR=%ROOT%server"
set "CLIENT_DIR=%ROOT%client"
set "TESTS_DIR=%ROOT%test"
set "SERVER_CACHE_DIR=%SERVER_DIR%\.gocache"
set "MAP_ROOT=%SERVER_DIR%\data\map"
set "TARGET=%~1"

if "%TARGET%"=="" set "TARGET=all"

if /I "%TARGET%"=="help" goto :help
if /I "%TARGET%"=="server" goto :server
if /I "%TARGET%"=="server-race" goto :serverrace
if /I "%TARGET%"=="client" goto :client
if /I "%TARGET%"=="end2end" goto :endtoend
if /I "%TARGET%"=="all" goto :all

echo Unknown target: %TARGET%
echo.
goto :help

:server
if not exist "%SERVER_CACHE_DIR%" mkdir "%SERVER_CACHE_DIR%"
pushd "%SERVER_DIR%"
set "CGO_ENABLED=0"
set "GOCACHE=%SERVER_CACHE_DIR%"
go test ./test/...
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:serverrace
if not exist "%SERVER_CACHE_DIR%" mkdir "%SERVER_CACHE_DIR%"
pushd "%SERVER_DIR%"
set "GOCACHE=%SERVER_CACHE_DIR%"
go test -race ./test/...
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:client
pushd "%CLIENT_DIR%"
call npm.cmd run test
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:eedeps
if exist "%TESTS_DIR%\node_modules\playwright\cli.js" exit /b 0
pushd "%TESTS_DIR%"
echo Installing end-to-end test dependencies...
call npm.cmd install
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:resolve_map_path
if "%~1"=="" exit /b 1
set "REL_PATH=%~1"
if not "%REL_PATH::=%"=="%REL_PATH%" (
  echo %~2 in project.env.test must be relative to server\data\map.
  exit /b 1
)
if "%REL_PATH:~0,1%"=="\" (
  echo %~2 in project.env.test must be relative to server\data\map.
  exit /b 1
)
if "%REL_PATH:~0,1%"=="/" (
  echo %~2 in project.env.test must be relative to server\data\map.
  exit /b 1
)
set "%~3=%MAP_ROOT%\%REL_PATH%"
set "REL_PATH="
exit /b 0

:endtoend
  if not defined TEST_REGION_DIR (
  echo TEST_REGION_DIR was not set in project.env.test.
  exit /b 1
)
call :resolve_map_path "%TEST_REGION_DIR%" TEST_REGION_DIR RESOLVED_TEST_REGION_DIR
if errorlevel 1 exit /b %ERRORLEVEL%
if not exist "%RESOLVED_TEST_REGION_DIR%\nodes.bin" (
  echo End-to-end region files not found under: %RESOLVED_TEST_REGION_DIR%
  exit /b 1
)
call :eedeps
if errorlevel 1 exit /b %ERRORLEVEL%
pushd "%TESTS_DIR%"
call npm.cmd run test:end2end
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:all
call "%~f0" server
if errorlevel 1 exit /b %ERRORLEVEL%

call "%~f0" server-race
if errorlevel 1 exit /b %ERRORLEVEL%

call "%~f0" client
if errorlevel 1 exit /b %ERRORLEVEL%

call "%~f0" end2end
if errorlevel 1 exit /b %ERRORLEVEL%

exit /b 0

:help
echo Usage:
echo   run-tests.cmd [target]
echo.
echo Targets:
echo   all          Run server, server-race, client and end2end suites
echo   server       Run all Go server tests under server\test
echo   server-race  Run all Go server tests under server\test with -race
echo   client       Run all client tests under client\test
echo   end2end      Run Playwright smoke tests
echo   help         Show this help
exit /b 1
