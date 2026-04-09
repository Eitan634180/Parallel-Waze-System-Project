@echo off
setlocal EnableDelayedExpansion

set "ROOT=%~dp0"
set "SERVER_DIR=%ROOT%server"
set "CACHE_DIR=%ROOT%.cache"
set "GOCACHE_DIR=%CACHE_DIR%\go-bench"
set "BIN_DIR=%CACHE_DIR%\bin"
set "OUT_DIR=%ROOT%.benchmarks"
set "BUILD_WORK_DIR=%CACHE_DIR%\bench-build"
set "TARGET=%~1"
set "SERVER_EXE=%BIN_DIR%\server-bench.exe"
set "LOADBENCH_EXE=%BIN_DIR%\route-loadbench.exe"
set "BUILDER_EXE=%BIN_DIR%\map-builder-bench.exe"
set "REPORT_EXE=%BIN_DIR%\benchmark-report.exe"
set "BENCH_CORPUS=israel-and-palestine-bench.json"
set "BENCH_TRAFFIC=israel-and-palestine-live.json"
set "SERVER_DATA=%SERVER_DIR%\data\map\israel-and-palestine"
set "SERVER_ADDR=:8090"

if "%TARGET%"=="" set "TARGET=all"

if not exist "%CACHE_DIR%" mkdir "%CACHE_DIR%"
if not exist "%GOCACHE_DIR%" mkdir "%GOCACHE_DIR%"
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"
if not exist "%OUT_DIR%\server-scale" mkdir "%OUT_DIR%\server-scale"
if not exist "%OUT_DIR%\build-scale" mkdir "%OUT_DIR%\build-scale"
if not exist "%BUILD_WORK_DIR%" mkdir "%BUILD_WORK_DIR%"

if /I "%TARGET%"=="compare-static" goto :compare_static
if /I "%TARGET%"=="compare-live" goto :compare_live
if /I "%TARGET%"=="customize" goto :customize
if /I "%TARGET%"=="server-scale" goto :server_scale
if /I "%TARGET%"=="build-scale" goto :build_scale
if /I "%TARGET%"=="all" goto :all
if /I "%TARGET%"=="help" goto :help

echo Unknown target: %TARGET%
echo.
goto :help

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

:compare_static
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
set "BENCH_CORPUS=%BENCH_CORPUS%"
go test ./test/benchmark -run TestBenchmarkCorpusMatchesBaseAStarStatic -bench BenchmarkRouterCompareStatic -benchmem -count 3 > "%OUT_DIR%\compare-static.txt" 2>&1
set "EXIT_CODE=%ERRORLEVEL%"
popd
if not "%EXIT_CODE%"=="0" exit /b %EXIT_CODE%
call :write_report
exit /b %ERRORLEVEL%

:compare_live
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
set "BENCH_CORPUS=%BENCH_CORPUS%"
set "BENCH_TRAFFIC=%BENCH_TRAFFIC%"
go test ./test/benchmark -run TestBenchmarkCorpusMatchesBaseAStarLiveTraffic -bench BenchmarkRouterCompareLive -benchmem -count 3 > "%OUT_DIR%\compare-live.txt" 2>&1
set "EXIT_CODE=%ERRORLEVEL%"
popd
if not "%EXIT_CODE%"=="0" exit /b %EXIT_CODE%
call :write_report
exit /b %ERRORLEVEL%

:customize
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
pushd "%SERVER_DIR%"
set "GOCACHE=%GOCACHE_DIR%"
set "CGO_ENABLED=0"
set "BENCH_CORPUS=%BENCH_CORPUS%"
set "BENCH_TRAFFIC=%BENCH_TRAFFIC%"
go test ./test/benchmark -run ^$ -bench "Benchmark(OverlayCustomizationLive|CustomizationAmortizedLive)" -benchmem -count 3 > "%OUT_DIR%\customize.txt" 2>&1
set "EXIT_CODE=%ERRORLEVEL%"
popd
if not "%EXIT_CODE%"=="0" exit /b %EXIT_CODE%
call :write_report
exit /b %ERRORLEVEL%

:server_scale
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
for %%M in (hierarchical base-astar) do (
  for %%P in (1 2 4 8) do (
    echo Running server-scale for mode=%%M GOMAXPROCS=%%P
    powershell -NoProfile -Command ^
      "$env:GOMAXPROCS='%%P'; $env:GOCACHE='%GOCACHE_DIR%'; $env:CGO_ENABLED='0';" ^
      "$proc = Start-Process -FilePath '%SERVER_EXE%' -ArgumentList @('--addr','%SERVER_ADDR%','--data','%SERVER_DATA%','--routing-mode','%%M') -WorkingDirectory '%SERVER_DIR%' -PassThru;" ^
      "Start-Sleep -Seconds 5;" ^
      "try { & '%LOADBENCH_EXE%' '--server' 'http://127.0.0.1:8090' '--cases' '%BENCH_CORPUS%' '--concurrency' '32' '--requests' '256' '--warmup' '64' '--routing-mode' '%%M' '--traffic-profile' 'static' '--out' '%OUT_DIR%\server-scale\%%M-p%%P' } finally { Stop-Process -Id $proc.Id -Force }"
    if errorlevel 1 exit /b %ERRORLEVEL%
  )
)
call :write_report
exit /b %ERRORLEVEL%

:build_scale
call :build_tools
if errorlevel 1 exit /b %ERRORLEVEL%
for %%P in (1 2 4 8) do (
  echo Running build-scale for workers=%%P
  pushd "%SERVER_DIR%"
  set "GOCACHE=%GOCACHE_DIR%"
  set "CGO_ENABLED=0"
  set "GOMAXPROCS=%%P"
  "%BUILDER_EXE%" --pbf "%SERVER_DATA%\israel-and-palestine-latest.osm.pbf" --out "%BUILD_WORK_DIR%\p%%P" --cell-size 2000 --workers %%P > "%OUT_DIR%\build-scale\p%%P.log" 2>&1
  set "EXIT_CODE=!ERRORLEVEL!"
  popd
  if not "!EXIT_CODE!"=="0" exit /b !EXIT_CODE!
)
call :write_report
exit /b %ERRORLEVEL%

:all
call "%~f0" compare-static
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" compare-live
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~f0" customize
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
echo   compare-live    Run hierarchical vs base-astar/base-dijkstra live-weight benchmarks
echo   customize       Run overlay customization and amortized live-query benchmarks
echo   server-scale    Benchmark /route throughput for hierarchical and base-astar with GOMAXPROCS=1,2,4,8
echo   build-scale     Benchmark map-builder overlay construction with workers=1,2,4,8
echo   all             Run every benchmark target
echo   help            Show this help
echo.
echo Reports are written under .\benchmarks
exit /b 1
