#!/bin/bash
set -e
cd /root/r3p-work/imagebuilder/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64
make clean 2>/dev/null || true
make image PROFILE="xiaomi_mi-router-3-pro" \
    PACKAGES="base-files busybox ca-bundle dropbear dnsmasq firewall4 nftables kmod-nft-offload uci uclient-fetch urandom-seed urngd odhcpd-ipv6only odhcp6c kmod-mt7615e kmod-mt7615-firmware wpad-basic-mbedtls iwinfo kmod-usb3 kmod-usb-storage kmod-fs-ext4 kmod-fs-vfat kmod-fs-exfat dosfstools e2fsprogs block-mount -luci" \
    FILES="/mnt/d/R3P_Project/build/files"
mkdir -p /mnt/d/R3P_Project/releases/v1.0.0-final
cp bin/targets/ramips/mt7621/* /mnt/d/R3P_Project/releases/v1.0.0-final/
echo "BUILD_COMPLETE_SUCCESS"
