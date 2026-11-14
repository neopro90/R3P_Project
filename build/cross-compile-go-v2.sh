#!/bin/bash
# ============================================================
# R3P Go API — Cross-compile helper for all targets
# Run in WSL or Linux
# ============================================================

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT_DIR/source/backend"
ARTIFACT_DIR="$ROOT_DIR/build/artifacts"
OUT_DIR="$(dirname "${BASH_SOURCE[0]}")/output"

mkdir -p "$ARTIFACT_DIR" "$OUT_DIR" "$ROOT_DIR/build/files/usr/bin"

echo "=== R3P API Cross-Compile ==="
echo "Source: $SRC_DIR"
echo "Artifact: $ARTIFACT_DIR/r3p-api"
echo "Output: $OUT_DIR"

# ---- MIPS LE (target: Xiaomi R3P / MT7621) ----
echo ""
echo "→ Building for MIPS LE softfloat (production target)..."
GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0 \
    go build -C "$SRC_DIR" \
    -ldflags="-s -w -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -trimpath \
    -o "$ARTIFACT_DIR/r3p-api" .

cp -f "$ARTIFACT_DIR/r3p-api" "$ROOT_DIR/build/files/usr/bin/r3p-api"
cp -f "$ARTIFACT_DIR/r3p-api" "$OUT_DIR/r3p-api-mipsle"

size=$(stat -c%s "$ARTIFACT_DIR/r3p-api")
echo "  ✅ r3p-api ready: $size bytes ($(echo "scale=1; $size/1024" | bc) KB)"

# Verify ELF
file "$ARTIFACT_DIR/r3p-api"

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

if curl -kfsS https://127.0.0.1:8443/api/v1/health | python3 -m json.tool; then
    echo "  ✅ API health check passed"
else
    echo "  ❌ API health check failed"
fi

kill $API_PID 2>/dev/null || true
