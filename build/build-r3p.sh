#!/bin/bash
# R3P Firmware — ImageBuilder Build Script
# Phase 0: Minimal image without LuCI, with uhttpd serving static files
# Run from the extracted ImageBuilder directory
# Usage: bash build-r3p.sh 2>&1 | tee ../logs/build.log

set -e

PROFILE="xiaomi_mi-router-3-pro"
BUILD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${BUILD_DIR}/../output"
WEB_STATIC="${BUILD_DIR}/../web-static"

echo "================================================"
echo "R3P Firmware ImageBuilder — Phase 0 Build"
echo "Profile: ${PROFILE}"
echo "Target:  ramips/mt7621"
echo "Start:   $(date)"
echo "================================================"

# Copy static web files into ImageBuilder files overlay
mkdir -p "${BUILD_DIR}/imagebuilder/files/www"
cp -r "${WEB_STATIC}/." "${BUILD_DIR}/imagebuilder/files/www/"
echo "✓ Static files copied to overlay"

# Generate uhttpd config (no LuCI, just static files)
mkdir -p "${BUILD_DIR}/imagebuilder/files/etc/config"
cat > "${BUILD_DIR}/imagebuilder/files/etc/config/uhttpd" << 'UHTTP'
config uhttpd main
    option listen_http  0.0.0.0:80
    option home         /www
    option max_requests 3
    option no_dirlists  1
UHTTP
echo "✓ uhttpd config written"

# Packages:
# KEEP: base system, wifi, usb, ssh, network management
# REMOVE: luci and all luci-* packages (prefix with -)
PACKAGES=" \
  base-files libc libgcc1 \
  kmod-mt7615-firmware kmod-usb3 kmod-usb-ledtrig-usbport \
  kmod-usb-storage kmod-fs-ext4 block-mount \
  netifd dnsmasq \
  uhttpd \
  openssh-sftp-server dropbear \
  ubus uci rpcd rpcd-mod-rpcsys \
  iwinfo kmod-cfg80211 \
  ip-full iptables \
  nftables kmod-nft-core kmod-nft-nat \
  uboot-envtools mtd \
  -luci -luci-base -luci-mod-admin-full \
  -luci-app-firewall -luci-app-package-manager \
  -luci-app-attendedsysupgrade \
  -luci-theme-bootstrap \
  -luci-mod-network -luci-mod-status -luci-mod-system \
  -luci-light -luci-ssl -luci-proto-ipv6 -luci-proto-ppp \
"

cd "${BUILD_DIR}/imagebuilder"

echo ""
echo "Running make image..."
echo "PROFILE=${PROFILE}"
echo "PACKAGES=${PACKAGES}"
echo ""

make image \
  PROFILE="${PROFILE}" \
  PACKAGES="${PACKAGES}" \
  FILES="${BUILD_DIR}/imagebuilder/files" \
  2>&1

# Copy output
echo ""
echo "Copying output files..."
mkdir -p "${OUTPUT_DIR}"
cp bin/targets/ramips/mt7621/openwrt-25.12.5-ramips-mt7621-${PROFILE}-* "${OUTPUT_DIR}/" 2>/dev/null || true

echo ""
echo "================================================"
echo "Build complete: $(date)"
echo "Output files:"
ls -lah "${OUTPUT_DIR}/" 2>/dev/null
echo "================================================"
