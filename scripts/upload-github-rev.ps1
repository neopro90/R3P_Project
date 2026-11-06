param(
    [switch]$InitialV1 = $false,
    [string]$TargetVersion = "",
    [string]$Message = "",
    [string]$RepoUrl = "https://github.com/neopro90/R3P_Project.git",
    [string]$Branch = "main"
)

$ErrorActionPreference = "Stop"
$RootDir = Split-Path -Parent $PSScriptRoot
Set-Location $RootDir

$VersionFile = Join-Path $RootDir "VERSION"
$ReleaseFile = Join-Path $RootDir "release.json"

# 1. Read or Initialize Version & Revision
$rev = 1
$ver = "v1.0.0"

if (Test-Path $ReleaseFile) {
    $relObj = Get-Content $ReleaseFile -Raw | ConvertFrom-Json
    if ($relObj.revision) { $rev = [int]$relObj.revision }
    if ($relObj.version) { $ver = [string]$relObj.version }
}

if (-not $InitialV1) {
    # Increment revision by +1 on every upload!
    $rev = $rev + 1
    if (-not [string]::IsNullOrWhiteSpace($TargetVersion)) {
        $ver = $TargetVersion
    } elseif ($ver -match '^v(\d+)\.(\d+)\.(\d+)$') {
        $major = [int]$Matches[1]
        $minor = [int]$Matches[2]
        $patch = $rev - 1
        $ver = "v$major.$minor.$patch"
    } else {
        $ver = "v1.0.$($rev - 1)"
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

# 2. Write updated VERSION & release.json
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

# 3. Rebuild r3p-update-bundle.tar.gz so the bundle on GitHub has the exact new version.json
python "$RootDir\scripts\package-and-deploy.py"

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

# Create/update tag for this revision
git tag -f $ver -m "R3P Home Server OS $ver (Revision #$rev)"

Write-Host "[*] Pushing branch '$Branch' and tag '$ver' to $RepoUrl ..." -ForegroundColor Yellow
git push -u origin $Branch --force
git push origin $ver --force

Write-Host "[OK] Uploaded $ver (Revision #$rev) to $RepoUrl successfully!" -ForegroundColor Green
