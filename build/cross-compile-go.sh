#!/bin/bash
# R3P Firmware — Go API Cross-Compile Script for MIPS LE (MT7621)
# Run on Linux (WSL or native)
# Requires: Go 1.21+
# Usage: bash cross-compile-go.sh

set -e

API_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/source/backend"
OUTPUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/output"

echo "=== R3P Go API Cross-Compile — Phase 0 ==="
echo "Source: ${API_DIR}/main.go"
echo "Target: MIPS little-endian (softfloat, MT7621)"

cd "${API_DIR}"

# Initialize Go module if needed
if [ ! -f go.mod ]; then
  go mod init r3p-firmware/api
fi

# Cross-compile for MIPS little-endian with softfloat
# MT7621 is MIPS 1004Kc — mipsle, softfloat (no FPU)
echo ""
echo "Compiling..."
GOOS=linux \
GOARCH=mipsle \
GOMIPS=softfloat \
CGO_ENABLED=0 \
go build \
  -ldflags="-s -w -X main.BuildVersion=phase0 -X main.BuildTarget=mipsle-softfloat" \
  -trimpath \
  -o "${OUTPUT_DIR}/r3p-api-mipsle" \
  ./main.go

echo ""
echo "=== Result ==="
ls -lah "${OUTPUT_DIR}/r3p-api-mipsle"
file "${OUTPUT_DIR}/r3p-api-mipsle"

# Check if it's dynamically linked (should be static)
ldd "${OUTPUT_DIR}/r3p-api-mipsle" 2>&1 || echo "(static binary — expected)"

echo ""
echo "SHA256:"
sha256sum "${OUTPUT_DIR}/r3p-api-mipsle"

echo ""
echo "✓ Cross-compile complete"
