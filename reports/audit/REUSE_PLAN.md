# REUSE PLAN — R3P HOME SERVER OS

## 1. Strategy Overview
Rather than reinventing functional components from scratch, the project maximizes reuse of proven, validated assets already tested on MT7621 architecture while systematically refining them to meet the Home Server OS specification.

## 2. Reusable Assets & Enhancement Path

### 2.1. Backend (`source/backend/`)
- **Direct Reuse:**
  - Standard library architecture (no heavy CGO or dynamic dependencies).
  - High-performance procfs parsers for CPU, RAM, and network counters.
  - Constant-time token authentication in `auth.go`.
  - Self-signed TLS certificate generation in `cert.go`.
- **Enhancements Planned:**
  - Add WebSocket stream endpoint for ultra-low latency real-time telemetry (TASK 018).
  - Expand system adapters for full ubus/UCI configuration writes with automated rollback (TASK 019).
  - Integrate SQLite persistent event store on USB storage (TASK 008).

### 2.2. Frontend (`source/frontend/`)
- **Direct Reuse:**
  - Vue 3 + TypeScript architecture.
  - Dark Glassmorphism aesthetic matching the user requirement.
  - Responsive mobile drawer and desktop sidebar navigation.
  - Production build pipeline producing an ultra-lightweight static bundle (~140 KB JS + 33 KB CSS).
- **Enhancements Planned:**
  - Complete WebSocket real-time binding across all 13 view tabs.
  - Add form validation and confirmation modal dialogs for critical system actions.

### 2.3. Firmware Platform (`Supplementary Files/`)
- **Direct Reuse:**
  - OpenWrt 25.12.5 ImageBuilder (`ramips/mt7621`).
  - Native kernel modules for MT7615 Wi-Fi, USB 3.0, and Ext4/VFAT filesystems.
  - Pre-built package feeds (`kmods/6.12.94-1-...`).
- **Enhancements Planned:**
  - Strict exclusion of LuCI packages (`-luci* -liblucihttp*`).
  - Embedded procd service scripts for `r3p-api` and `r3p-aria2`.
