# DATA MODEL SPECIFICATION — R3P HOME SERVER OS

## 1. Storage & State Strategy

The R3P Home Server OS data model separates **ephemeral runtime metrics** (stored in memory) from **system configurations** (persisted in UCI/NAND) and **application data** (stored in SQLite on external USB storage).

---

## 2. Core Entities

### 2.1. System Telemetry (`TelemetryRecord`)
```json
{
  "timestamp": 1794056230,
  "cpu": {
    "usage_percent": 12.4,
    "load_avg": [0.15, 0.22, 0.18],
    "temp_celsius": 48.5
  },
  "memory": {
    "total_bytes": 536870912,
    "used_bytes": 115343360,
    "free_bytes": 421527552,
    "cached_bytes": 62914560,
    "buffers_bytes": 14680064
  },
  "storage": [
    {
      "mount": "/",
      "filesystem": "ubifs",
      "total_bytes": 220200960,
      "used_bytes": 41943040,
      "free_bytes": 178257920
    }
  ],
  "network": {
    "rx_bytes_per_sec": 128450,
    "tx_bytes_per_sec": 42100,
    "interfaces": {
      "br-lan": { "rx_bytes": 184592010, "tx_bytes": 94820120 },
      "wan": { "rx_bytes": 845920100, "tx_bytes": 45820120 }
    }
  }
}
```

### 2.2. Network & DHCP Client (`ClientDevice`)
```json
{
  "ip": "192.168.2.100",
  "mac": "00:E0:03:45:1A:6B",
  "hostname": "DESKTOP-6ULOEKT",
  "interface": "br-lan",
  "connection_type": "ethernet",
  "lease_expires": 1794099430,
  "rx_rate_bps": 85000,
  "tx_rate_bps": 12000
}
```

### 2.3. Wireless Radio Configuration (`WirelessRadio`)
```json
{
  "radio": "radio0",
  "band": "5GHz",
  "chipset": "MediaTek MT7615",
  "enabled": true,
  "ssid": "R3P_5G_HomeServer",
  "encryption": "psk2+ccmp",
  "channel": 36,
  "htmode": "VHT80",
  "txpower_dbm": 23,
  "associated_clients": 4
}
```

### 2.4. Download Task (`DownloadTask`)
```json
{
  "gid": "2089b05e0a3d43d1",
  "status": "active",
  "name": "ubuntu-24.04-server.iso",
  "total_bytes": 2831155200,
  "completed_bytes": 1415577600,
  "download_speed_bps": 5242880,
  "upload_speed_bps": 0,
  "destination_dir": "/mnt/usb/downloads",
  "created_at": 1794055000
}
```

### 2.5. Automation Rule (`AutomationJob`)
```json
{
  "id": "job-clean-temp",
  "name": "Nightly Temporary File Cleanup",
  "enabled": true,
  "cron_expression": "0 3 * * *",
  "action_type": "script",
  "action_target": "/etc/r3p/scripts/cleanup.sh",
  "notify_channel": "telegram",
  "last_executed_at": 1794022800,
  "last_exit_code": 0
}
```
