@echo off
setlocal EnableExtensions

set "ROOT=%~dp0"
if not exist "%ROOT%project.env.test" (echo Env file not found: %ROOT%project.env.test & exit /b 1)
for /f "usebackq eol=# tokens=1* delims==" %%A in ("%ROOT%project.env.test") do if not "%%~A"=="" set "%%~A=%%~B"

set "SERVER_DIR=%ROOT%server"
set "CLIENT_DIR=%ROOT%client"
set "TESTS_DIR=%ROOT%test"

if not defined NAV_MAP_ROOT echo NAV_MAP_ROOT must be set in project.env.test. & exit /b 1
call :resolve_server_path "%NAV_MAP_ROOT%" MAP_ROOT
if errorlevel 1 exit /b %ERRORLEVEL%

:menu
echo.
echo =======================================
echo          Select a Test Target
echo =======================================
echo   1. all         - Run server, server-race, client and end2end
echo   2. server      - Run Go server package tests
echo   3. server-race - Run race-enabled Go tests
echo   4. client      - Run client tests
echo   5. end2end     - Run Playwright smoke tests
echo   6. loadbot     - Run browser load tests
echo =======================================
echo.
choice /C 123456 /N /M "Enter your choice (1-6): "

if errorlevel 6 goto :loadbot
if errorlevel 5 goto :end2end
if errorlevel 4 goto :client
if errorlevel 3 goto :server-race
if errorlevel 2 goto :server
if errorlevel 1 goto :all

:server
set "GO_ARGS=./..."
set "CGO_ENABLED=1"
goto :run_go

:server-race
set "CGO_ENABLED=1"
set "GO_ARGS=-race ./src/graph/... ./src/navigation/... ./src/routing/... ./src/simulation/... ./src/traffic/... ./src/transport/... ./src/utilities/... ./test/component ./test/correctness ./test/external ./test/integration ./test/unit"

:run_go
pushd "%SERVER_DIR%"
go test %GO_ARGS%
set "ERR=%ERRORLEVEL%"
popd
exit /b %ERR%

:client
pushd "%CLIENT_DIR%"
call npm.cmd run test
set "ERR=%ERRORLEVEL%"
popd
exit /b %ERR%

:eedeps
if exist "%TESTS_DIR%\node_modules\playwright\cli.js" exit /b 0
echo Installing end-to-end test dependencies...
pushd "%TESTS_DIR%"
call npm.cmd install
set "ERR=%ERRORLEVEL%"
popd
exit /b %ERR%

:end2end
if not defined TEST_REGION_DIR echo TEST_REGION_DIR was not set in project.env.test. & exit /b 1
call :resolve_map_path "%TEST_REGION_DIR%" TEST_REGION_DIR RESOLVED_TEST_REGION_DIR
if errorlevel 1 exit /b %ERRORLEVEL%

if not exist "%RESOLVED_TEST_REGION_DIR%\nodes.bin" echo End-to-end region files not found under: %RESOLVED_TEST_REGION_DIR% & exit /b 1

call :eedeps
if errorlevel 1 exit /b %ERRORLEVEL%

pushd "%TESTS_DIR%"
call npm.cmd run test:end2end
set "ERR=%ERRORLEVEL%"
popd
exit /b %ERR%

:loadbot
set "LOADBOT_DIR=%TESTS_DIR%\performance\loadbot"
if not exist "%TESTS_DIR%\package.json" echo Loadbot was not found in "%LOADBOT_DIR%". & exit /b 1
where node >nul 2>nul || (echo Node.js was not found in PATH. & exit /b 1)

set "CLIENTS=20"
echo. & set /p CLIENTS=How many browser clients should run? [default: 20]: 
if "%CLIENTS%"=="" set "CLIENTS=20"

echo. & echo Starting browser load test with %CLIENTS% clients & echo.

call :eedeps
if errorlevel 1 exit /b %ERRORLEVEL%

pushd "%TESTS_DIR%"
call node performance\loadbot\index.mjs --clients %CLIENTS%
set "ERR=%ERRORLEVEL%"
popd

if "%ERR%"=="0" (echo. & echo Load test finished.) else (echo. & echo Load test exited with code %ERR%.)
exit /b %ERR%

:all
call :server || exit /b 1
call :server-race || exit /b 1
call :client || exit /b 1
call :end2end || exit /b 1
exit /b 0

:resolve_map_path
set "P=%~1"
if "%P%"=="" exit /b 1
if not "%P::=%"=="%P%" echo %~2 in project.env.test must be relative to NAV_MAP_ROOT. & exit /b 1
if "%P:~0,1%"=="\" echo %~2 in project.env.test must be relative to NAV_MAP_ROOT. & exit /b 1
if "%P:~0,1%"=="/" echo %~2 in project.env.test must be relative to NAV_MAP_ROOT. & exit /b 1
set "%~3=%MAP_ROOT%\%P%"
exit /b 0

:resolve_server_path
set "P=%~1"
if "%P%"=="" exit /b 1
if not "%P::=%"=="%P%" ( set "%~2=%P%" ) else if "%P:~0,1%"=="\" ( set "%~2=%P%" ) else if "%P:~0,1%"=="/" ( set "%~2=%P%" ) else ( set "%~2=%SERVER_DIR%\%P%" )
exit /b 0