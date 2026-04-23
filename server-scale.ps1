param(
    [Parameter(Mandatory)][string]$ServerExe, [Parameter(Mandatory)][string]$ServerWorkdir,
    [Parameter(Mandatory)][int]$GOMAXPROCS, [Parameter(Mandatory)][int]$StartupWaitSec,
    [Parameter(Mandatory)][string]$BenchmarkExe, [Parameter(Mandatory)][string]$Out
)

$ErrorActionPreference = 'Stop'

$psi = [System.Diagnostics.ProcessStartInfo]@{ FileName=$ServerExe; WorkingDirectory=$ServerWorkdir; UseShellExecute=$false }
$psi.EnvironmentVariables['GOMAXPROCS'] = $GOMAXPROCS
$psi.EnvironmentVariables['NAV_SERVER_ADDR'] = "$env:TEST_HOST:$env:TEST_BENCH_SERVER_PORT"

$srv = [System.Diagnostics.Process]::Start($psi)
if (-not $srv) { throw "Failed to start server process" }

Start-Sleep $StartupWaitSec
if ($srv.HasExited) { throw "Server exited before benchmark with code $($srv.ExitCode)" }

try {
    & $BenchmarkExe route-load --target-gomaxprocs $GOMAXPROCS --out $Out
    exit $LASTEXITCODE
}
finally {
    if (-not $srv.HasExited) { $srv.Kill() }
}