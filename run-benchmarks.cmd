@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "ROOT=%~dp0"
call "%ROOT%load-env.cmd" "%ROOT%project.env.test"
if errorlevel 1 exit /b %ERRORLEVEL%

set "SERVER_DIR=%ROOT%server"
set "CACHE_DIR=%ROOT%.cache"
set "GOCACHE_DIR=%CACHE_DIR%\go-bench"
set "BIN_DIR=%CACHE_DIR%\bin"
set "OUT_DIR=%ROOT%.benchmarks"
set "BUILD_WORK_DIR=%CACHE_DIR%\bench-build"
set "TARGET=%~1"
set "SERVER_EXE=%BIN_DIR%\server-bench.exe"
set "BENCHMARK_EXE=%BIN_DIR%\benchmark.exe"
set "BUILDER_EXE=%BIN_DIR%\map-builder-bench.exe"
set "SERVER_SCALE_SCRIPT=%ROOT%server-scale.ps1"

if "%TARGET%"=="" set "TARGET=all"

if not exist "%CACHE_DIR%" mkdir "%CACHE_DIR%"
if not exist "%GOCACHE_DIR%" mkdir "%GOCACHE_DIR%"
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"
if not exist "%OUT_DIR%\server-scale" mkdir "%OUT_DIR%\server-scale"
if not exist "%OUT_DIR%\build-scale" mkdir "%OUT_DIR%\build-scale"
if not exist "%OUT_DIR%\overlay-scale" mkdir "%OUT_DIR%\overlay-scale"
if not exist "%OUT_DIR%\customization-scale" mkdir "%OUT_DIR%\customization-scale"
if not exist "%BUILD_WORK_DIR%" mkdir "%BUILD_WORK_DIR%"

if /I "%TARGET%"=="compare-static" goto :cmp
if /I "%TARGET%"=="server-scale" goto :server_scale
if /I "%TARGET%"=="build-scale" goto :build_scale
if /I "%TARGET%"=="overlay-scale" goto :overlay_scale
if /I "%TARGET%"=="customization-scale" goto :customization_scale
if /I "%TARGET%"=="all" goto :all
if /I "%TARGET%"=="help" goto :help

echo Unknown target: %TARGET%
echo.
goto :help

:ready
set "CHECK_DIR=%~1"
if not defined CHECK_DIR exit /b 1
if not exist "%CHECK_DIR%\nodes.bin" exit /b 1
if not exist "%CHECK_DIR%\edges.bin" exit /b 1
if not exist "%CHECK_DIR%\base_adj.bin" exit /b 1
if not exist "%CHECK_DIR%\cells.bin" exit /b 1
if not exist "%CHECK_DIR%\boundary.bin" exit /b 1
if not exist "%CHECK_DIR%\overlay_adj.bin" exit /b 1
exit /b 0

:ensure_go
where go >nul 2>nul
if errorlevel 1 (
  echo Go was not found in PATH.
  exit /b 1
)
exit /b 0

:build_tools
call :ensure_go
if errorlevel 1 exit /b %ERRORLEVEL%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
go build -buildvcs=false -o "%SERVER_EXE%" .\cmd\server
if errorlevel 1 (
  popd
  exit /b %ERRORLEVEL%
)
go build -buildvcs=false -o "%BUILDER_EXE%" .\cmd\map-builder
if errorlevel 1 (
  popd
  exit /b %ERRORLEVEL%
)
go build -buildvcs=false -o "%BENCHMARK_EXE%" .\cmd\benchmark
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:write_report
"%BENCHMARK_EXE%" report --dir "%OUT_DIR%"
exit /b %ERRORLEVEL%

:cmp
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
go test ./test/benchmark -run TestBenchmarkCorpusMatchesBaseAStarStatic -bench BenchmarkRouterCompareStatic -benchmem -count %TEST_BENCH_REPEAT_COUNT% -timeout %TEST_BENCH_GO_TEST_TIMEOUT% > "%OUT_DIR%\compare-static.txt" 2>&1
set "EXIT_CODE=%ERRORLEVEL%"
popd
if not "%EXIT_CODE%"=="0" exit /b %EXIT_CODE%
call :write_report
exit /b %ERRORLEVEL%

:server_scale
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
for %%P in (%TEST_BENCH_GOMAXPROCS%) do (
  echo Running server-scale for mode=%TEST_BENCH_ROUTING_MODE% GOMAXPROCS=%%P
  powershell -NoProfile -ExecutionPolicy Bypass -File "%SERVER_SCALE_SCRIPT%" -ServerExe "%SERVER_EXE%" -ServerWorkdir "%SERVER_DIR%" -GOMAXPROCS %%P -StartupWaitSec %TEST_BENCH_SERVER_STARTUP_WAIT_SEC% -BenchmarkExe "%BENCHMARK_EXE%" -Out "%OUT_DIR%\server-scale\%TEST_BENCH_ROUTING_MODE%-p%%P"
  if errorlevel 1 exit /b !ERRORLEVEL!
)
call :write_report
exit /b %ERRORLEVEL%

:build_scale
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
for %%P in (%TEST_BENCH_GOMAXPROCS%) do (
  echo Running build-scale for GOMAXPROCS=%%P
  pushd "%SERVER_DIR%"
  set "GOCACHE=%GOCACHE_DIR%"
  set "CGO_ENABLED=0"
  set "GOMAXPROCS=%%P"
  "%BUILDER_EXE%" --out "%BUILD_WORK_DIR%\p%%P" > "%OUT_DIR%\build-scale\p%%P.log" 2>&1
  set "EXIT_CODE=!ERRORLEVEL!"
  popd
  if not "!EXIT_CODE!"=="0" exit /b !EXIT_CODE!
)
call :write_report
exit /b %ERRORLEVEL%

:overlay_scale
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
echo Running overlay-scale for workers=%TEST_BENCH_GOMAXPROCS%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
"%BENCHMARK_EXE%" overlay-build > "%OUT_DIR%\overlay-scale\summary.log" 2>&1
set "EXIT_CODE=!ERRORLEVEL!"
popd
if not "%EXIT_CODE%"=="0" exit /b %EXIT_CODE%
call :write_report
exit /b %ERRORLEVEL%

:customization_scale
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
echo Running customization-scale for workers=%TEST_BENCH_GOMAXPROCS%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
"%BENCHMARK_EXE%" overlay-customization > "%OUT_DIR%\customization-scale\summary.log" 2>&1
set "EXIT_CODE=!ERRORLEVEL!"
popd
if not "%EXIT_CODE%"=="0" exit /b %EXIT_CODE%
call :write_report
exit /b %ERRORLEVEL%

:all
call "%~f0" compare-static
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" server-scale
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" build-scale
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" overlay-scale
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" customization-scale
if errorlevel 1 exit /b %ERRORLEVEL%
exit /b 0

:help
echo Usage:
echo   run-benchmarks.cmd [target]
echo.
echo Targets:
echo   compare-static  Run hierarchical vs base-astar/base-dijkstra static benchmarks
echo   server-scale    Benchmark /route throughput as GOMAXPROCS increases
echo   build-scale     Benchmark map-builder throughput as GOMAXPROCS increases
echo   overlay-scale   Benchmark BuildOverlayGraph over a saved graph as GOMAXPROCS increases
echo   customization-scale Benchmark overlay customization throughput as GOMAXPROCS increases
echo   all             Run every benchmark target
echo   help            Show this help
echo.
echo Reports are written under .\.benchmarks
exit /b 1
