# SYSTEM ARCHITECTURE SPECIFICATION — R3P HOME SERVER OS (`v1.1.26`)

## 1. High-Level Architecture Overview

R3P Home Server OS employs a self-contained **Modular Monolith** designed specifically for the hardware envelope of the **Xiaomi Mi Router 3 Pro** (`MT7621AT` dual-core 880 MHz MIPS32 LE, 512 MB DDR3 RAM, 256 MB SLC NAND, USB 3.0).

```
+-------------------------------------------------------------------------+
|                              FRONTEND LAYER                             |
|    Vue 3.5 / TypeScript / Pinia / Tailwind CSS v4 / Hass Multi-Theme    |
|     Native Fetch (apiRequest/apiFetchRaw) + EventSource SSE Client      |
+-------------------------------------------------------------------------+
                                    |
                                    | HTTPS :8443 (JSON API & text/event-stream)
                                    v
+-------------------------------------------------------------------------+
|                         BACKEND LAYER (r3p-api)                         |
|         Single Static Go Binary (linux/mipsle softfloat, -s -w)         |
|                                                                         |
|  +-------------------------------------------------------------------+  |
|  | TLS 1.2+ HTTP Server | Session Auth | Rate Limiter | Same-Host CORS|  |
|  +-------------------------------------------------------------------+  |
|  |                     Core Feature Modules                          |  |
|  |  dashboard | network_config | network_speedtest | wireless        |  |
|  |  firewall  | adblock        | ai_agent          | ddns            |  |
|  |  r3p_downloader + aria2 + telegram | gdrive_sync | nas            |  |
|  |  file_explorer | apps_pkg | automation | iot | plugins | ota      |  |
|  +-------------------------------------------------------------------+  |
+-------------------------------------------------------------------------+
                                    |
                                    | ubus / uci / netifd / procd / fw4 / procfs
                                    v
+-------------------------------------------------------------------------+
|                        OPENWRT EMBEDDED LINUX                           |
|          OpenWrt 25.12.5 (ramips/mt7621) · Linux 6.12 · musl            |
+-------------------------------------------------------------------------+
```

---

## 2. Layer Definitions

### 2.1. Presentation Layer (Frontend SPA)
- **Framework:** Vue 3.5 (`<script setup lang="ts">`) + Pinia stores (`auth`, `system`, `network`) + Vue Router.
- **Theming & i18n:** Custom Hass-inspired themes (`hass-classic`, `hass-mushroom`, `hass-neon`), Dark/Light/System modes, 5 accent color palettes (`blue`, `teal`, `purple`, `green`, `orange`), and `vi`/`en`/`cn` localization in `useUiPreferences.ts`.
- **Transport:** Lightweight native `fetch` wrapper (`api.ts`: `apiRequest` & `apiFetchRaw`) with automatic `r3p:unauthorized` session invalidation + visibility-aware polling + Server-Sent Events (`useSSE.ts` -> `/api/v1/monitor/stream`). Zero external Axios or WebSocket runtime bloat.
- **Verification:** `vue-tsc -b` strict TypeScript checking + `vitest` unit test suite (`happy-dom`).

### 2.2. Backend Daemon (`source/backend` -> `/usr/bin/r3p-api`)
- **Target:** Cross-compiled statically (`GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0 -trimpath -ldflags="-s -w"`).
- **Network & Speedtest (`network_config.go`, `network_speedtest.go`):** Configures LAN/WAN/WWAN/USBWAN interfaces with subnet overlap prevention and runs real socket-bound (`SO_BINDTODEVICE`/local IP dial) multi-stream HTTP download/upload speedtests with TCP RTT/jitter measurement and concurrency locking (`409 Conflict` when busy).
- **AdBlock & DNS (`adblock.go`):** Atomically generates `/tmp/dnsmasq.d/r3p-adblock.conf` from local builtins + SSRF-checked public sources, controls `dnsmasq` `force_dns` / upstream resolvers / SafeSearch, and classifies live DNS queries from `logread`.
- **Downloader, Telegram & GDrive (`r3p_downloader.go`, `aria2.go`, `telegram.go`, `gdrive_sync.go`):** Fully local download queue powered by native HTTP/S, `yt-dlp`, and `aria2` JSON-RPC on `/mnt/sda1/downloads`, paired with Telegram long-polling message analyzer (routing pure URLs to Downloader and natural-language messages to AI) and background Google Drive sync with mutex-coalesced OAuth2 refresh.
- **AI Assistant (`ai_agent.go`):** Multi-provider OpenAI-compatible + Gemini agent with Browser Bridge option, `r3p.db` persistence for config/memory/quick-commands, session history, URL pre-filtering, `/sh` / `!` root shell routing gated by `allow_exec`, and SSRF/DNS-rebinding-checked base URLs.
- **Router Operations (`handlers_ext.go`, `apps_pkg.go`, `file_explorer.go`, `ddns.go`, `firmware_update.go`):** Covers MT7615 Wi-Fi + station management, `fw4`/UCI port forwards & traffic rules with snapshot rollback, NAS Samba4/FTP + USB 3.0 mount/format/benchmark, APK/IPK + GitHub App Manager, ICMP/TCP IoT & MQTT broker liveness probing, preset-allowlisted Plugins & strict 5-field Automation cron jobs, raw UCI editor with secret masking, `uci show` validation & `.bak` restore, sanitized backups, and verified OTA updates.

---

## 3. Storage Hierarchy & Constraints
| Medium | Mount / Path | Purpose |
|---|---|---|
| **SLC NAND (UBIFS)** | `/etc/r3p/r3p.db`, `/etc/config/`, `/www/`, `/usr/bin/r3p-api` | Embedded `bbolt` Settings DB (`0600`), UCI configs, SPA assets, Go binary |
| **RAM (`tmpfs`)** | `/tmp/` | `/tmp/dnsmasq.d/r3p-adblock.conf`, runtime logs, staged OTA/restore uploads |
| **USB 3.0 External** | `/mnt/sda1/` (ext4/exfat/vfat) | Downloaded media (`/mnt/sda1/downloads`), NAS shares, Google Drive local mirror |
