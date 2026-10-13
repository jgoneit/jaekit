# Developer checks: requires source checkout, Git and the Go version in go.mod.
# This does not install ha or establish installed-binary/host acceptance.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-WindowsPlatform
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Push-Location -LiteralPath $root
try {
    # Do not permit persistent/environment Go test filters to hide fixtures.
    $flags = & go env GOFLAGS
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read Go test configuration.' }
    if (-not [string]::IsNullOrWhiteSpace($flags)) { throw 'Use an environment with empty GOFLAGS for the explicit native selection.' }
    $target = @(& go env GOOS GOARCH)
    if ($LASTEXITCODE -ne 0 -or $target.Count -ne 2 -or $target[0] -cne 'windows' -or $target[1] -cne 'amd64') {
        throw 'The effective Go target must be windows/amd64.'
    }
    & go test -mod=readonly -count=1 -run '^TestNative' ./internal/run ./internal/record ./internal/evidence ./cmd/ha
    if ($LASTEXITCODE -ne 0) { throw 'Native developer checks failed.' }
    Write-Output 'PASS: native developer fixtures. Installed lifecycle and host observations are separate.'
} finally { Pop-Location }
