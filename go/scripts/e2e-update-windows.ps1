# End-to-end test of the in-app self-update on Windows (the success path of e2e-update-linux.sh).
# Run from the go/ folder:  powershell -File scripts\e2e-update-windows.ps1
# Builds 0.9.0 and 0.9.1 with the TEST-ONLY update key (-tags updatetest), serves 0.9.1 from a local
# signed server, starts the old copy with --background and a temp AGENTCHIME_HOME, then checks ->
# installs. Never touches the real AppData, ~/.claude or ~/.codex.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$src = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$env:CGO_ENABLED = '0'
$mod = 'github.com/quanht99/agent-chime/go/internal/version.Version'
$arch = (& go env GOARCH).Trim()
$startDir = (Get-Location).Path

$tmp = Join-Path ([IO.Path]::GetTempPath()) ('agentchime-e2e-' + [Guid]::NewGuid().ToString('N'))
$work = Join-Path $tmp 'w'
$srv = Join-Path $tmp 'srv'
$home_ = Join-Path $tmp 'home'
New-Item -ItemType Directory -Force -Path $work, $srv, $home_ | Out-Null
$env:AGENTCHIME_HOME = $home_
$data = Join-Path $home_ 'AppData\Local\AgentChime'
$log = Join-Path $data 'agentchime.log'
$procs = @()
$failed = $true

function Fail($msg) { throw "FAIL: $msg" }

function Run-Go {
    & go @args
    if ($LASTEXITCODE -ne 0) { Fail "go $($args -join ' ') failed" }
}

function Wait-Until([int]$seconds, [scriptblock]$cond) {
    $end = (Get-Date).AddSeconds($seconds)
    while ((Get-Date) -lt $end) {
        if (& $cond) { return $true }
        Start-Sleep -Milliseconds 200
    }
    return $false
}

function Read-Instance {
    $f = Join-Path $data 'instance.json'
    if (-not (Test-Path $f)) { return $null }
    try { return Get-Content -Raw $f | ConvertFrom-Json } catch { return $null }
}

function Alive($processId) {
    return [bool](Get-Process -Id $processId -ErrorAction SilentlyContinue)
}

function Api([string]$method, [string]$path) {
    $i = Read-Instance
    if (-not $i) { Fail "no instance.json" }
    $uri = "http://127.0.0.1:$($i.port)$path"
    $h = @{ 'X-AgentChime-Token' = $i.token }
    if ($method -eq 'POST') {
        return Invoke-RestMethod -Method Post -Uri $uri -Headers $h -ContentType 'application/json' -Body '{}' -TimeoutSec 10
    }
    return Invoke-RestMethod -Method Get -Uri $uri -Headers $h -TimeoutSec 10
}

function Phase-Is([string]$phase) {
    try {
        $s = Api 'GET' '/api/state'
        return ((-not $s.update.busy) -and $s.update.phase -eq $phase)
    } catch { return $false }
}

try {
    # 1. build: old (0.9.0), the release (0.9.1), a helper stub (exits 2, so settings open the browser)
    foreach ($b in @(@('old', '0.9.0'), @('rel', '0.9.1'))) {
        $d = Join-Path $work $b[0]
        New-Item -ItemType Directory -Force -Path $d | Out-Null
        Push-Location $src
        Run-Go build -tags updatetest -ldflags "-X $mod=$($b[1])" -o (Join-Path $d 'AgentChime.exe') ./cmd/agentchime
        Pop-Location
        $stubDir = Join-Path $work ('stub-' + $b[0])
        New-Item -ItemType Directory -Force -Path $stubDir | Out-Null
        Set-Content -Path (Join-Path $stubDir 'main.go') -Value "package main`nimport `"os`"`nfunc main() { os.Exit(2) }`n"
        Push-Location $stubDir
        Set-Content -Path (Join-Path $stubDir 'go.mod') -Value "module stub`ngo 1.21`n"
        Run-Go build -o (Join-Path $d 'AgentChime-Settings.exe') .
        Pop-Location
    }
    Push-Location $src
    Run-Go build -tags updatetest -o (Join-Path $work 'signmanifest.exe') ./scripts/signmanifest
    Run-Go build -o (Join-Path $work 'e2eserver.exe') ./scripts/e2eserver
    Pop-Location

    # 2. the release: zip, sign with the TEST-ONLY key (set only around the signing call)
    $zip = Join-Path $srv "AgentChime-0.9.1-windows-$arch.zip"
    Compress-Archive -Path (Join-Path $work 'rel\AgentChime.exe'), (Join-Path $work 'rel\AgentChime-Settings.exe') -DestinationPath $zip
    $seedFile = Join-Path $src 'internal\update\testdata\TEST-ONLY-signing-seed.txt'
    $env:UPDATE_SIGNING_KEY = (Get-Content $seedFile | Select-Object -Last 1).Trim()
    & (Join-Path $work 'signmanifest.exe') -version 0.9.1 -require "windows-$arch" -dir $srv -out $srv | Out-Null
    $signCode = $LASTEXITCODE
    Remove-Item Env:\UPDATE_SIGNING_KEY
    if ($signCode -ne 0) { Fail 'signmanifest failed' }

    # 3. the server
    $portFile = Join-Path $tmp 'srv.port'
    $procs += Start-Process -FilePath (Join-Path $work 'e2eserver.exe') -ArgumentList '-dir', $srv, '-portfile', $portFile -PassThru -WindowStyle Hidden
    if (-not (Wait-Until 10 { (Test-Path $portFile) -and ((Get-Item $portFile).Length -gt 0) })) { Fail 'e2eserver did not start' }
    $env:AGENTCHIME_TEST_UPDATE_URL = "http://127.0.0.1:$((Get-Content $portFile).Trim())/"

    # 4. config, then the old app
    New-Item -ItemType Directory -Force -Path $data | Out-Null
    Set-Content -NoNewline -Path (Join-Path $data 'config.json') -Value '{"settingsShownOnce":true,"checkForUpdates":false}'
    $old = Start-Process -FilePath (Join-Path $work 'old\AgentChime.exe') -ArgumentList '--background' -PassThru -WindowStyle Hidden
    $procs += $old
    if (-not (Wait-Until 20 { Read-Instance })) { Fail 'instance.json did not appear within 20 s' }
    $oldInst = Read-Instance
    $oldPid = [int]$oldInst.pid

    # 5. check -> install
    Api 'POST' '/api/update/check' | Out-Null
    Start-Sleep -Seconds 1
    if (-not (Wait-Until 30 { Phase-Is 'available' })) { Fail 'phase did not become available' }
    Api 'POST' '/api/update/install' | Out-Null

    # 6. within 30 s: the old process is gone, a new instance runs and says 0.9.1 / updated
    $swapped = Wait-Until 30 {
        $i = Read-Instance
        $i -and ([int]$i.pid -ne $oldPid) -and (Alive ([int]$i.pid))
    }
    if (-not $swapped) { Fail 'no new instance appeared after the install' }
    $new = Read-Instance
    if (-not (Wait-Until 30 { -not (Alive $oldPid) })) { Fail 'the old process is still running' }
    if ([int]$new.port -eq [int]$oldInst.port -and $new.token -eq $oldInst.token) { Fail 'the instance did not change' }
    # A script block passed to Wait-Until runs in its own scope: it can't set $s, so read it again.
    if (-not (Wait-Until 30 { try { (Api 'GET' '/api/state').update.phase -eq 'updated' } catch { $false } })) { Fail 'phase did not become updated' }
    $s = Api 'GET' '/api/state'
    if ($s.version -ne '0.9.1') { Fail "the state shows version $($s.version), not 0.9.1" }
    $ver = (& (Join-Path $data 'AgentChime.exe') --version).Trim()
    if ($ver -ne '0.9.1') { Fail "the installed program prints $ver, not 0.9.1" }
    # A leftover .old-* file from the replace is fine; so is none.
    if (Test-Path (Join-Path $data 'update')) {
        if (-not (Wait-Until 5 { -not (Test-Path (Join-Path $data 'update')) })) { Fail 'the update folder was left behind' }
    }

    $failed = $false
    Write-Output 'e2e update windows ok'
}
catch {
    Write-Host "$_"
    if (Test-Path $log) { Write-Output '--- log tail ---'; Get-Content $log -Tail 50 }
}
finally {
    # Leave any Push-Location folder first, so the temp folder can be removed.
    Set-Location $startDir
    $i = Read-Instance
    if ($i) { Stop-Process -Id ([int]$i.pid) -Force -ErrorAction SilentlyContinue }
    foreach ($p in $procs) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Seconds 1
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
if ($failed) { exit 1 }
