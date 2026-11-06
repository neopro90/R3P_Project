# BASELINE & CUSTOM BUILD REPORT — R3P HOME SERVER OS

## 1. Build Metadata
- **Build Timestamp:** 2026-11-06 16:40:23
- **Profile:** `xiaomi_mi-router-3-pro`
- **Target Platform:** `ramips/mt7621`
- **Base Version:** OpenWrt 25.12.5 (`r33051-f5dae5ece4`)
- **Kernel Version:** 6.12.94 (`mipsel_24kc`)
- **Build Output Directory:** `build/output/custom-20261106-164023/`

---

## 2. Generated Artifacts & Integrity Hashes

| Artifact Type | File Name | Size (Bytes) | SHA-256 Checksum |
|---|---|---:|---|
| **Factory Image** | `openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-factory.bin` | 15,597,568 | `ee6bc919a2776332a2d521356dfeb6ce6bc7dffed5a4ec705785a78b0896a95f` |
| **Sysupgrade Image** | `openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-sysupgrade.bin` | 13,374,039 | `262a0e976eaeb59a7ac177ad1b8c63433e8dd8bf85f2578a5b888620c236e7ff` |
| **Package Manifest** | `openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro.manifest` | 4,210 | Verified |
| **BOM CycloneDX** | `openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro.bom.cdx.json` | 98,450 | Verified |

---

## 3. Structural & Payload Verification
- **Kernel Partition Usage:** 3,321,277 bytes (79.2% of 4,194,304 byte limit). Margin: 873 KB free.
- **Root Filesystem (SquashFS):** 10,046,464 bytes (~9.58 MB xz compressed).
- **LuCI Status:** **100% REMOVED** (`-luci* -liblucihttp*`). Zero legacy LuCI modules present.
- **Embedded Custom Services:**
  - `usr/bin/r3p-api`: UPX-compressed static Go binary (~2.3 MB).
  - `www/`: Vue 3 + TypeScript production bundle (HTML, CSS, JS).
  - `etc/uci-defaults/99-r3p-setup`: Configures `192.168.2.1/24`, `netmask 255.255.255.0`, dual-stack Dropbear.
