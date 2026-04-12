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
set "BENCH_CASES_DIR=%SERVER_DIR%\test\testdata\benchmark-cases"
set "TARGET=%~1"
set "SERVER_EXE=%BIN_DIR%\server-bench.exe"
set "LOADBENCH_EXE=%BIN_DIR%\route-loadbench.exe"
set "BUILDER_EXE=%BIN_DIR%\map-builder-bench.exe"
set "REPORT_EXE=%BIN_DIR%\benchmark-report.exe"
set "SERVER_ADDR=%TEST_HOST%:%TEST_BENCH_SERVER_PORT%"
set "SERVER_URL=http://%TEST_HOST%:%TEST_BENCH_SERVER_PORT%"

if "%TARGET%"=="" set "TARGET=all"

if not exist "%CACHE_DIR%" mkdir "%CACHE_DIR%"
if not exist "%GOCACHE_DIR%" mkdir "%GOCACHE_DIR%"
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"
if not exist "%OUT_DIR%\server-scale" mkdir "%OUT_DIR%\server-scale"
if not exist "%OUT_DIR%\build-scale" mkdir "%OUT_DIR%\build-scale"
if not exist "%BUILD_WORK_DIR%" mkdir "%BUILD_WORK_DIR%"

if /I "%TARGET%"=="compare-static" goto :cmp
if /I "%TARGET%"=="server-scale" goto :server_scale
if /I "%TARGET%"=="build-scale" goto :build_scale
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

:rdir
if not defined TEST_REGION_DIR (
  echo TEST_REGION_DIR was not set in project.env.test.
  exit /b 1
)
set "SERVER_DATA=%TEST_REGION_DIR%"
call :ready "%SERVER_DATA%"
if errorlevel 1 (
  echo Benchmark region files not found under: %SERVER_DATA%
  exit /b 1
)
exit /b 0

:bcorp
if not defined TEST_BENCH_CORPUS (
  echo TEST_BENCH_CORPUS was not set in project.env.test.
  exit /b 1
)
if not exist "%BENCH_CASES_DIR%\%TEST_BENCH_CORPUS%" (
  echo Benchmark corpus file not found: %BENCH_CASES_DIR%\%TEST_BENCH_CORPUS%
  exit /b 1
)
exit /b 0

:bsinp
call :rdir
if errorlevel 1 exit /b %ERRORLEVEL%
if not defined TEST_BENCH_PBF_PATH (
  echo TEST_BENCH_PBF_PATH was not set in project.env.test.
  exit /b 1
)
set "REGION_PBF=%TEST_BENCH_PBF_PATH%"
if not defined REGION_PBF (
  echo TEST_BENCH_PBF_PATH was not set in project.env.test.
  exit /b 1
)
if not exist "%REGION_PBF%" (
  echo Benchmark PBF file not found: %REGION_PBF%
  exit /b 1
)
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
go build -buildvcs=false -o "%LOADBENCH_EXE%" .\cmd\route-loadbench
if errorlevel 1 (
  popd
  exit /b %ERRORLEVEL%
)
go build -buildvcs=false -o "%BUILDER_EXE%" .\cmd\map-builder
if errorlevel 1 (
  popd
  exit /b %ERRORLEVEL%
)
go build -buildvcs=false -o "%REPORT_EXE%" .\cmd\benchmark-report
set "EXIT_CODE=%ERRORLEVEL%"
popd
exit /b %EXIT_CODE%

:write_report
"%REPORT_EXE%" --dir "%OUT_DIR%"
exit /b %ERRORLEVEL%

:cmp
call :rdir
if errorlevel 1 exit /b %ERRORLEVEL%
call :bcorp
if errorlevel 1 exit /b %ERRORLEVEL%
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
go test ./test/benchmark -run TestBenchmarkCorpusMatchesBaseAStarStatic -bench BenchmarkRouterCompareStatic -benchmem -count %TEST_BENCH_REPEAT_COUNT% > "%OUT_DIR%\compare-static.txt" 2>&1
set "EXIT_CODE=%ERRORLEVEL%"
popd
if not "%EXIT_CODE%"=="0" exit /b %EXIT_CODE%
call :write_report
exit /b %ERRORLEVEL%

:server_scale
call :rdir
if errorlevel 1 exit /b %ERRORLEVEL%
call :bcorp
if errorlevel 1 exit /b %ERRORLEVEL%
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
for %%P in (%TEST_BENCH_GOMAXPROCS%) do (
  echo Running server-scale for mode=%TEST_BENCH_ROUTING_MODE% GOMAXPROCS=%%P
  powershell -NoProfile -Command ^
    "$ErrorActionPreference = 'Stop';" ^
    "$env:GOMAXPROCS='%%P'; $env:GOCACHE='%GOCACHE_DIR%'; $env:CGO_ENABLED='0';" ^
    "$proc = Start-Process -FilePath '%SERVER_EXE%' -ArgumentList @('--addr','%SERVER_ADDR%','--data','%SERVER_DATA%','--routing-mode','%TEST_BENCH_ROUTING_MODE%') -WorkingDirectory '%SERVER_DIR%' -PassThru;" ^
    "Start-Sleep -Seconds %TEST_BENCH_SERVER_STARTUP_WAIT_SEC%;" ^
    "$benchExit = 0;" ^
    "try { & '%LOADBENCH_EXE%' '--server' '%SERVER_URL%' '--cases' '%TEST_BENCH_CORPUS%' '--concurrency' '%TEST_BENCH_CONCURRENCY%' '--requests' '%TEST_BENCH_REQUESTS%' '--warmup' '%TEST_BENCH_WARMUP%' '--routing-mode' '%TEST_BENCH_ROUTING_MODE%' '--out' '%OUT_DIR%\server-scale\%TEST_BENCH_ROUTING_MODE%-p%%P'; $benchExit = $LASTEXITCODE } finally { if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force } }" ^
    "exit $benchExit"
  if errorlevel 1 exit /b %ERRORLEVEL%
)
call :write_report
exit /b %ERRORLEVEL%

:build_scale
call :bsinp
if errorlevel 1 exit /b %ERRORLEVEL%
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
for %%P in (%TEST_BENCH_GOMAXPROCS%) do (
  echo Running build-scale for workers=%%P
  pushd "%SERVER_DIR%"
  set "GOCACHE=%GOCACHE_DIR%"
  set "CGO_ENABLED=0"
  set "GOMAXPROCS=%%P"
  "%BUILDER_EXE%" --pbf "%REGION_PBF%" --out "%BUILD_WORK_DIR%\p%%P" --cell-size %TEST_BENCH_CELL_SIZE% --workers %%P > "%OUT_DIR%\build-scale\p%%P.log" 2>&1
  set "EXIT_CODE=!ERRORLEVEL!"
  popd
  if not "!EXIT_CODE!"=="0" exit /b !EXIT_CODE!
)
call :write_report
exit /b %ERRORLEVEL%

:all
call "%~f0" compare-static
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" server-scale
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" build-scale
if errorlevel 1 exit /b %ERRORLEVEL%
exit /b 0

:help
echo Usage:
echo   run-benchmarks.cmd [target]
echo.
echo Targets:
echo   compare-static  Run hierarchical vs base-astar/base-dijkstra static benchmarks
echo   server-scale    Benchmark /route throughput as GOMAXPROCS increases
echo   build-scale     Benchmark map-builder overlay construction as workers increases
echo   all             Run every benchmark target
echo   help            Show this help
echo.
echo Reports are written under .\.benchmarks
exit /b 1
