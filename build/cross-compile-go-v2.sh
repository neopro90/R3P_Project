#!/bin/bash
# ============================================================
# R3P Go API — Cross-compile helper for all targets
# Run in WSL or Linux
# ============================================================

set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/source/backend"
OUT_DIR="$(dirname "${BASH_SOURCE[0]}")/output"

mkdir -p "$OUT_DIR"

echo "=== R3P API Cross-Compile ==="
echo "Source: $SRC_DIR"
echo "Output: $OUT_DIR"

# ---- MIPS LE (target: Xiaomi R3P / MT7621) ----
echo ""
echo "→ Building for MIPS LE softfloat (production target)..."
GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0 \
    go build -C "$SRC_DIR" \
    -ldflags="-s -w -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -trimpath \
    -o "$OUT_DIR/r3p-api-mipsle" .

command -v upx >/dev/null || {
    echo "UPX is required to compress the MIPSLE API; install upx-ucl." >&2
    exit 1
}
upx --best --lzma "$OUT_DIR/r3p-api-mipsle"
upx -t "$OUT_DIR/r3p-api-mipsle"
size=$(stat -c%s "$OUT_DIR/r3p-api-mipsle")
echo "  ✅ r3p-api-mipsle UPX-packed: $size bytes ($(echo "scale=1; $size/1024" | bc) KB)"

# Verify ELF
file "$OUT_DIR/r3p-api-mipsle"

# ---- Linux amd64 (test reference / WSL test) ----
echo ""
echo "→ Building for Linux amd64 (dev/test reference)..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -C "$SRC_DIR" \
    -ldflags="-s -w -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -trimpath \
    -o "$OUT_DIR/r3p-api-linux-amd64" .

size=$(stat -c%s "$OUT_DIR/r3p-api-linux-amd64")
echo "  ✅ r3p-api-linux-amd64: $size bytes ($(echo "scale=1; $size/1024" | bc) KB)"

echo ""
echo "=== All builds complete ==="
ls -lh "$OUT_DIR"/r3p-api-*

# ---- Quick test (amd64) ----
echo ""
echo "→ Quick API test..."
"$OUT_DIR/r3p-api-linux-amd64" &
API_PID=$!
sleep 1

if curl -sf http://127.0.0.1:8080/api/v1/health | python3 -m json.tool; then
    echo "  ✅ API health check passed"
else
    echo "  ❌ API health check failed"
fi

kill $API_PID 2>/dev/null || true
