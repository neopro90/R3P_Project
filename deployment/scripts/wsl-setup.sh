#!/bin/bash
# ============================================================
# R3P WSL Setup Script — Run AFTER system reboot
# Execute in WSL Ubuntu: bash /mnt/d/R3P_Project/scripts/wsl-setup.sh
# ============================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_step()  { echo -e "${BLUE}[STEP]${NC} $*"; }

PROJECT_WIN="D:\\R3P_Project"
PROJECT_WSL="/mnt/d/R3P_Project"

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║   R3P Firmware — WSL Build Environment  ║"
echo "╚══════════════════════════════════════════╝"
echo ""

# ---- 1. Check WSL environment ----
log_step "1/6 Checking WSL environment..."
if [ ! -d "/mnt/d" ]; then
    log_error "/mnt/d not accessible. Is WSL2 running and D: mounted?"
    exit 1
fi

if [ ! -d "$PROJECT_WSL" ]; then
    log_error "Project not found at $PROJECT_WSL"
    exit 1
fi
log_info "Project found at $PROJECT_WSL ✅"

# ---- 2. Install dependencies ----
log_step "2/6 Installing build dependencies..."
sudo apt-get update -qq
sudo apt-get install -y -qq \
    build-essential \
    zstd \
    python3 \
    python3-distutils \
    unzip \
    libncurses5 \
    git \
    rsync \
    wget \
    curl \
    file \
    xxd \
    upx-ucl \
    golang-go

log_info "Dependencies installed ✅"

# ---- 3. Check Go version ----
log_step "3/6 Checking Go toolchain..."
go version
# Also check if Windows Go is accessible (for cross-compile)
if /mnt/c/Program\ Files/Go/bin/go version 2>/dev/null; then
    log_info "Windows Go also accessible ✅"
fi

# ---- 4. Cross-compile Go API ----
log_step "4/6 Cross-compiling Go API for MIPS LE..."
cd "$PROJECT_WSL/build"
bash cross-compile-go-v2.sh
log_info "Go API compiled ✅"

# ---- 5. Extract and run ImageBuilder ----
log_step "5/6 Extracting ImageBuilder..."
IB_DIR="$PROJECT_WSL/build/imagebuilder"
mkdir -p "$IB_DIR"

TARBALL="$PROJECT_WSL/Supplementary Files/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64.tar.zst"

if [ ! -d "$IB_DIR/openwrt-imagebuilder-25.12.5-ramips-mt7621.Linux-x86_64" ]; then
    log_info "Extracting ImageBuilder (this takes ~2-3 minutes)..."
    tar -xf "$TARBALL" -C "$IB_DIR/"
    log_info "Extraction complete ✅"
else
    log_info "ImageBuilder already extracted ✅"
fi

# ---- 6. Run build ----
log_step "6/6 Running ImageBuilder..."
bash "$PROJECT_WSL/build/build-r3p-v2.sh"

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║   🎉 WSL Setup and Build Complete!       ║"
echo "╚══════════════════════════════════════════╝"
echo ""
log_info "Firmware images are in: $PROJECT_WSL/build/output/"
ls -lh "$PROJECT_WSL/build/output/"*.bin 2>/dev/null || log_warn "No .bin files yet"
