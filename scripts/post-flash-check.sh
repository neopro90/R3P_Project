#!/bin/bash
# ============================================================
# R3P — Post-flash verification script
# Run via SSH on the router AFTER flashing firmware
# Usage: ssh root@192.168.1.1 'bash -s' < post-flash-check.sh
# ============================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { echo -e "${GREEN}✅${NC} $*"; }
warn() { echo -e "${YELLOW}⚠️${NC}  $*"; }
fail() { echo -e "${RED}❌${NC} $*"; }

echo ""
echo "=== R3P Post-Flash Verification ==="
echo ""

# ---- Basic system ----
echo "--- System ---"
hostname && ok "Hostname: $(hostname)" || fail "hostname"
uname -a && ok "Kernel running" || fail "kernel"
uptime && ok "Uptime OK" || fail "uptime"

echo ""
echo "--- OpenWrt Version ---"
cat /etc/openwrt_release | grep -E "DISTRIB_(RELEASE|REVISION)" && ok "Version read" || fail "version"

echo ""
echo "--- NAND Chip Check ---"
NAND_ID=$(dmesg | grep -i "Manufacturer ID" | head -1 || echo "NOT FOUND")
echo "NAND dmesg: $NAND_ID"
if echo "$NAND_ID" | grep -q "0xc8"; then
    ok "NAND chip: ESMT (safe) — Manufacturer ID 0xc8"
elif echo "$NAND_ID" | grep -q "0x2c"; then
    warn "NAND chip: Micron (0x2c) — extra caution with bad blocks!"
else
    warn "NAND Manufacturer ID not found in dmesg — check manually: dmesg | grep -i nand"
fi

echo ""
echo "--- LuCI Removal Check ---"
if opkg list-installed 2>/dev/null | grep -q "^luci "; then
    fail "LuCI is still installed! Removal failed."
elif apk list --installed 2>/dev/null | grep -q "^luci-"; then
    fail "LuCI package found via apk!"
else
    ok "LuCI not installed ✅"
fi

echo ""
echo "--- R3P API Check ---"
if [ -x /usr/bin/r3p-api ]; then
    ok "r3p-api binary present and executable"
    file /usr/bin/r3p-api
else
    fail "r3p-api NOT found at /usr/bin/r3p-api"
fi

if pgrep -x r3p-api > /dev/null; then
    ok "r3p-api process running (PID: $(pgrep -x r3p-api))"
else
    fail "r3p-api is NOT running"
fi

# ---- API Health Check ----
echo ""
echo "--- API Health Check ---"
if (command -v curl >/dev/null 2>&1 && curl -kfsS https://127.0.0.1:8443/api/v1/health) || wget -qO- --no-check-certificate https://127.0.0.1:8443/api/v1/health 2>/dev/null; then
    echo ""
    ok "API health endpoint responding"
else
    fail "API health endpoint NOT responding"
fi

if [ -f /etc/r3p/r3p.db ]; then
    ok "/etc/r3p/r3p.db present"
else
    warn "/etc/r3p/r3p.db not initialized yet"
fi

echo ""
echo "--- Web UI Check ---"
if [ -f /www/index.html ]; then
    ok "/www/index.html present"
    wc -c /www/index.html
else
    fail "/www/index.html NOT found"
fi

echo ""
echo "--- Services ---"
for svc in dropbear dnsmasq uhttpd rpcd; do
    if /etc/init.d/$svc status 2>/dev/null | grep -q "running"; then
        ok "$svc: running"
    else
        warn "$svc: not running (may be normal)"
    fi
done

echo ""
echo "--- Storage ---"
df -h | grep -v tmpfs
echo ""
echo "--- Memory ---"
free -m

echo ""
echo "=== Verification Complete ==="
