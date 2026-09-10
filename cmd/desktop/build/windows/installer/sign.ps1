param(
    [Parameter(Mandatory = $true)]
    [string] $File
)

$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:SQLWARDEN_SIGNING_THUMBPRINT)) {
    Write-Host "Signing disabled for $File"
    exit 0
}

if (-not (Test-Path -LiteralPath $File -PathType Leaf)) {
    throw "Signing target does not exist: $File"
}

$kitsRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
$signTool = Get-ChildItem -Path $kitsRoot -Filter signtool.exe -File -Recurse |
    Where-Object { $_.DirectoryName -like '*\x64' } |
    Sort-Object -Property FullName -Descending |
    Select-Object -First 1
if ($null -eq $signTool) {
    throw "SignTool was not found under $kitsRoot"
}

$timestampURL = $env:WINDOWS_TIMESTAMP_URL
if ([string]::IsNullOrWhiteSpace($timestampURL)) {
    $timestampURL = 'http://timestamp.digicert.com'
}

& $signTool.FullName sign `
    /sha1 $env:SQLWARDEN_SIGNING_THUMBPRINT `
    /s My `
    /fd SHA256 `
    /tr $timestampURL `
    /td SHA256 `
    /d 'SQLWarden' `
    $File
if ($LASTEXITCODE -ne 0) {
    throw "SignTool failed for $File with exit code $LASTEXITCODE"
}

& $signTool.FullName verify /pa /all $File
if ($LASTEXITCODE -ne 0) {
    throw "Signature verification failed for $File with exit code $LASTEXITCODE"
}
