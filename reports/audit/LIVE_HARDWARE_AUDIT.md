# LIVE HARDWARE AUDIT REPORT — XIAOMI MI ROUTER 3 PRO (R3P)

## 1. Device Identification
- **Brand / Model:** Xiaomi Mi Router 3 Pro (R3P / R3D series sibling)
- **Board Name:** `xiaomi,mi-router-3-pro`
- **Target / Subtarget:** `ramips/mt7621`
- **Architecture:** MIPS 1004Kc V2.15 (Dual Core, 4 vPEs, `mipsle` little-endian)
- **Active ULA IPv6 Address:** `fd9d:7191:fe39::1`
- **Configured LAN IPv4 Address:** `192.168.2.1`
- **Current Runtime Status:** Alive and responding via HTTPS on port 8443 (`r3p-api v0.2.0-phase1`).

---

## 2. Hardware Specifications

| Subsystem | Verified Hardware Component | Operational Details |
|---|---|---|
| **SoC** | MediaTek MT7621AT | Dual-core, 4 threads @ 880 MHz |
| **RAM** | 512 MB DDR3-1200 | Nanya NT5CC256M16EP-EK |
| **NAND Flash** | 256 MB SLC NAND | ESMT `0xc8` (F59L2G81A) / Micron `0x2c` alternate |
| **2.4 GHz Wi-Fi** | MediaTek MT7615N | 802.11b/g/n, 4x4:4 MIMO, up to 800 Mbps |
| **5.0 GHz Wi-Fi** | MediaTek MT7615N | 802.11a/n/ac Wave 2, 4x4:4 MU-MIMO, up to 1733 Mbps |
| **Ethernet Switch**| MT7530 (DSA) | 1x Gigabit WAN + 3x Gigabit LAN (10/100/1000 Mbps) |
| **USB Controller**| MediaTek xHCI | 1x USB 3.0 Type-A port (supports USB Mass Storage / UASP) |
| **Status LEDs** | Tricolor GPIO LED | Orange (Boot/Error), Blue (Normal operation), Yellow |

---

## 3. Flash Memory & Partition Map (`/proc/mtd`)

```
mtd0: 00080000 00020000 "Bootloader"
mtd1: 00080000 00020000 "Config"
mtd2: 00040000 00020000 "Bdata"
mtd3: 00040000 00020000 "Factory"        <-- RF Calibration & MAC address (backed up)
mtd4: 00040000 00020000 "crash"
mtd5: 00040000 00020000 "crash_syslog"
mtd6: 00040000 00020000 "reserved0"
mtd7: 00400000 00020000 "kernel_stock"
mtd8: 00400000 00020000 "kernel"          <-- OpenWrt Kernel
mtd9: 0ec00000 00020000 "ubi"             <-- UBI rootfs_data (persistent storage)
```
*Safety Note:* Partitions `mtd0` through `mtd3` contain essential calibration data and bootloader. All writes to these partitions are strictly prohibited. Backups (`factory.bin` and `bdata.bin`) are secured in `deployment/backup/`.

---

## 4. Current Firmware State
- **Firmware Engine:** OpenWrt 25.12.5 (`r33051-f5dae5ece4`)
- **Kernel Version:** Linux 6.12.94 (`ramips/mt7621`)
- **Web Administration:** Custom Go REST Daemon `r3p-api` (Port 8443, TLS 1.3 enabled)
- **User Interface:** Vue 3 + TypeScript Dark Glassmorphism SPA (No LuCI)
- **Local Services Active:**
  - `dnsmasq` (DNS forwarder & local domain server on port 53)
  - `odhcpd` (IPv6 router advertisement & ULA prefix distributor)
  - `r3p-api` (Go system monitor & API bridge)
