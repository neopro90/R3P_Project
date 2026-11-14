# API SPECIFICATION — R3P HOME SERVER OS (`v1.1.26`)

Base URL: `https://<router-ip>:8443/api/v1`

Every JSON response wraps data in a uniform envelope:
- Success: `{ "ok": true, "data": <payload> }`
- Error: `{ "ok": false, "error": "<message>" }`

All endpoints except `GET /health` and `/auth/session` require a valid `r3p_session` session cookie and same-host origin.

---

## 1. Authentication & Telemetry
| Method | Route | Description |
|---|---|---|
| `GET` | `/health` | Unauthenticated liveness status (`version`, `arch`, `os`, `uptime_seconds`) |
| `GET` | `/auth/session` | Check active session status |
| `POST` | `/auth/session` | Authenticate with `/etc/shadow` Linux password or 64-char API token (`r3p_session` cookie) |
| `DELETE` | `/auth/session` | Revoke active session |
| `GET` | `/monitor/stream` | SSE live telemetry stream (`text/event-stream`) |
| `GET` | `/system` | System summary (`hostname`, `kernel`, `arch`, `version`, `hardware`, `lan_ip`, `timezone`, `internet`) |
| `GET` | `/memory` | `/proc/meminfo` & `/proc/iomem` breakdown (`physical_total_kb`, `reserved_kb`, `total_kb`, `used_kb`, `used_percent`) |

## 2. Network, Speedtest, Wireless & Firewall
| Method | Route | Description |
|---|---|---|
| `GET` | `/network` | Interface RX/TX byte counters and link status |
| `GET` | `/network/details` | Kernel routing table, active `/etc/resolv.conf` resolvers, ARP neighbors |
| `GET` | `/network/internet` | Live WAN reachability probe |
| `GET \| PUT` | `/network/interfaces-config` | View/save LAN, WAN, WWAN, USBWAN & quick gateway/DNS (enforces subnet overlap check) |
| `POST` | `/network/interfaces-config/action` | Bring interface `ifup` / `ifdown` or `reload` network stack |
| `POST` | `/network/speedtest?interface=wan` | Bound multi-stream HTTP download/upload speedtest + TCP RTT/jitter (returns `409` if test already active) |
| `GET \| PUT` | `/wireless` | Dual-band MT7615 radio configuration (`key` is masked on `GET`) |
| `POST` | `/wireless/restart` | Reload wireless radios |
| `GET` | `/wireless/scan` | Nearby SSID scan + client Wi-Fi management (`/wireless/client`, `/wireless/saved*`) |
| `GET` | `/firewall` | Active UCI zones, NAT redirects, traffic rules & conntrack stats |
| `POST \| PATCH \| DELETE` | `/firewall/redirect` | Manage NAT port-forwarding rules (IPv4 + port validation, auto-rollback on commit error) |
| `POST \| PATCH \| DELETE` | `/firewall/rule` | Manage firewall traffic rules (`ACCEPT`/`DROP`/`REJECT`, `tcp`/`udp`/`all`) |

## 3. AdBlock, Access, DDNS & AI Assistant
| Method | Route | Description |
|---|---|---|
| `GET` | `/adblock` | AdBlock engine stats, DNS listening check, sources, whitelist & blacklist |
| `PUT` | `/adblock/config` | Update `enabled`, `force_dns`, `block_mode`, `upstream_dns`, `safe_search`, `auto_update` |
| `POST` | `/adblock/update` | Force refresh remote blocklists (SSRF checked) & reload `dnsmasq` |
| `POST \| PATCH \| DELETE` | `/adblock/lists` | Add/toggle/remove custom blocklist URLs |
| `POST \| DELETE` | `/adblock/domains` | Manage whitelist/blacklist entries & `/adblock/test` domain lookup |
| `GET` | `/adblock/streams` | Live DNS stream inspector + `DELETE` stats reset |
| `GET` | `/access` | Dropbear SSH, DHCP pool, active/static leases, masked DDNS state |
| `PUT` | `/access/ssh` | Update SSH port & password/root login policy |
| `PUT` | `/access/dhcp` | Configure `dnsmasq` LAN DHCP pool |
| `POST \| DELETE` | `/access/static-lease` | Add/remove MAC-bound DHCP static leases |
| `POST` | `/access/password` | Synchronize Linux `root` password |
| `GET \| PUT` | `/access/ddns` | Configure DuckDNS/Cloudflare/No-IP/DynDNS/Custom DDNS + `POST /access/ddns/update` |
| `GET \| PUT` | `/ai/config` | Configure LLM provider, base URL, model & execution toggles (`api_key` masked) |
| `POST \| DELETE` | `/ai/chat` | Interactive chat, Browser Bridge continuation, bare-URL forwarding, `/sh ` commands & clear chat |
| `GET \| POST \| PUT \| DELETE` | `/ai/sessions` | Manage chat sessions, `/ai/memory` notes, `/ai/blueprint`, `/ai/quick-commands` |

## 4. Downloads, Telegram, GDrive, NAS & Files
| Method | Route | Description |
|---|---|---|
| `GET \| POST \| DELETE` | `/download/jobs` | Manage native `yt-dlp` / Aria2 download queue (`/download/jobs/{id}`, `/retry`) |
| `GET \| POST` | `/download/files` | Browse USB download library & `GET /download/file?sub_path=...` authenticated download |
| `GET \| PUT` | `/download/settings`, `/download/config` | Configure output directory, concurrency & quality preferences |
| `GET \| PUT \| DELETE` | `/telegram`, `/download/telegram/*` | Configure Telegram bot polling, URL vs AI auto-router & logs |
| `GET \| POST \| PUT` | `/nas/gdrive-sync` | Configure Google Drive sync mappings & OAuth credentials (`POST /nas/gdrive-sync/run`, `/refresh`, `/clear-log`) |
| `GET` | `/usb/status`, `/nas/overview` | Disk partitions, temperatures & Samba4/FTP status (`POST /usb/mount`, `/usb/format`, `/usb/benchmark`) |
| `GET \| POST \| PUT \| DELETE` | `/nas/services`, `/nas/shares`, `/nas/users` | Manage Samba4/FTP daemons, shared folders & UNIX/SMB accounts (`/nas/upload`, `/nas/download`, `/nas/mkdir`) |
| `GET \| POST` | `/explorer/list`, `/explorer/read`, `/explorer/write`, `/explorer/create`, `/explorer/delete`, `/explorer/raw` | Sandboxed Router File Explorer (blocks secret configs, roots & symlinks) |

## 5. Applications, Automation, IoT, Plugins, UCI & OTA
| Method | Route | Description |
|---|---|---|
| `GET \| POST` | `/services/all`, `/services/control` | Inspect procd services & control (`r3p-api` protected from stop/disable) |
| `GET \| POST \| DELETE` | `/packages`, `/packages/repos`, `/packages/search`, `/packages/install`, `/packages/remove` | APK/IPK package manager & GitHub repo inspector |
| `GET \| POST \| PATCH \| DELETE` | `/automation/cron` | Manage root crontab schedules + `POST /automation/run` |
| `GET \| POST \| PUT \| DELETE` | `/iot`, `POST /iot/ping` | IoT LAN device & MQTT broker monitoring |
| `GET \| POST` | `/plugins` | Preset-allowlisted system plugins (`ram_flush`, `lan_probe`, `nand_health`, `dns_reload`, etc.) |
| `GET \| PUT` | `/config`, `GET /config/backups`, `POST /config/restore` | Direct UCI editor with `uci show` syntax validation, `.bak` backup & restore |
| `GET` | `/system/backup` | Download sanitized `.tar.gz` config archive (includes sanitized `r3p.db` snapshot; add `?include_secrets=1` for full backup) |
| `GET \| POST` | `/system/db` | Inspect embedded `/etc/r3p/r3p.db` bucket counts/size and run `sync` / `migrate` |
| `GET \| PUT \| POST` | `/ui/preferences` | Persist UI theme, locale, layout, and dashboard state in `r3p.db` |
| `POST` | `/system/restore`, `/system/settings`, `/system/drop-caches`, `/system/reboot` | Validated archive restore, hostname/timezone update & power controls |
| `GET \| PUT \| POST` | `/system/firmware/status`, `/config`, `/github-check`, `/github-update`, `/upload`, `/apply` | GitHub/manual OTA updater with SHA-256, MIPSLE ELF32 & tar safety verification |
