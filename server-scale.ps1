param(
    [Parameter(Mandatory = $true)][string]$ServerExe,
    [Parameter(Mandatory = $true)][string]$ServerWorkdir,
    [Parameter(Mandatory = $true)][int]$GOMAXPROCS,
    [Parameter(Mandatory = $true)][int]$StartupWaitSec,
    [Parameter(Mandatory = $true)][string]$BenchmarkExe,
    [Parameter(Mandatory = $true)][string]$Out
)

$ErrorActionPreference = 'Stop'

$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $ServerExe
$psi.WorkingDirectory = $ServerWorkdir
$psi.UseShellExecute = $false
$psi.EnvironmentVariables['GOMAXPROCS'] = [string]$GOMAXPROCS
$psi.EnvironmentVariables['NAV_SERVER_ADDR'] = ('{0}:{1}' -f $env:TEST_HOST, $env:TEST_BENCH_SERVER_PORT)

$serverProc = [System.Diagnostics.Process]::Start($psi)
if (-not $serverProc) {
    throw 'failed to start server process'
}

Start-Sleep -Seconds $StartupWaitSec
if ($serverProc.HasExited) {
    throw "server exited before benchmark with code $($serverProc.ExitCode)"
}

$benchExit = 0
try {
    & $BenchmarkExe `
        route-load `
        --target-gomaxprocs $GOMAXPROCS `
        --out $Out
    $benchExit = $LASTEXITCODE
}
finally {
    if ($serverProc -and -not $serverProc.HasExited) {
        Stop-Process -Id $serverProc.Id -Force
    }
}

exit $benchExit
