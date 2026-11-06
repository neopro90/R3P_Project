#!/usr/bin/env bash
set -euo pipefail

export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

PROJECT_ROOT=/mnt/d/R3P_Project
WORK_ROOT=/root/r3p-work
IMAGEBUILDER_ROOT="$WORK_ROOT/imagebuilder/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64"
API_BINARY="$PROJECT_ROOT/build/output/r3p-api-mipsle"
FRONTEND_DIST="$PROJECT_ROOT/frontend/dist"
PROFILE=xiaomi_mi-router-3-pro
STAMP="$(date +%Y%m%d-%H%M%S)"
OUTPUT_DIR="$PROJECT_ROOT/build/output/custom-$STAMP"
OVERLAY_DIR="$WORK_ROOT/custom-overlays/$STAMP"
LOG="$PROJECT_ROOT/BUILD_LOGS/custom-$STAMP.log"

mkdir -p "$PROJECT_ROOT/build/output" "$PROJECT_ROOT/BUILD_LOGS" "$WORK_ROOT/custom-overlays"
exec > >(tee -a "$LOG") 2>&1

echo "R3P custom firmware build"
echo "Started: $(date -Is)"
echo "Profile: $PROFILE"
echo "Overlay: $OVERLAY_DIR"

[[ -f "$IMAGEBUILDER_ROOT/Makefile" ]] || {
    echo "ImageBuilder is not extracted; build the baseline first." >&2
    exit 1
}
[[ -f "$API_BINARY" ]] || {
    echo "MIPSLE API binary is missing: $API_BINARY" >&2
    exit 1
}
[[ -f "$FRONTEND_DIST/index.html" ]] || {
    echo "Frontend production build is missing; run npm build first." >&2
    exit 1
}

file "$API_BINARY" | tee /dev/stderr | grep -q 'ELF 32-bit LSB.*MIPS'
if ! command -v upx >/dev/null 2>&1; then
    echo "UPX is required to package the MIPSLE API binary; install upx-ucl in the project-local WSL environment." >&2
    exit 1
fi
if upx -t "$API_BINARY" >/dev/null 2>&1; then
    echo "MIPSLE API is already UPX-packed."
else
    upx --best --lzma "$API_BINARY"
fi
upx -t "$API_BINARY"
mkdir -p "$OVERLAY_DIR"
cp -a "$PROJECT_ROOT/build/files/." "$OVERLAY_DIR/"
mkdir -p "$OVERLAY_DIR/usr/bin" "$OVERLAY_DIR/www"
install -m 0755 "$API_BINARY" "$OVERLAY_DIR/usr/bin/r3p-api"
cp -a "$FRONTEND_DIST/." "$OVERLAY_DIR/www/"

PACKAGES="kmod-mt7615-firmware kmod-usb3 kmod-usb-storage kmod-fs-ext4 block-mount uhttpd \
aria2 adblock-fast ksmbd-server kmod-fs-ksmbd \
-luci -luci-base -luci-light -luci-mod-admin-full -luci-mod-network -luci-mod-status \
-luci-mod-system -luci-app-firewall -luci-app-package-manager -luci-app-attendedsysupgrade \
-luci-theme-bootstrap -luci-ssl -luci-proto-ipv6 -luci-proto-ppp -luci-lib-uqr \
-rpcd-mod-luci -px5g-mbedtls"

mkdir -p "$OUTPUT_DIR"
cd "$IMAGEBUILDER_ROOT"
echo "Selected packages: $PACKAGES"
make image PROFILE="$PROFILE" PACKAGES="$PACKAGES" FILES="$OVERLAY_DIR" BIN_DIR="$OUTPUT_DIR"

shopt -s nullglob
images=("$OUTPUT_DIR"/*.bin)
if (( ${#images[@]} == 0 )); then
    echo "Build returned success but produced no custom firmware .bin images" >&2
    exit 1
fi

for image in "${images[@]}"; do
    size="$(stat -c%s "$image")"
    if (( size > 261619712 )); then
        echo "Image exceeds profile metadata limit: $image ($size bytes)" >&2
        exit 1
    fi
    stat --printf='%n %s bytes\n' "$image"
    sha256sum "$image"
    file "$image"
done
echo "Custom firmware build completed. Do not flash without separate device/hardware validation."
echo "Finished: $(date -Is)"
