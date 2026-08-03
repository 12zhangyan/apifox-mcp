param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$packageJsonPath = Join-Path $repoRoot "npm\package.json"
$packageJson = Get-Content -LiteralPath $packageJsonPath -Raw -Encoding UTF8 | ConvertFrom-Json
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = [string]$packageJson.version
}
if ($Version -ne [string]$packageJson.version) {
    throw "Version $Version does not match npm/package.json version $($packageJson.version)"
}

$goExe = $env:APIFOX_GO_EXE
if ([string]::IsNullOrWhiteSpace($goExe)) {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($null -eq $goCommand) {
        throw "Go compiler not found; set APIFOX_GO_EXE for the build environment"
    }
    $goExe = $goCommand.Source
}

$pythonExe = $env:APIFOX_PYTHON_EXE
if ([string]::IsNullOrWhiteSpace($pythonExe)) {
    $pythonExe = Join-Path $repoRoot ".venv\Scripts\python.exe"
}
if (-not (Test-Path -LiteralPath $pythonExe -PathType Leaf)) {
    throw "Python build environment not found: $pythonExe"
}

$vendorDir = Join-Path $repoRoot "npm\vendor"
$buildDir = Join-Path $repoRoot "build\windows-npm"
New-Item -ItemType Directory -Path $vendorDir -Force | Out-Null
New-Item -ItemType Directory -Path $buildDir -Force | Out-Null

$cliOutput = Join-Path $vendorDir "apifox-cli.exe"
$mcpOutput = Join-Path $vendorDir "apifox-mcp.exe"
foreach ($output in @($cliOutput, $mcpOutput)) {
    if (Test-Path -LiteralPath $output) {
        Remove-Item -LiteralPath $output -Force
    }
}

$commit = (& git -C $repoRoot rev-parse --short=12 HEAD).Trim()
$buildDate = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
$ldflags = "-s -w " +
    "-X github.com/iwen-conf/apifox-mcp/internal/apifoxcli.version=$Version " +
    "-X github.com/iwen-conf/apifox-mcp/internal/apifoxcli.commit=$commit " +
    "-X github.com/iwen-conf/apifox-mcp/internal/apifoxcli.date=$buildDate"

Push-Location -LiteralPath $repoRoot
try {
    & $goExe build -trimpath "-ldflags=$ldflags" -o $cliOutput ./cmd/apifox-cli
    if ($LASTEXITCODE -ne 0) {
        throw "Go CLI build failed with exit code $LASTEXITCODE"
    }

    & $pythonExe -m PyInstaller `
        --noconfirm `
        --clean `
        --onefile `
        --console `
        --name apifox-mcp `
        --distpath $vendorDir `
        --workpath (Join-Path $buildDir "work") `
        --specpath (Join-Path $buildDir "spec") `
        --paths $repoRoot `
        --collect-all mcp `
        --collect-all mcp_types `
        scripts\pyinstaller_entry.py
    if ($LASTEXITCODE -ne 0) {
        throw "PyInstaller build failed with exit code $LASTEXITCODE"
    }

    node npm\scripts\verify-package.cjs
    if ($LASTEXITCODE -ne 0) {
        throw "npm payload verification failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

Write-Host "Built Windows npm payload for version $Version"
Write-Host "  $mcpOutput"
Write-Host "  $cliOutput"
