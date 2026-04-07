@echo off
setlocal

set "ROOT=%~dp0"
set "SERVER_DIR=%ROOT%server"
set "CLIENT_DIR=%ROOT%client"
set "TESTS_DIR=%ROOT%test"
set "SERVER_CACHE_DIR=%SERVER_DIR%\.gocache"
set "TARGET=%~1"

if "%TARGET%"=="" set "TARGET=all"

if /I "%TARGET%"=="help" goto :help
if /I "%TARGET%"=="server" goto :server
if /I "%TARGET%"=="server-race" goto :server_race
if /I "%TARGET%"=="client" goto :client
if /I "%TARGET%"=="end2end" goto :end2end
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

:server_race
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

:ensure_end2end_deps
if exist "%TESTS_DIR%\node_modules\playwright\cli.js" exit /b 0
pushd "%TESTS_DIR%"
echo Installing end-to-end test dependencies...
call npm.cmd install
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:end2end
call :ensure_end2end_deps
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

exit /b 0

:help
echo Usage:
echo   run-tests.cmd [target]
echo.
echo Targets:
echo   all          Run server, server-race, and client suites
echo   server       Run all Go server tests under server\test
echo   server-race  Run all Go server tests under server\test with -race
echo   client       Run all client tests under client\test
echo   end2end      Run Playwright smoke tests
echo   help         Show this help
exit /b 1
