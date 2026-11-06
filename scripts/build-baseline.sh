#!/usr/bin/env bash
set -euo pipefail

# WSL appends Windows PATH entries; drive-letter colons can create relative
# components that GNU find rejects when ImageBuilder uses -execdir.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

PROJECT_ROOT=/mnt/d/R3P_Project
ARCHIVE="$PROJECT_ROOT/Supplementary Files/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64.tar.zst"
EXPECTED_SHA256=e825abe3659d510320ca7e5daf3b9471ca214208fe7bb90c4c6e2ed0a708fa07
WORK_ROOT=/root/r3p-work
IMAGEBUILDER_ROOT="$WORK_ROOT/imagebuilder/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64"
PROFILE=xiaomi_mi-router-3-pro
STAMP="$(date +%Y%m%d-%H%M%S)"
OUTPUT_DIR="$PROJECT_ROOT/build/output/baseline-$STAMP"
LOG="$PROJECT_ROOT/BUILD_LOGS/baseline-$STAMP.log"

mkdir -p "$WORK_ROOT/imagebuilder" "$PROJECT_ROOT/build/output" "$PROJECT_ROOT/BUILD_LOGS"
exec > >(tee -a "$LOG") 2>&1

echo "R3P baseline firmware build"
echo "Started: $(date -Is)"
echo "Profile: $PROFILE"
echo "Project: $PROJECT_ROOT"
echo "Build workspace: $IMAGEBUILDER_ROOT"

for tool in make python3 rsync zstd tar sha256sum; do
    command -v "$tool" >/dev/null || {
        echo "Missing required build tool: $tool" >&2
        exit 1
    }
done

actual_sha256="$(sha256sum "$ARCHIVE" | awk '{print $1}')"
if [[ "$actual_sha256" != "$EXPECTED_SHA256" ]]; then
    echo "ImageBuilder checksum mismatch: $actual_sha256" >&2
    exit 1
fi
echo "ImageBuilder SHA256 verified: $actual_sha256"

if [[ ! -f "$IMAGEBUILDER_ROOT/Makefile" ]]; then
    echo "Extracting ImageBuilder into project-local WSL filesystem..."
    tar --zstd -xf "$ARCHIVE" -C "$WORK_ROOT/imagebuilder"
fi
[[ -f "$IMAGEBUILDER_ROOT/Makefile" ]] || {
    echo "ImageBuilder Makefile missing after extraction" >&2
    exit 1
}

mkdir -p "$OUTPUT_DIR"
cd "$IMAGEBUILDER_ROOT"
echo "Running unmodified baseline ImageBuilder profile..."
make image PROFILE="$PROFILE" BIN_DIR="$OUTPUT_DIR"

shopt -s nullglob
images=("$OUTPUT_DIR"/*.bin)
if (( ${#images[@]} == 0 )); then
    echo "Build returned success but produced no firmware .bin images" >&2
    exit 1
fi

echo "Baseline output:"
for image in "${images[@]}"; do
    stat --printf='%n %s bytes\n' "$image"
    sha256sum "$image"
    file "$image"
done
echo "Finished: $(date -Is)"
