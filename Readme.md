Supported Versions
Brand
Model
Version
Supported Current Rel
OEM Device Homepage URL
OWrt Forum Topic URL
Xiaomi
Mi Router 3 Pro
25.12.2
mi.com
Discourse: support-for-xiaomi-wifi-r3p-pro
Edit

Hardware Highlights
Version
CPU
CPU Cores_numcores
CPU MHz
Flash MB
RAM MB
WLAN Hardware
WLAN 2.4GHz
WLAN 5.0GHz
Ethernet 1Gbit ports
USB ports
MediaTek MT7621AT
2
880
256NAND
512
MediaTek MT7615
b/g/n
a/n/ac
4
1x 3.0

Installation
Model
Version
Supported Current Rel
Firmware OpenWrt Install URL
Firmware OpenWrt Upgrade URL
Firmware OEM Stock URL
Mi Router 3 Pro
25.12.2
Factory image
Sysupgrade image

NAND chip warning
Users have reported issues with the models that use the Micron NAND chip. Particularly those that have bad blocks. Until the problem has been resolved, try to acquire a model whose NAND chip is manufactured by ESMT since your aim is to install OpenWrt. That is of course easier said than done, so if you do end up with a Micron-NAND version, the procedure suggested and the end of this forum post may get you going. (Having a 3.3V UART handy in case things fall apart is recommended.)

You can determine the NAND chip on your router by looking at the bootlog message (by ssh'ing to your router and running dmesg. If you're feeling particularly lazy you can run dmesg | grep “Manufacturer ID: ”. Note that if your router has been running for a while the boot message may not show up in dmesg. Reboot your router.).

A router with an ESMT chip will have a message like:

[    3.360000] Support this Device in MTK table! c8da
[    3.370000] NAND device: Manufacturer ID: **0xc8**, Chip ID: 0xda (Unknown NAND 256MiB 3,3V 8-bit), 256MiB, page size: 2048, OOB size: 64
[    3.380000] [NAND]select ecc bit:4, sparesize :64 spare_per_sector=16''
while one with a Micron chip will have something like:

[ 6.553177] Device found in MTK table, ID: 2cda, EXT_ID: 909506
[ 6.559076] Support this Device in MTK table! 2cda
[ 6.564084] [NAND]select ecc bit:4, sparesize :64 spare_per_sector=16
[ 6.570551] nand: device found, Manufacturer ID: **0x2c**, Chip ID: 0xda
[ 6.576887] nand: Micron NAND 256MiB 3,3V 8-bit
[ 6.581398] nand: 256 MiB, SLC, erase size: 128 KiB, page size: 2048, OOB size: 64''
The exact message may vary, but the main thing is that an ESMT chip will have a “Manufacturer ID” of 0xc8, while a Micron chip will have 0x2c.

Unlocking & Installing OpenWrt
You must unlock it to install non OEM software, such as OpenWrt. There are two schemes for unlocking. One method requires some equipments and a soldering iron. The other requires installing an App on your phone, creating an account with Xiaomi which puts your privacy at risk.

// Official Xiaomi developer firmware http://cdn.cnbj1.fds.api.mi-img.com/xiaoqiang/rom/r3p/miwifi_r3p_firmware_daddf_2.13.65.bin//

// OpenWrt “releases” with closed-source kernel module for mt7615e wifi https://github.com/iscilyas/openwrt-r3p/releases///

// Stock OpenWrt now (as of August 10, 2019) has built-in wifi support for mt7615e (via mt76); the “closed-source” version is no longer necessary https://downloads.openwrt.org/snapshots/targets/ramips/mt7621/openwrt-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-factory.bin//

Connect to the router via LAN cable (browser will pop up and router setup wizard will launch)
Wizard will ask you for 3 things: admin password, wi-fi password and PPoE creds (provided by your ISP). You can use Google Lens in translate mode on your phone. You should now have working internet connection.
Download Xiaomi “developer firmware” from http://cdn.cnbj1.fds.api.mi-img.com/xiaoqiang/rom/r3p/miwifi_r3p_firmware_daddf_2.13.65.bin
Goto http://192.168.31.1
Logon and find the page where you can upgrade the firmware (look for a big yellow dot with an “i” inside). You will see the version number of the router and there is a button below where you can browse for a file. Upload miwifi_r3p_firmware_daddf_2.13.65.bin (developer firmware) and wait a few minutes.
Install Android “Mi Wi-Fi” app from https://play.google.com/store/apps/details?id=com.xiaomi.router to your phone/tablet (there is also an iOS app)
Open “Mi Wi-Fi” app (and sign-up) then sign-in to your account. Router will be detected and added to your account (assuming you are connected to the WiFi on the router and the routers WAN port is connected to Internet).
On a PC, visit https://d.miwifi.com/rom/ssh and sign-in to you account. You will get to a page that should display your router, the root password and a download button. Hit the button to get miwifi_ssh.bin and save the password. (If the redirect download page doesn't work, try with “http:” instead of “https:” and vice versa)
Grab the “factory” installation image for OpenWrt (“openwrt-ramips-mt7621-xiaomi_mir3p_squashfs-factory.bin”) from openwrt snapshots (see above)
Make sure your USB drive is formatted with FAT32 and copy miwifi_ssh.bin, and openwrt-ramips-mt7621-xiaomi_mir3p-squashfs-factory.bin to root (not a subfolder) of the USB drive
Cut the power to the router, put the USB drive in the router, press and hold “reset” button (with a paper-clip), power on the router (while holding reset). When the router starts flashing yellow release the reset button. Wait until router has rebooted and you should (finally...) have SSH access.
Login to the router using SSH (192.168.31.1). Username is “root” and the password is what you got when downloading miwifi_ssh.bin
In SSH console (note: you should be able to copy-paste all these lines. For safety's sake, copy-paste each line individually.)
cd /extdisks/sda1 #(can be different if you remove and reinsert the usb stick)
mv openwrt-ramips-mt7621-xiaomi_mir3p-squashfs-factory.bin factory.bin # give it a shorter name
nvram set flag_try_sys1_failed=1 
nvram set flag_try_sys2_failed=0 
nvram set flag_boot_success=0 
nvram commit
dd if=factory.bin bs=1M count=4 | mtd write - kernel1
mtd erase rootfs0
mtd erase rootfs1
mtd erase overlay
dd if=factory.bin bs=1M skip=4 | mtd write - rootfs0
reboot
OpenWrt should be installed and accessible at 192.168.1.1

If you can't access the web interface, SSH to the router and type:

opkg update
opkg install luci
Once luci is installed, you should be able to access the router's web interface.

Reverting from OpenWrt to Stock Xiaomi firmware
If you decide you'd rather go back to using stock Xiaomi firmware (instead of OpenWrt), the process is easy:

Download stock (or preferably developer) firmware from Xiaomi (see link above) and rename it to miwifi.bin
Log in to your router using SSH (192.168.1.1) and run the following:
fw_setenv flag_try_sys1_failed 0
fw_setenv flag_try_sys2_failed 1
fw_setenv flag_boot_success 0
And then:

Turn off your router
Now, (format your flash drive to FAT/FAT32 if it isn't FAT32 already) and copy your miwifi.bin file to the root of the flash drive (not a subfolder).
Connect your flash drive to the router
Press and hold the reset button and power it up. Hold the reset button until the yellow light turns on. Wait for 5 minutes for the stock firmware to get installed. You can now login to your router at 192.168.31.1
Debricking
Using Serial/TFTP method :

First of all you'll need a UART (TTL/Serial adapter), and you'll also need to solder pin headers into the UART on the router's PCB (see earlier images).

To boot to OpenWrt:

Download the *initramfs-kernel.bin file (hereafter “initramfs.bin”) for your router (from the “releases” page). And configure a tftpd server to serve up the “initramfs.bin” file. Make sure your router and your tftpd server computer are connected to the same subnet (direct ethernet works well).

After power-on, your router will pause for 5 seconds at the uboot menu. Pressing '1' will get you into the tftpboot menu.

From there, enter in the router's IP address (arbitrary, in the subnet where your tftpd server is running), and the IP address of the machine running tftpd and the name of the file to boot (“initramfs.bin”).

Alternatively, by default the router assumes that it's address is 192.168.1.1, the TFTP server's is 192.168.1.3 and that the file is named test.bin. If you configure everything in this fashion, you just have to press “Enter” 3 times and the router will boot to the ramdisk.

You can fix things from there (ie, sysupgrade or mtd write to flash everything from scratch).

To recover to Xiaomi stock:

One of stock Xiaomi's “kernel” partitions is saved in the MTD partition “kernel_stock”. Download stock (or 'developer') Xiaomi firmware and put it on a USB stick and plug it in to the router.

After power-on, your router will pause for 5 seconds at the uboot menu. Pressing '4' will get you into the “interactive” menu. From there type:

''setenv flag_try_sys1_failed 0 setenv flag_try_sys2_failed 1 setenv uart_en 1 saveenv reset ''

Your system will boot into Xiaomi stock “recovery” mode. At some point you will see a message saying it's waiting for you to press the 'reset' button. Press the reset button. It will proceed to reinstall Xiaomi firmware from the file you copied to the USB.

If Booting Xiaomi Stock Doesn't work

If you accidentally overwrote the kernel0 (aka kernel_stock) partition, you won't have a working “stock” kernel to revert to (Hint: If after doing the above steps the boot message says the kernel version is 4.14. something instead of 3.10.14, or doesn't boot at all, keep reading.). In this case the above steps won't work. You will need to restore kernel0 using one of the following methods:

A copy of the kernel0 partition can be found at https://github.com/iscilyas/openwrt-r3p/releases/download/v0.1-rc1%2Bmt7615e-3/kernel0 Download it.
Put a copy of xiaomi stock (or developer) firmware in your FAT32-formatted USB drive (see above) and plug it in to your router
_Option 1: Using TFTP (you'll need to setup a tftp server on your pc):_

On your PC: Rename kernel0 to kernel0.bin and configure your tftp server to share it
On your router (using UART) on the menu that shows up for a few seconds right after you turn it on, Select option 1: System Load Linux to SDRAM via TFTP. Enter the values for your network.
At this point you should be booting “stock recovery” so wait until you see a message saying it's waiting for you to press the reset button and press it. It should install stock xiaomi (and reboot).
_Option 2: Using SSH (assuming you have a working OpenWRT installation):_

Copy kernel0 file into the router's /tmp directory (using winscp). Then run: '' cd /tmp mtd write kernel0 kernel_stock fw_setenv flag_try_sys1_failed 0 fw_setenv flag_try_sys2_failed 1 reboot''

Installing OpenWrt on a unit that already has the Breed bootloader
Some second-hand units come with the third-party Breed bootloader already flashed in place of the stock U-Boot (mtd Bootloader). The procedure below was tested with Breed r1280 (2020-01-14) and OpenWrt 25.12.6. It needs no soldering.

Things to know about Breed on this device:

Breed identifies the board as R3G, not R3P. Its web UI flash targets (固件1 / 固件2) use the R3G layout, which does not match the OpenWrt mir3p layout. Do not flash OpenWrt from the Breed web UI. Doing so leaves an unbootable UBI and a boot loop.
Breed only accepts images that begin with a uImage header (magic 27051956). The sysupgrade.bin (a tar archive) and the stock Xiaomi firmware (HDR1 header) are both rejected with 无法判断固件类型 (“cannot determine firmware type”).
Breed's flash write writes raw data. Writing the UBI part of an image to NAND this way does not produce a usable UBI volume: the kernel boots, but it cannot mount rootfs. Let OpenWrt's sysupgrade do the UBI formatting.
To enter Breed, hold the reset button while you power on the router. Release it after about 8–10 seconds, once the LED blinks. Breed is then reachable at 192.168.1.1 (web on port 80, telnet on port 23). While Breed is running, the router answers with MAC 00:0c:43:00:00:01 (a MediaTek default), not with its Xiaomi MAC. This is an easy way to tell whether Breed or OpenWrt is answering.
Procedure:

Download the initramfs-kernel.bin and squashfs-sysupgrade.bin images for xiaomi_mi-router-3-pro. Serve them from an HTTP server on your PC. Give the PC a static IP such as 192.168.1.10/24, because a DHCP lease is lost every time the router reboots.
Enter Breed and connect with telnet 192.168.1.1. Then run:
wget http://192.168.1.10:8080/initramfs-kernel.bin
boot mem 0x80001000
Wait until wget finishes before you run boot. wget loads the image into RAM at 0x80001000.

OpenWrt now boots from RAM. Move your cable to a LAN port. With DSA in current releases, the ports are wan, lan1, lan2 and lan3, and 192.168.1.1 answers only on the LAN ports. Breed's switch answers on every port, so a cable left in the WAN port makes the initramfs look dead. This is easy to mistake for a kernel panic.
Log in with ssh root@192.168.1.1 (no password). Copy the sysupgrade image to /tmp and run:
sysupgrade --test /tmp/openwrt-*-squashfs-sysupgrade.bin
sysupgrade -n /tmp/openwrt-*-squashfs-sysupgrade.bin
sysupgrade writes the kernel to the kernel partition (0x600000) and formats the ubi partition (0xa00000) properly.

The router reboots into OpenWrt from NAND. To confirm the install is persistent, check that mount shows ubifs on /overlay.
Caveat (not fully verified): Breed's R3G profile autoboots from offset 0x200000 (kernel_stock), while OpenWrt sysupgrade writes the kernel to 0x600000. On the tested unit, 0x200000 also held an OpenWrt kernel of the same version from an earlier attempt. Because of that, it was not confirmed which of the two kernels Breed actually booted. After any future sysupgrade, check that the running kernel version (uname -r) matches the new release.