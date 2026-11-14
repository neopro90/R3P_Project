param(
    [switch]$InitialV1 = $false,
    [string]$TargetVersion = "",
    [string]$Message = "",
    [string]$RepoUrl = "https://github.com/neopro90/R3P_Project.git",
    [string]$Branch = "main",
    [switch]$ForcePush = $false
)

$ErrorActionPreference = "Stop"
$RootDir = Split-Path -Parent $PSScriptRoot
Set-Location $RootDir

$VersionFile = Join-Path $RootDir "VERSION"
$ReleaseFile = Join-Path $RootDir "release.json"
$BuildVerFile = Join-Path $RootDir "build\files\etc\r3p\version.json"
$PkgJsonFile = Join-Path $RootDir "frontend\package.json"

# 1. Read or Initialize Version & Revision
$rev = 1
$ver = "v1.0.0"

if (Test-Path $ReleaseFile) {
    $relObj = Get-Content $ReleaseFile -Raw | ConvertFrom-Json
    if ($relObj.revision) { $rev = [int]$relObj.revision }
    if ($relObj.version) { $ver = [string]$relObj.version }
}

if (-not $InitialV1) {
    # Increment revision by +1 on every upload and keep version patch synchronized!
    $rev = $rev + 1
    if (-not [string]::IsNullOrWhiteSpace($TargetVersion)) {
        $ver = $TargetVersion
    } elseif ($ver -match '^v(\d+)\.(\d+)\.(\d+)$') {
        $major = [int]$Matches[1]
        $minor = [int]$Matches[2]
        $ver = "v$major.$minor.$rev"
    } else {
        $ver = "v1.0.$rev"
    }
} else {
    $rev = 1
    $ver = if (-not [string]::IsNullOrWhiteSpace($TargetVersion)) { $TargetVersion } else { "v1.0.0" }
}

$dateStr = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss")
if ([string]::IsNullOrWhiteSpace($Message)) {
    $Message = "Release $ver (Revision #$rev) - R3P Home Server OS update"
}

Write-Host "======================================================" -ForegroundColor Cyan
Write-Host " Preparing R3P_Project Release: $ver (Revision #$rev)" -ForegroundColor Green
Write-Host "======================================================" -ForegroundColor Cyan

# 2. Write updated VERSION, release.json, build/files/etc/r3p/version.json & frontend/package.json
$newRelease = [ordered]@{
    version     = $ver
    revision    = $rev
    released_at = $dateStr
    repo_url    = $RepoUrl
    branch      = $Branch
    commit_sha  = $ver
    changelog   = $Message
}
$jsonStr = $newRelease | ConvertTo-Json -Depth 4
[System.IO.File]::WriteAllText($ReleaseFile, $jsonStr, [System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText($VersionFile, $ver, [System.Text.UTF8Encoding]::new($false))
if (Test-Path (Split-Path -Parent $BuildVerFile)) {
    [System.IO.File]::WriteAllText($BuildVerFile, $jsonStr, [System.Text.UTF8Encoding]::new($false))
}
if (Test-Path $PkgJsonFile) {
    $semVer = $ver.TrimStart('v')
    $pkgRaw = Get-Content $PkgJsonFile -Raw
    $pkgUpdated = [regex]::Replace($pkgRaw, '"version"\s*:\s*"[^"]+"', """version"": ""$semVer""", 1)
    [System.IO.File]::WriteAllText($PkgJsonFile, $pkgUpdated, [System.Text.UTF8Encoding]::new($false))
}

# 3. Rebuild frontend SPA & cross-compile MIPSLE r3p-api before packaging bundle
Write-Host "[*] Building frontend SPA..." -ForegroundColor Yellow
Push-Location (Join-Path $RootDir "frontend")
try {
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
} finally {
    Pop-Location
}

Write-Host "[*] Cross-compiling r3p-api (linux/mipsle softfloat)..." -ForegroundColor Yellow
$ArtifactDir = Join-Path $RootDir "build\artifacts"
New-Item -ItemType Directory -Force -Path $ArtifactDir | Out-Null
$ArtifactBin = Join-Path $ArtifactDir "r3p-api"
$env:GOOS = "linux"; $env:GOARCH = "mipsle"; $env:GOMIPS = "softfloat"; $env:CGO_ENABLED = "0"
try {
    Push-Location (Join-Path $RootDir "source\backend")
    try {
        $buildIso = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        $semVer = $ver.TrimStart('v')
        go build -ldflags="-s -w -X main.Version=$ver -X main.RevisionStr=$rev -X main.BuildDate=$buildIso" -trimpath -o $ArtifactBin .
        if ($LASTEXITCODE -ne 0) { throw "Go cross-compile failed" }
    } finally {
        Pop-Location
    }
} finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:GOMIPS, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
Copy-Item -Force $ArtifactBin (Join-Path $RootDir "build\files\usr\bin\r3p-api")
Copy-Item -Force $ArtifactBin (Join-Path $RootDir "build\output\r3p-api-mipsle")
if (Test-Path (Join-Path $RootDir "build\web-static")) { Remove-Item -Recurse -Force (Join-Path $RootDir "build\web-static") }
Copy-Item -Recurse -Force (Join-Path $RootDir "frontend\dist") (Join-Path $RootDir "build\web-static")
if (Test-Path (Join-Path $RootDir "build\files\www")) { Remove-Item -Recurse -Force (Join-Path $RootDir "build\files\www") }
Copy-Item -Recurse -Force (Join-Path $RootDir "frontend\dist") (Join-Path $RootDir "build\files\www")

# 4. Rebuild r3p-update-bundle.tar.gz so the bundle has exact binary, SPA & version.json
python "$RootDir\scripts\package-and-deploy.py"
if ($LASTEXITCODE -ne 0) { throw "Packaging failed" }

# 4. Initialize Git if needed & configure remote
if (-not (Test-Path (Join-Path $RootDir ".git"))) {
    git init
}
git branch -M $Branch

$remotes = git remote
if ($remotes -contains "origin") {
    git remote set-url origin $RepoUrl
} else {
    git remote add origin $RepoUrl
}

# 5. Stage, Commit, Tag & Push to GitHub
git add -A
$status = git status --porcelain
if ($status) {
    git commit -m "$ver (Rev $rev): $Message"
} else {
    git commit --allow-empty -m "$ver (Rev $rev): $Message"
}

# Create annotated tag for this revision (never rewrite existing tags silently)
git tag -a $ver -m "R3P Home Server OS $ver (Revision #$rev)"

Write-Host "[*] Pushing branch '$Branch' and tag '$ver' to $RepoUrl ..." -ForegroundColor Yellow
if ($ForcePush) {
    git push -u origin $Branch --force-with-lease
} else {
    git push -u origin $Branch
}
git push origin $ver

Write-Host "[OK] Uploaded $ver (Revision #$rev) to $RepoUrl successfully!" -ForegroundColor Green
