param([switch]$SkipTests)
$ErrorActionPreference = 'Stop'
Push-Location -LiteralPath $PSScriptRoot
try {
    foreach ($tool in @('go', 'pnpm')) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "Required build tool not found: $tool" }
    }
    & git -C .. submodule update --init --recursive
    if ($LASTEXITCODE -ne 0) { throw 'Could not initialize pinned submodules' }
    $toolDir = Join-Path $PSScriptRoot '.tools'
    New-Item -ItemType Directory -Path $toolDir -Force | Out-Null
    $wailsPath = Join-Path $toolDir 'wails.exe'
    & go -C (Join-Path $PSScriptRoot 'buildtools') build -mod=readonly -trimpath -o $wailsPath github.com/wailsapp/wails/v2/cmd/wails
    if ($LASTEXITCODE -ne 0) { throw 'Could not build the pinned Wails tool' }
    & $wailsPath generate module
    if ($LASTEXITCODE -ne 0) { throw 'Binding generation failed' }
    Push-Location -LiteralPath (Join-Path $PSScriptRoot 'frontend')
    try {
        & pnpm install --frozen-lockfile --ignore-scripts
        if ($LASTEXITCODE -ne 0) { throw 'Frontend install failed' }
        & pnpm build
        if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
        if (-not $SkipTests) { & pnpm test; if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed' } }
    } finally { Pop-Location }
    if (-not $SkipTests) {
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Backend tests failed' }
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    }
    & $wailsPath build -ldflags '-s -w' -trimpath
    if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed' }
} finally { Pop-Location }
