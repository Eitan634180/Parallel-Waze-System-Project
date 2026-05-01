@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..") do set "ROOT=%%~fI\"
set "ENV_FILE=%SCRIPT_DIR%project.env.test"
if not exist "%ENV_FILE%" (echo Env file not found: %ENV_FILE% & exit /b 1)
for /f "usebackq eol=# tokens=1* delims==" %%A in ("%ENV_FILE%") do if not "%%~A"=="" set "%%~A=%%~B"

set "SERVER_DIR=%ROOT%server"
set "BIN_DIR=%ROOT%bin"
set "OUT_DIR=%ROOT%.benchmarks"
set "BUILD_WORK_DIR=%ROOT%tmp\bench-build"
set "SERVER_EXE=%BIN_DIR%\server-bench.exe"
set "BENCHMARK_EXE=%BIN_DIR%\benchmark.exe"
set "BUILDER_EXE=%BIN_DIR%\map-builder-bench.exe"
set "SERVER_SCALE_SCRIPT=%SCRIPT_DIR%server-scale.ps1"

for %%D in ("%BIN_DIR%" "%OUT_DIR%" "%OUT_DIR%\server-scale" "%OUT_DIR%\build-scale" "%OUT_DIR%\overlay-scale" "%OUT_DIR%\customization-scale" "%BUILD_WORK_DIR%") do if not exist "%%~D" mkdir "%%~D"

:menu
echo.
echo ====================================================
echo               Select a Benchmark Target
echo ====================================================
echo   1. all                 - Run every benchmark target
echo   2. compare-static      - Run hierarchical vs base
echo   3. server-scale        - Benchmark /route throughput
echo   4. build-scale         - Benchmark map-builder
echo   5. overlay-scale       - Benchmark BuildOverlayGraph
echo   6. customization-scale - Benchmark customization
echo ====================================================
echo.
choice /C 123456 /N /M "Enter your choice (1-6): "

if errorlevel 6 goto :customization-scale
if errorlevel 5 goto :overlay-scale
if errorlevel 4 goto :build-scale
if errorlevel 3 goto :server-scale
if errorlevel 2 goto :compare-static
if errorlevel 1 goto :all

:ready
if "%~1"=="" exit /b 1
for %%F in (nodes edges base_adj cells overlay_adj) do if not exist "%~1\%%F.bin" exit /b 1
exit /b 0

:build_tools
where go >nul 2>nul || (echo Go was not found in PATH. & exit /b 1)
pushd "%SERVER_DIR%"
set "CGO_ENABLED=1"
go build -buildvcs=false -o "%SERVER_EXE%" .\cmd\server || (popd & exit /b 1)
go build -buildvcs=false -o "%BUILDER_EXE%" .\cmd\map-builder || (popd & exit /b 1)
go build -buildvcs=false -o "%BENCHMARK_EXE%" .\cmd\benchmark || (popd & exit /b 1)
popd & exit /b 0

:compare-static
call :build_tools || exit /b 1
pushd "%SERVER_DIR%"
set "CGO_ENABLED=1"
go test ./test/benchmark -run TestBenchmarkCorpusMatchesBaseAStarStatic -bench BenchmarkRouterCompareStatic -benchmem -count %TEST_BENCH_REPEAT_COUNT% -timeout %TEST_BENCH_GO_TEST_TIMEOUT% > "%OUT_DIR%\compare-static.txt" 2>&1
set "ERR=!ERRORLEVEL!" & popd
if "!ERR!"=="0" "%BENCHMARK_EXE%" report --dir "%OUT_DIR%"
exit /b !ERR!

:server-scale
call :build_tools || exit /b 1
for %%P in (%TEST_BENCH_GOMAXPROCS%) do (
  echo Running server-scale for mode=%TEST_BENCH_ROUTING_MODE% GOMAXPROCS=%%P
  powershell -NoProfile -ExecutionPolicy Bypass -File "%SERVER_SCALE_SCRIPT%" -ServerExe "%SERVER_EXE%" -ServerWorkdir "%SERVER_DIR%" -GOMAXPROCS %%P -StartupWaitSec %TEST_BENCH_SERVER_STARTUP_WAIT_SEC% -BenchmarkExe "%BENCHMARK_EXE%" -Out "%OUT_DIR%\server-scale\%TEST_BENCH_ROUTING_MODE%-p%%P" || exit /b 1
)
"%BENCHMARK_EXE%" report --dir "%OUT_DIR%"
exit /b %ERRORLEVEL%

:build-scale
call :build_tools || exit /b 1
for %%P in (%TEST_BENCH_GOMAXPROCS%) do (
  echo Running build-scale for GOMAXPROCS=%%P
  pushd "%SERVER_DIR%"
  set "CGO_ENABLED=1" & set "GOMAXPROCS=%%P"
  "%BUILDER_EXE%" --out "%BUILD_WORK_DIR%\p%%P" > "%OUT_DIR%\build-scale\p%%P.log" 2>&1
  set "ERR=!ERRORLEVEL!" & popd
  if not "!ERR!"=="0" exit /b !ERR!
)
"%BENCHMARK_EXE%" report --dir "%OUT_DIR%"
exit /b %ERRORLEVEL%

:overlay-scale
call :build_tools || exit /b 1
echo Running overlay-scale for workers=%TEST_BENCH_GOMAXPROCS%
pushd "%SERVER_DIR%"
set "CGO_ENABLED=1"
"%BENCHMARK_EXE%" overlay-build > "%OUT_DIR%\overlay-scale\summary.log" 2>&1
set "ERR=!ERRORLEVEL!" & popd
if "!ERR!"=="0" "%BENCHMARK_EXE%" report --dir "%OUT_DIR%"
exit /b !ERR!

:customization-scale
call :build_tools || exit /b 1
echo Running customization-scale for workers=%TEST_BENCH_GOMAXPROCS%
pushd "%SERVER_DIR%"
set "CGO_ENABLED=1"
"%BENCHMARK_EXE%" overlay-customization > "%OUT_DIR%\customization-scale\summary.log" 2>&1
set "ERR=!ERRORLEVEL!" & popd
if "!ERR!"=="0" "%BENCHMARK_EXE%" report --dir "%OUT_DIR%"
exit /b !ERR!

:all
call :compare-static || exit /b 1
call :server-scale || exit /b 1
call :build-scale || exit /b 1
call :overlay-scale || exit /b 1
call :customization-scale || exit /b 1
exit /b 0
