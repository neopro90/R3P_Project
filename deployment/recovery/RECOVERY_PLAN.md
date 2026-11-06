# RECOVERY PLAN & FAIL-SAFE PROCEDURES — XIAOMI MI ROUTER 3 PRO

## 1. Hardware Overview & Safety Architecture
The Xiaomi Mi Router 3 Pro utilizes a dual-boot capable U-Boot bootloader stored on `mtd0` (0x00000000 - 0x00080000). The bootloader contains a built-in TFTP client and USB recovery mechanism triggered by holding the hardware Reset button during cold power-on.

---

## 2. Emergency Recovery Methods

### Method 1: Xiaomi Official USB Recovery (No Serial Cable Needed)
1. Prepare a USB flash drive (FAT32 formatted, MBR partition table, 32GB or smaller recommended).
2. Download or copy the recovery ROM (e.g., `miwifi_r3p_firmware.bin` or OpenWrt stock factory uImage) to the root directory of the USB drive.
3. Rename the firmware image to `miwifi.bin`.
4. Disconnect power cable from R3P.
5. Insert the USB drive into the R3P USB 3.0 port.
6. Using a paperclip or pin, **press and hold the Reset button**.
7. Plug in the power cord while keeping the Reset button held.
8. When the front LED turns from **Solid Yellow** to **Blinking Yellow**, release the Reset button.
9. R3P will read `miwifi.bin` from the USB drive, flash the kernel and rootfs, and reboot automatically (takes 2–3 minutes). Front LED will turn **Solid Blue** upon completion.

---

### Method 2: U-Boot Serial / TFTP Recovery
If the kernel fails to boot and USB recovery does not trigger:
1. Connect a 3.3V USB-to-TTL UART adapter to the router PCB (Header J1: GND, TX, RX - baud rate 115200 8N1).
2. Set computer static IP to `192.168.1.2`, subnet `255.255.255.0`.
3. Launch a TFTP server (e.g., `tftpd32` / `tftpd64`) hosting the factory firmware image `openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-factory.bin`.
4. Power on router and interrupt U-Boot by pressing `4` or spacebar.
5. Run U-Boot TFTP command:
   ```sh
   tftpboot 0x80001000 openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-factory.bin
   bootm 0x80001000
   ```

---

## 3. Restoring Factory Calibration Data
If `mtd3` (Factory) or `mtd2` (Bdata) is ever corrupted:
The verified backups are stored locally on the host machine:
- `D:\R3P_Project\deployment\backup\factory.bin` (262,144 bytes, SHA256 verified)
- `D:\R3P_Project\deployment\backup\bdata.bin` (262,144 bytes, SHA256 verified)
- `D:\R3P_Project\deployment\backup\backup-config.tar.gz` (OpenWrt configuration archive)

Restoration command over SSH:
```sh
mtd write /tmp/factory.bin /dev/mtd3
```
*(Warning: Never overwrite `mtd3` unless restoring the device's exact original backup).*
