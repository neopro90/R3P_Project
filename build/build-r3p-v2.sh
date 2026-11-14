#!/bin/bash
# ============================================================
# R3P Firmware — ImageBuilder Build Script v2
# Target: Xiaomi Mi Router 3 Pro (ramips/mt7621)
# OpenWrt: 25.12.5
# Run inside: WSL Ubuntu after reboot
# ============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Windows D: drive accessible via /mnt/d
SUPP_DIR="/mnt/d/R3P_Project/Supplementary Files"
IB_TARBALL="$SUPP_DIR/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64.tar.zst"
IB_DIR="$SCRIPT_DIR/imagebuilder/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64"
FILES_DIR="$SCRIPT_DIR/files"
OUTPUT_DIR="$SCRIPT_DIR/output"
LOG_DIR="$SCRIPT_DIR/logs"

PROFILE="xiaomi_mi-router-3-pro"
OPENWRT_VER="25.12.5"

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# ---- Pre-flight checks ----
preflight() {
    log_info "=== R3P ImageBuilder v2 — Pre-flight ==="
    log_info "Project: $PROJECT_DIR"
    log_info "Profile: $PROFILE"
    log_info "OpenWrt: $OPENWRT_VER"

    local deps_ok=true
    for cmd in make python3 zstd tar file sha256sum; do
        if command -v "$cmd" &>/dev/null; then
            log_info "  ✅ $cmd found"
        else
            log_error "  ❌ $cmd NOT FOUND"
            deps_ok=false
        fi
    done

    if [ "$deps_ok" = false ]; then
        log_error "Install missing deps: sudo apt install -y build-essential zstd python3"
        exit 1
    fi

    mkdir -p "$IB_DIR" "$OUTPUT_DIR" "$LOG_DIR" "$SCRIPT_DIR/imagebuilder"
}

# ---- Extract ImageBuilder ----
extract_ib() {
    if [ -f "$IB_DIR/Makefile" ]; then
        log_info "ImageBuilder already extracted at $IB_DIR"
        return
    fi

    log_info "Extracting ImageBuilder (this may take a few minutes)..."
    if [ ! -f "$IB_TARBALL" ]; then
        log_error "ImageBuilder tarball not found: $IB_TARBALL"
        exit 1
    fi

    # Verify checksum first
    log_info "Verifying SHA256..."
    EXPECTED="e825abe3659d510320ca7e5daf3b9471ca214208fe7bb90c4c6e2ed0a708fa07"
    ACTUAL=$(sha256sum "$IB_TARBALL" | awk '{print $1}')
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        log_error "SHA256 mismatch! Expected: $EXPECTED, Got: $ACTUAL"
        exit 1
    fi
    log_info "SHA256 verified ✅"

    tar -xf "$IB_TARBALL" -C "$SCRIPT_DIR/imagebuilder/"
    log_info "Extraction complete"
}

# ---- Copy overlay files ----
copy_files() {
    log_info "Preparing overlay files..."

    # Copy API binary and frontend SPA bundle
    local api_bin="$SCRIPT_DIR/artifacts/r3p-api"
    [ -f "$api_bin" ] || api_bin="$OUTPUT_DIR/r3p-api-mipsle"
    if [ -f "$api_bin" ]; then
        mkdir -p "$FILES_DIR/usr/bin"
        cp "$api_bin" "$FILES_DIR/usr/bin/r3p-api"
        chmod +x "$FILES_DIR/usr/bin/r3p-api"
        log_info "  ✅ r3p-api binary copied ($api_bin)"
    else
        log_warn "  ⚠️  r3p-api artifact not found — API will not be in image"
    fi
    local dist_dir="$PROJECT_DIR/frontend/dist"
    [ -f "$dist_dir/index.html" ] || dist_dir="$SCRIPT_DIR/web-static"
    if [ -f "$dist_dir/index.html" ]; then
        rm -rf "$FILES_DIR/www/assets"
        mkdir -p "$FILES_DIR/www"
        cp -a "$dist_dir/." "$FILES_DIR/www/"
        log_info "  ✅ Frontend assets synced from $dist_dir"
    fi

    # Copy init.d script
    if [ -f "$FILES_DIR/etc/init.d/r3p-api" ]; then
        chmod +x "$FILES_DIR/etc/init.d/r3p-api"
        log_info "  ✅ init.d/r3p-api ready"
    fi

    # Copy web static files
    if [ -d "$SCRIPT_DIR/web-static" ] && [ "$(ls -A $SCRIPT_DIR/web-static)" ]; then
        cp -r "$SCRIPT_DIR/web-static/." "$FILES_DIR/www/"
        log_info "  ✅ Web static files copied to /www"
    fi

    log_info "Overlay files ready at $FILES_DIR"
}

# ---- Packages ----
PACKAGES_KEEP="
    kmod-mt7615-firmware
    kmod-usb3
    kmod-usb-ledtrig-usbport
    kmod-usb-storage
    kmod-fs-ext4
    kmod-fs-vfat
    kmod-nft-core
    kmod-nft-nat
    block-mount
    netifd
    dnsmasq
    uhttpd
    dropbear
    ubus
    uci
    rpcd
    rpcd-mod-rpcsys
    iwinfo
    nftables
    ip-full
    kmod-cfg80211
    uboot-envtools
    mtd
    snapshot-tool
    nand-utils
    ubi-utils
    usbutils
    kmod-usb-net
"

PACKAGES_REMOVE="
    -luci
    -luci-base
    -luci-light
    -luci-mod-admin-full
    -luci-mod-network
    -luci-mod-status
    -luci-mod-system
    -luci-app-firewall
    -luci-app-package-manager
    -luci-app-attendedsysupgrade
    -luci-theme-bootstrap
    -luci-ssl
    -luci-proto-ipv6
    -luci-proto-ppp
    -luci-lib-uqr
    -px5g-mbedtls
    -owut
    -liblucihttp
    -liblucihttp-ucode
"

PACKAGES="$(echo $PACKAGES_KEEP $PACKAGES_REMOVE | tr '\n' ' ' | xargs)"

# ---- Run build ----
run_build() {
    log_info "=== Starting ImageBuilder ==="
    log_info "Profile: $PROFILE"
    log_info "Output: $OUTPUT_DIR"

    local build_log="$LOG_DIR/build-$(date +%Y%m%d-%H%M%S).log"
    log_info "Log: $build_log"

    cd "$IB_DIR"

    make image \
        PROFILE="$PROFILE" \
        PACKAGES="$PACKAGES" \
        FILES="$FILES_DIR" \
        BIN_DIR="$OUTPUT_DIR" \
        2>&1 | tee "$build_log"

    BUILD_EXIT=${PIPESTATUS[0]}

    if [ $BUILD_EXIT -ne 0 ]; then
        log_error "Build FAILED (exit $BUILD_EXIT). Check: $build_log"
        exit $BUILD_EXIT
    fi

    log_info "=== Build SUCCESS ==="
}

# ---- Validate output ----
validate_output() {
    log_info "=== Validating firmware images ==="

    local factory=""
    local sysupgrade=""

    for f in "$OUTPUT_DIR"/*.bin; do
        [ -f "$f" ] || continue
        fname=$(basename "$f")
        size=$(stat -c%s "$f")
        sha=$(sha256sum "$f" | awk '{print $1}')

        log_info "File: $fname"
        log_info "  Size: $size bytes ($(echo "scale=2; $size/1024/1024" | bc) MB)"
        log_info "  SHA256: $sha"

        # Check magic bytes (OpenWrt bin usually starts with specific magic)
        magic=$(xxd "$f" | head -1 | awk '{print $2$3}')
        log_info "  Magic: $magic"

        # Size check — must be < 249.5 MB
        MAX_SIZE=261619712
        if [ "$size" -gt "$MAX_SIZE" ]; then
            log_error "  ❌ Image too large! Max: $MAX_SIZE bytes"
        else
            log_info "  ✅ Size OK"
        fi

        if echo "$fname" | grep -q "factory"; then
            factory="$f"
            log_info "  → Factory image"
        elif echo "$fname" | grep -q "sysupgrade"; then
            sysupgrade="$f"
            log_info "  → Sysupgrade image"
        fi
    done

    if [ -n "$sysupgrade" ]; then
        log_info ""
        log_info "=== FLASH COMMANDS (when ready) ==="
        log_info "# From router SSH (OpenWrt running):"
        log_info "sysupgrade -v -n '$sysupgrade'"
        log_info ""
        log_info "# Or via web UI at http://192.168.1.1/cgi-bin/luci/admin/system/upgrade"
    fi
}

# ---- Main ----
main() {
    preflight
    extract_ib
    copy_files
    run_build
    validate_output
    log_info ""
    log_info "🎉 R3P Build complete! Images in: $OUTPUT_DIR"
}

main "$@"
