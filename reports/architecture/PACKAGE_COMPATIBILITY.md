# PACKAGE COMPATIBILITY & RESOURCE BUDGET — R3P HOME SERVER OS

## 1. Resource Budget

The Xiaomi Mi Router 3 Pro provides **512 MB DDR3 RAM** and **256 MB SLC NAND Flash**.

### Profile Allocations:

| Resource | Base System (Essential) | Home Server Profile | Advanced / Peak Load | Hardware Maximum |
|---|:---:|:---:|:---:|:---:|
| **Linux Kernel & Buffers** | 35 MB | 45 MB | 60 MB | - |
| **Network Core (netifd/dnsmasq)**| 12 MB | 16 MB | 20 MB | - |
| **Go API Daemon (`r3p-api`)** | 12 MB | 16 MB | 24 MB | - |
| **Aria2 Download Engine** | - | 25 MB | 50 MB (capped) | - |
| **Samba / SFTP File Sharing** | - | 30 MB | 65 MB | - |
| **Adblock / DNS Cache** | - | 18 MB | 30 MB | - |
| **Free / Page Cache Margin** | **453 MB** | **362 MB** | **263 MB** | **512 MB Total** |
| **NAND Flash Usage** | ~18 MB | ~28 MB | ~45 MB | **256 MB Total** |

*Verdict:* The 512 MB RAM capacity of R3P provides ample headroom for all planned Home Server microservices without triggering kernel OOM (Out Of Memory) killer.

---

## 2. Go Toolchain Compatibility (TASK 012)
- **Target OS:** `linux`
- **Target Architecture:** `mipsle` (MIPS 32-bit little-endian)
- **Floating Point:** `GOMIPS=softfloat` (MT7621 1004Kc does not have hardware FPU)
- **CGO Dependency:** `CGO_ENABLED=0` (Produces fully static binary, zero runtime musl dependency)
- **Binary Footprint:** ~2.3 MB stripped (`-ldflags="-s -w"`)
- **Compatibility Status:** **VERIFIED & TESTED LIVE** on device kernel 6.12.94.

---

## 3. Package Compatibility Matrix (TASK 013)

| Package Name | Architecture | Repository Feed | Function | Compatibility Status |
|---|---|---|---|:---:|
| `kmod-mt7615e` | `mipsel_24kc` | Base Kernel | Wi-Fi 2.4/5GHz Hardware Driver | **PASS** (Built-in) |
| `kmod-usb3` | `mipsel_24kc` | Base Kernel | USB 3.0 xHCI Controller Driver | **PASS** (Built-in) |
| `kmod-usb-storage` | `mipsel_24kc` | Base Kernel | Mass Storage Device Support | **PASS** (Built-in) |
| `kmod-fs-ext4` | `mipsel_24kc` | Base Kernel | Linux Ext4 Filesystem Driver | **PASS** (Built-in) |
| `kmod-fs-vfat` | `mipsel_24kc` | Base Kernel | FAT32 / ExFAT USB Filesystems | **PASS** (Built-in) |
| `kmod-nft-core` | `mipsel_24kc` | Base Kernel | Netfilter Core for Firewall4 | **PASS** (Built-in) |
| `nftables` | `mipsel_24kc` | Packages | Modern Packet Filtering CLI | **PASS** (In image) |
| `dnsmasq` | `mipsel_24kc` | Base System | DNS Forwarder & DHCP Server | **PASS** (In image) |
| `dropbear` | `mipsel_24kc` | Base System | Lightweight SSH Server | **PASS** (In image) |
| `aria2` | `mipsel_24kc` | Packages | Multi-protocol Download Daemon | **PASS** (Available in feed) |
| `samba4-server` | `mipsel_24kc` | Packages | SMB3 File Sharing Daemon | **PASS** (Available in feed) |
| `adblock-fast` | `mipsel_24kc` | Packages | Low-memory DNS Adblocker | **PASS** (Available in feed) |
| `uhttpd` | `mipsel_24kc` | Base System | Static SPA Web Server | **PASS** (In image) |
| `luci*` | Any | LuCI Feed | Legacy Web Interface | **EXCLUDED** (Per project design) |
