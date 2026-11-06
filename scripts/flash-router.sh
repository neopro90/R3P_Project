#!/bin/bash
# ============================================================
# R3P — Safe Flash Procedure Script
# Run from WSL or Linux, NOT from Windows PowerShell
# Prerequisites:
#   - Router accessible at 192.168.1.1 via SSH
#   - sysupgrade .bin built and present in build/output/
# ============================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

ROUTER_IP="${ROUTER_IP:-192.168.1.1}"
ROUTER_USER="${ROUTER_USER:-root}"
OUTPUT_DIR="/mnt/d/R3P_Project/build/output"

echo ""
echo "╔═══════════════════════════════════════════╗"
echo "║     R3P — Safe Flash Procedure v1.0      ║"
echo "╚═══════════════════════════════════════════╝"
echo ""
log_warn "THIS WILL FLASH YOUR ROUTER. PROCEED WITH CARE."
echo ""

# ---- Find firmware image ----
SYSUPGRADE=$(ls "$OUTPUT_DIR"/*sysupgrade*.bin 2>/dev/null | head -1 || "")
if [ -z "$SYSUPGRADE" ]; then
    log_error "No sysupgrade .bin found in $OUTPUT_DIR"
    log_error "Run build-r3p-v2.sh first"
    exit 1
fi

log_info "Firmware: $SYSUPGRADE"
log_info "Size: $(stat -c%s "$SYSUPGRADE") bytes ($(du -sh "$SYSUPGRADE" | awk '{print $1}'))"
echo ""

# ---- Pre-flash checks ----
log_info "=== Pre-flash checks ==="

# 1. Ping router
if ping -c 2 "$ROUTER_IP" &>/dev/null; then
    log_info "Router $ROUTER_IP reachable ✅"
else
    log_error "Router $ROUTER_IP NOT reachable"
    exit 1
fi

# 2. SSH check
if ssh -o ConnectTimeout=5 -o BatchMode=yes "$ROUTER_USER@$ROUTER_IP" 'echo SSH_OK' 2>/dev/null | grep -q SSH_OK; then
    log_info "SSH to router OK ✅"
else
    log_error "SSH to router FAILED. Set up SSH key or use ssh-copy-id."
    exit 1
fi

# 3. Check NAND chip on router
log_info "Checking NAND chip on router..."
NAND_INFO=$(ssh "$ROUTER_USER@$ROUTER_IP" 'dmesg | grep -i "Manufacturer ID" | head -1' 2>/dev/null || "")
if echo "$NAND_INFO" | grep -q "0xc8"; then
    log_info "NAND: ESMT (0xc8) — SAFE ✅"
elif echo "$NAND_INFO" | grep -q "0x2c"; then
    log_warn "NAND: Micron (0x2c) — CAUTION: known bad block issues"
    echo -n "Continue anyway? (yes/no): "
    read CONFIRM
    [ "$CONFIRM" = "yes" ] || exit 1
else
    log_warn "NAND chip not identified: $NAND_INFO"
    echo -n "Continue anyway? (yes/no): "
    read CONFIRM
    [ "$CONFIRM" = "yes" ] || exit 1
fi

# 4. Check current router firmware
CURRENT_VER=$(ssh "$ROUTER_USER@$ROUTER_IP" 'cat /etc/openwrt_release | grep DISTRIB_RELEASE' 2>/dev/null || "unknown")
log_info "Router current: $CURRENT_VER"

# 5. Free space check on router
FREE_TMP=$(ssh "$ROUTER_USER@$ROUTER_IP" 'df -k /tmp | tail -1 | awk '"'"'{print $4}'"'"'' 2>/dev/null || "0")
FW_SIZE_KB=$(( $(stat -c%s "$SYSUPGRADE") / 1024 + 1 ))
if [ "$FREE_TMP" -gt "$FW_SIZE_KB" ]; then
    log_info "Router /tmp free: ${FREE_TMP}KB — Firmware needs: ${FW_SIZE_KB}KB ✅"
else
    log_error "Not enough space in /tmp (free: ${FREE_TMP}KB, need: ${FW_SIZE_KB}KB)"
    exit 1
fi

echo ""
log_warn "=== FINAL WARNING ==="
log_warn "About to flash: $(basename $SYSUPGRADE)"
log_warn "Router: $ROUTER_IP"
log_warn "The router will reboot. SSH session will drop."
echo ""
echo -n "Type 'FLASH' to confirm: "
read FINAL_CONFIRM
[ "$FINAL_CONFIRM" = "FLASH" ] || { log_info "Aborted."; exit 0; }

# ---- Upload firmware ----
log_info "Uploading firmware to router /tmp/..."
scp "$SYSUPGRADE" "$ROUTER_USER@$ROUTER_IP:/tmp/firmware.bin"
log_info "Upload complete ✅"

# ---- Flash ----
log_info "Starting sysupgrade (router will reboot in ~90 seconds)..."
ssh "$ROUTER_USER@$ROUTER_IP" 'sysupgrade -v -n /tmp/firmware.bin' || true

log_info "Waiting for router to come back online..."
sleep 30
for i in $(seq 1 20); do
    if ping -c 1 "$ROUTER_IP" &>/dev/null; then
        log_info "Router is back online after $((30 + i*5)) seconds ✅"
        sleep 5
        break
    fi
    sleep 5
    echo -n "."
done
echo ""

# ---- Post-flash verification ----
log_info "Running post-flash verification..."
bash "$(dirname "${BASH_SOURCE[0]}")/post-flash-check.sh" || log_warn "Some checks failed — review above"

echo ""
echo "╔═══════════════════════════════════════════╗"
echo "║          Flash Procedure Complete         ║"
echo "╚═══════════════════════════════════════════╝"
