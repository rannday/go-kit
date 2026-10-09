param(
    [ValidateSet('isolated','workspace')][string]$Mode = 'isolated',
    [ValidateSet('logx','envx','confx','buildx')][string]$Module,
    [switch]$Race
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$previous = $env:GOWORK
$failures = @()
function Invoke-Check([string]$Label, [scriptblock]$Command) {
    Write-Host "CHECK $Label"
    & $Command
    if ($LASTEXITCODE -ne 0) { $script:failures += $Label }
}
function Module-State {
    foreach ($name in @('go.mod','go.sum')) {
        if (Test-Path -LiteralPath $name) { "$name $((Get-FileHash -LiteralPath $name -Algorithm SHA256).Hash)" }
        else { "$name absent" }
    }
}
try {
    if ($Mode -eq 'isolated') { $env:GOWORK = 'off' }
    # Auto discovers this workspace while allowing buildx's temporary test modules
    # to run their child Go commands outside the workspace.
    else { $env:GOWORK = 'auto' }
    & go version
    if ($LASTEXITCODE -ne 0) { throw 'Go unavailable' }
    $modules = Get-ChildItem -LiteralPath (Join-Path $root 'modules') -Directory |
        Where-Object { (Test-Path -LiteralPath (Join-Path $_.FullName 'go.mod')) -and (!$Module -or $_.Name -eq $Module) }
    if (!$modules) { throw 'No modules selected' }
    foreach ($item in $modules) {
        Push-Location $item.FullName
        try {
            $unformatted = @(& gofmt -l .)
            if ($LASTEXITCODE -ne 0 -or $unformatted.Count) { $failures += "$($item.Name): format"; $unformatted | Write-Host }
            if ($Mode -eq 'isolated') {
                $before = @(Module-State)
                Invoke-Check "$($item.Name): tidy" { go mod tidy }
                $after = @(Module-State)
                if (Compare-Object $before $after) { $failures += "$($item.Name): tidy changed module files" }
            }
            Invoke-Check "$($item.Name): test" { go test -mod=readonly ./... }
            if ($Race) { Invoke-Check "$($item.Name): race" { go test -mod=readonly -race ./... } }
            Invoke-Check "$($item.Name): build" { go build ./... }
            Invoke-Check "$($item.Name): vet" { go vet ./... }
        } finally { Pop-Location }
    }
} finally { $env:GOWORK = $previous }
if ($failures.Count) { $failures | ForEach-Object { Write-Host "FAIL $_" }; exit 1 }
Write-Host "All $Mode checks passed."
