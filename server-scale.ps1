param(
    [Parameter(Mandatory = $true)][string]$ServerExe,
    [Parameter(Mandatory = $true)][string]$ServerWorkdir,
    [Parameter(Mandatory = $true)][string]$ServerAddr,
    [Parameter(Mandatory = $true)][string]$ServerData,
    [Parameter(Mandatory = $true)][string]$RoutingMode,
    [Parameter(Mandatory = $true)][int]$GOMAXPROCS,
    [Parameter(Mandatory = $true)][int]$StartupWaitSec,
    [Parameter(Mandatory = $true)][string]$BenchmarkExe,
    [Parameter(Mandatory = $true)][string]$ServerURL,
    [Parameter(Mandatory = $true)][string]$Cases,
    [Parameter(Mandatory = $true)][int]$Concurrency,
    [Parameter(Mandatory = $true)][int]$Requests,
    [Parameter(Mandatory = $true)][int]$Warmup,
    [Parameter(Mandatory = $true)][string]$Out
)

$ErrorActionPreference = 'Stop'

$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $ServerExe
$psi.WorkingDirectory = $ServerWorkdir
$psi.UseShellExecute = $false
$psi.Arguments = ('--addr "{0}" --data "{1}" --routing-mode "{2}"' -f $ServerAddr, $ServerData, $RoutingMode)
$psi.EnvironmentVariables['GOMAXPROCS'] = [string]$GOMAXPROCS

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
        --server $ServerURL `
        --cases $Cases `
        --concurrency $Concurrency `
        --requests $Requests `
        --warmup $Warmup `
        --routing-mode $RoutingMode `
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
