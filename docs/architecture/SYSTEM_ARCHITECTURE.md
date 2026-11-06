# SYSTEM ARCHITECTURE SPECIFICATION — R3P HOME SERVER OS

## 1. High-Level Architecture Overview

R3P Home Server OS employs a decoupled **Modular Monolith** architecture designed specifically for the resource footprint of the Xiaomi Mi Router 3 Pro (MT7621AT SoC, 512MB RAM, 256MB SLC NAND).

```
+-------------------------------------------------------------------------+
|                              FRONTEND LAYER                             |
|       Vue 3.4+ / TypeScript / Vite / Tailwind CSS / Glassmorphism       |
|                  Responsive Web SPA & PWA (Desktop & Mobile)             |
+-------------------------------------------------------------------------+
                                    |
                                    | REST (JSON) & WebSocket (SSE / WS)
                                    v
+-------------------------------------------------------------------------+
|                              BACKEND LAYER                              |
|                 r3p-api: Native Go Daemon (mipsle static)               |
|                                                                         |
|  +-------------------------------------------------------------------+  |
|  |                  API Gateway & Routing Engine                     |  |
|  +-------------------------------------------------------------------+  |
|  | Authentication | Authorization | Rate Limiting | Audit Logging    |  |
|  +-------------------------------------------------------------------+  |
|  |                          Core Services                            |  |
|  |  [Dashboard] [Network] [Wireless] [Firewall] [Storage & NAS]      |  |
|  |  [Downloads] [Automation] [Applications] [IoT] [Plugins] [System] |  |
|  +-------------------------------------------------------------------+  |
+-------------------------------------------------------------------------+
                                    |
                                    | System Adapter Layer
                                    v
+-------------------------------------------------------------------------+
|                          SYSTEM ADAPTER LAYER                           |
|       ubus-cli / UCI engine / netifd / procd / nftables / sysfs / procfs|
+-------------------------------------------------------------------------+
                                    |
                                    v
+-------------------------------------------------------------------------+
|                        OPENWRT EMBEDDED LINUX                           |
|      OpenWrt 25.12.5 Base / Linux Kernel 6.12.94 / musl libc / MT7621   |
+-------------------------------------------------------------------------+
```

---

## 2. Layer Definitions

### 2.1. Presentation Layer (Frontend SPA)
- **Framework:** Vue 3 Composition API with `<script setup lang="ts">`.
- **State Management:** Reactive stores using Pinia / Vue Reactivity.
- **Styling:** Custom Glassmorphism design system (`#0d1117` base, frosted glass borders, CSS backdrop filters, responsive grid).
- **Communication:** Axios HTTP client for REST APIs + Native WebSocket for live streaming telemetry.
- **Footprint:** Production bundle compressed under 200 KB total; served directly by backend or lightweight uhttpd.

### 2.2. Service & Gateway Layer (Go Modular Monolith)
- **Runtime:** Native single static binary compiled for `mipsle` (`softfloat`).
- **Memory Footprint:** Resident Set Size (RSS) under 18 MB.
- **Concurrency Model:** Goroutines with bounded worker pools to prevent CPU starvation on MT7621.
- **Core Modules:**
  - `AuthService`: Constant-time secret verification, 8-hour session tokens, IP-based rate limiting.
  - `MonitorService`: Zero-overhead `/proc` and `/sys` telemetry parser emitting 1-second system metrics.
  - `NetworkService`: Bridged interface statistics, DHCP lease table, DNS resolver diagnostics.
  - `WirelessService`: Dual-band MT7615 radio configuration via `iwinfo` and `uci`.
  - `FirewallService`: Validated zone rules, port forwarder, and active connection tracking via `nftables`.
  - `StorageService`: USB mass-storage hotplug detector, mount controller, and Samba / SFTP share manager.
  - `DownloadService`: Aria2 JSON-RPC client bridge with download queue, speed limiter, and storage selector.
  - `AutomationService`: Cron scheduler, webhook receiver, and Telegram alert dispatcher.
  - `SystemService`: Firmware sysupgrade trigger, NTP sync, process supervisor, and reboot/shutdown handler.

### 2.3. System Adapter Layer
To guarantee system stability and prevent arbitrary command execution:
1. **No direct shell execution from user input.**
2. All operating system modifications pass through typed Go adapter methods.
3. Every command is executed via `exec.CommandContext` with strict timeouts (max 5 seconds).
4. Command arguments are passed as discrete token arrays, eliminating shell injection vectors.
5. All configuration mutations invoke `uci commit` followed by targeted service reload (`procd`).

---

## 3. Storage Hierarchy & Partition Policy

| Storage Medium | Path | Persistence | Allocated Role |
|---|---|---|---|
| **NAND Flash (UBI)** | `/etc/`, `/root/` | Persistent | OS binaries, system configurations, TLS certificates |
| **RAM (tmpfs)** | `/tmp/`, `/var/run/` | Volatile | Process PID files, ephemeral telemetry cache, DHCP leases |
| **USB 3.0 External** | `/mnt/usb/` | Persistent | Download payloads, media files, SQLite persistent event database |
