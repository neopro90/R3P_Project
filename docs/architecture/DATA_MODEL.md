# DATA MODEL SPECIFICATION — R3P HOME SERVER OS (`v1.1.26`)

## 1. Storage & Persistence Model

R3P Home Server OS stores live telemetry strictly in memory (parsed from Linux `/proc` and `/sys`, including `/proc/iomem` physical DDR3 bounds), OpenWrt system configuration in `/etc/config/*` via UCI with `/etc/r3p/backups/*.bak` snapshots, all firmware module settings, collections, and UI preferences in an ACID embedded `bbolt` database at `/etc/r3p/r3p.db` (`0600`, auto-migrating legacy `/etc/r3p/*.json` files on startup), and bulk user media on USB 3.0 storage (`/mnt/sda1/`).

---

## 2. Core Data Entities

### 2.1. SSE Live Telemetry Payload (`/api/v1/monitor/stream`)
```json
{
  "cpu": 12.4,
  "ram": 28.5,
  "ramTotal": 512,
  "ramUsed": 145.9,
  "temp": 48.5,
  "uptime": 86400,
  "loadAvg": [0.18, 0.22, 0.19],
  "hostname": "R3P-Router",
  "appVersion": "v1.1.26"
}
```

### 2.2. Network Speedtest Result (`POST /api/v1/network/speedtest`)
```json
{
  "interface": "wan",
  "device": "eth0.2",
  "local_ip": "192.168.1.50",
  "server": "Cloudflare Edge",
  "download_mbps": 248.4,
  "upload_mbps": 182.1,
  "upload_supported": true,
  "latency_ms": 12.3,
  "jitter_ms": 1.8,
  "tested_at": "2026-11-14T06:30:00+07:00"
}
```

### 2.3. Download Job (`R3pDownloadJob`)
```json
{
  "id": "job-a1b2c3",
  "url": "https://example.com/video",
  "title": "Sample Media",
  "quality": "1080p",
  "engine": "yt-dlp",
  "status": "DOWNLOADING",
  "progress": 45.2,
  "speed": "8.4 MiB/s",
  "eta": "00:18",
  "output_path": "/mnt/sda1/downloads/Sample.mp4"
}
```

### 2.4. DDNS Configuration (`DDNSConfig` — `/etc/r3p/ddns.json`)
```json
{
  "enabled": true,
  "provider": "duckdns",
  "domain": "r3phome",
  "token": "••••1234",
  "check_interval": 10,
  "last_ip": "203.0.113.10",
  "last_status": "OK",
  "last_update": "2026-11-14 06:15:00"
}
```

### 2.5. System Plugin Manifest (`PluginManifest` — `/etc/r3p/plugins.json`)
```json
{
  "id": "plugin-nand-health",
  "name": "ESMT SLC NAND UBI Inspector",
  "version": "1.0.2",
  "author": "R3P Core",
  "description": "Reads UBI volume wear-leveling and bad block counters",
  "enabled": true,
  "command": "nand_health",
  "last_output": "...",
  "last_run": "2026-11-14 06:20:00"
}
```
> Only the 10 preset commands enumerated in `allowedPluginPresets` (`ram_flush`, `flush_ram_cache`, `lan_probe`, `nand_health`, `dns_reload`, `reload_dnsmasq`, `disk_usage`, `uptime_check`, `listening_sockets`, `ntp_sync`) are permitted on execution.

### 2.6. Release Metadata (`release.json` / `/etc/r3p/version.json`)
```json
{
  "version": "v1.1.26",
  "revision": 26,
  "released_at": "2026-11-14 08:45:00",
  "repo_url": "https://github.com/neopro90/R3P_Project.git",
  "branch": "main",
  "commit_sha": "v1.1.26"
}
```
