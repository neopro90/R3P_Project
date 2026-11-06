===================================================================
                MT7621   stage1 code 10:33:11 (ASIC)
                CPU=50000000 HZ BUS=16666666 HZ
==================================================================
Change MPLL source from XTAL to CR...
do MEMPLL setting..
MEMPLL Config : 0x11100000
3PLL mode + External loopback
=== XTAL-40Mhz === DDR-1200Mhz ===
PLL2 FB_DL: 0x7, 1/0 = 619/405 1D000000
PLL3 FB_DL: 0x8, 1/0 = 580/444 21000000
PLL4 FB_DL: 0xe, 1/0 = 549/475 39000000
do DDR setting..[00320381]
Apply DDR3 Setting...(use customer AC)
          0    8   16   24   32   40   48   56   64   72   80   88   96  104  112  120
      --------------------------------------------------------------------------------
0000:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0001:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0002:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0003:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0004:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0005:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0006:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0007:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0008:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0009:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
000A:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
000B:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
000C:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
000D:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
000E:|    0    0    0    0    0    0    0    0    0    0    1    1    1    1    1    1
000F:|    0    0    0    0    1    1    1    1    1    1    1    1    1    1    1    0
0010:|    1    1    1    1    1    1    1    1    1    0    0    0    0    0    0    0
0011:|    1    1    1    1    0    0    0    0    0    0    0    0    0    0    0    0
0012:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0013:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0014:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0015:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0016:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0017:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0018:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
0019:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
001A:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
001B:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
001C:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
001D:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
001E:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
001F:|    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0    0
rank 0 coarse = 15
rank 0 fine = 72
B:|    0    0    0    0    0    0    0    0    1    1    1    0    0    0    0    0
opt_dle value:9
DRAMC_R0DELDLY[018]=00002121
==================================================================
                RX      DQS perbit delay software calibration
==================================================================
1.0-15 bit dq delay value
==================================================================
bit|     0  1  2  3  4  5  6  7  8  9
--------------------------------------
0 |    11 7 8 9 6 5 7 5 7 9
10 |    9 9 9 12 7 11
--------------------------------------

==================================================================
2.dqs window
x=pass dqs delay value (min~max)center
y=0-7bit DQ of every group
input delay:DQS0 =33 DQS1 = 33
==================================================================
bit     DQS0     bit      DQS1
0  (1~65)33  8  (1~62)31
1  (1~62)31  9  (2~61)31
2  (1~60)30  10  (1~60)30
3  (1~66)33  11  (1~59)30
4  (1~64)32  12  (1~65)33
5  (1~62)31  13  (1~61)31
6  (1~61)31  14  (1~62)31
7  (1~65)33  15  (1~65)33
==================================================================
3.dq delay value last
==================================================================
bit|    0  1  2  3  4  5  6  7  8   9
--------------------------------------
0 |    11 9 11 9 7 7 9 5 9 11
10 |    12 12 9 14 9 11
==================================================================
==================================================================
     TX  perbyte calibration
==================================================================
DQS loop = 15, cmp_err_1 = ffff0000
dqs_perbyte_dly.last_dqsdly_pass[0]=15,  finish count=1
dqs_perbyte_dly.last_dqsdly_pass[1]=15,  finish count=2
DQ loop=15, cmp_err_1 = ffff0000
dqs_perbyte_dly.last_dqdly_pass[0]=15,  finish count=1
dqs_perbyte_dly.last_dqdly_pass[1]=15,  finish count=2
byte:0, (DQS,DQ)=(8,8)
byte:1, (DQS,DQ)=(8,8)
20,data:88
[EMI] DRAMC calibration passed

===================================================================
                MT7621   stage1 code done
                CPU=50000000 HZ BUS=16666666 HZ
===================================================================


U-Boot 1.1.3 (Nov 11 2016 - 11:39:34)

Board: Ralink APSoC DRAM:  256 MB
Power on memory test. Memory size= 256 MB...OK!
relocate_code Pointer at: 8ffac000

Config XHCI 40M PLL
Allocate 16 byte aligned buffer: 8ffe0050
Enable NFI Clock
# MTK NAND # : Use HW ECC
NAND ID [C8 DA 90 95 44]
Device found in MTK table, ID: c8da, EXT_ID: 909544
Support this Device in MTK table! c8da
select_chip
[NAND]select ecc bit:4, sparesize :64 spare_per_sector=16
Signature matched and data read!
load_fact_bbt success 2047
load fact bbt success
[mtk_nand] probe successfully!
mtd->writesize=2048 mtd->oobsize=64,    mtd->erasesize=131072  devinfo.iowidth=8
..============================================
Ralink UBoot Version: 5.0.0.0
--------------------------------------------
ASIC MT7621A DualCore (MAC to MT7530 Mode)
DRAM_CONF_FROM: Auto-Detection
DRAM_TYPE: DDR3
DRAM bus: 16 bit
Xtal Mode=3 OCP Ratio=1/3
Flash component: NAND Flash
Date:Nov 11 2016  Time:11:39:34
============================================
icache: sets:256, ways:4, linesz:32 ,total:32768
dcache: sets:256, ways:4, linesz:32 ,total:32768

 ##### The CPU freq = 880 MHZ ####
 estimate memory size =256 Mbytes
#Reset_MT7530
set LAN/WAN LLLLW

Please choose the operation:
   1: Load system code to SDRAM via TFTP.
   2: Load system code then write to Flash via TFTP.
   3: Boot system code via Flash (default).
   4: Entr boot command line interface.
   7: Load Boot Loader code then write to Flash via Serial.
   9: Load Boot Loader code then write to Flash via TFTP.                     0
Booting System 1
..Erasing NAND Flash...
ranand_erase: start:40000, len:20000
.Writing to NAND Flash...
done

3: System Boot system code via Flash.
## Booting image at bc200000 ...
   Image Name:   MIPS OpenWrt Linux-3.10.14
   Image Type:   MIPS Linux Kernel Image (lzma compressed)
   Data Size:    3387996 Bytes =  3.2 MB
   Load Address: 82001000
   Entry Point:  82459b20
....................................................   Verifying Checksum ... OK
   Uncompressing Kernel Image ... OK
commandline uart_en=0 factory_mode=0 usb_u3=0
No initrd
## Transferring control to Linux (at address 82459b20) ...
## Giving linux memsize in MB, 256

Starting kernel ...


LINUX started...

 THIS IS ASIC

SDK 5.0.S.0
[    0.000000] Initializing cgroup subsys cpuset
[    0.000000] Linux version 3.10.14 (jenkins@szy-rommaker) (gcc version 4.8.5 (crosstool-NG crosstool-ng-1.22.0) ) #1 SMP Fri Nov 11 11:49:23 CST 2016
[    0.000000]
[    0.000000]  The CPU feqenuce set to 880 MHz
[    0.000000] GCMP present
[    0.000000] CPU0 revision is: 0001992f (MIPS 1004Kc)
[    0.000000] Software DMA cache coherency
[    0.000000] Determined physical RAM map:
[    0.000000]  memory: 1c000000 @ 00000000 (usable)
[    0.000000]  memory: 04000000 @ 20000000 (usable)
[    0.000000] Initrd not found or empty - disabling initrd
[    0.000000] Zone ranges:
[    0.000000]   DMA      [mem 0x00000000-0x01ffffff]
[    0.000000]   Normal   [mem 0x02000000-0x1fffffff]
[    0.000000]   HighMem  [mem 0x20000000-0x23ffffff]
[    0.000000] Movable zone start for each node
[    0.000000] Early memory node ranges
[    0.000000]   node   0: [mem 0x00000000-0x1bffffff]
[    0.000000]   node   0: [mem 0x20000000-0x23ffffff]
[    0.000000] Detected 3 available secondary CPU(s)
[    0.000000] Primary instruction cache 32kB, 4-way, VIPT, linesize 32 bytes.
[    0.000000] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.000000] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.000000] PERCPU: Embedded 7 pages/cpu @82bfc000 s6848 r8192 d13632 u32768
[    0.000000] Built 1 zonelists in Zone order, mobility grouping on.  Total pages: 130048
[    0.000000] Kernel command line: console=ttyS1,115200n8 root=/dev/mtdblock5 uart_en=0 factory_mode=0 usb_u3=0
[    0.000000] PID hash table entries: 2048 (order: 1, 8192 bytes)
[    0.000000] Dentry cache hash table entries: 65536 (order: 6, 262144 bytes)
[    0.000000] Inode-cache hash table entries: 32768 (order: 5, 131072 bytes)
[    0.000000] Writing ErrCtl register=00008d2f
[    0.000000] Readback ErrCtl register=00008d2f
[    0.000000] allocated 1179648 bytes of page_cgroup
[    0.000000] please try 'cgroup_disable=memory' option if you don't want memory cgroups
[    0.000000] Memory: 510336k/458752k available (4500k kernel code, 13952k reserved, 1184k data, 1540k init, 65536k highmem)
[    0.000000] SLUB: HWalign=64, Order=0-3, MinObjects=0, CPUs=4, Nodes=1
[    0.000000] Hierarchical RCU implementation.
[    0.000000] NR_IRQS:128
[    0.000000] console [ttyS1] enabled
[    0.140000] Calibrating delay loop... 577.53 BogoMIPS (lpj=2887680)
[    0.200000] pid_max: default: 32768 minimum: 301
[    0.200000] Mount-cache hash table entries: 512
[    0.210000] Initializing cgroup subsys memory
[    0.210000] launch: starting cpu1
[    0.220000] launch: cpu1 gone!
[    0.220000] CPU1 revision is: 0001992f (MIPS 1004Kc)
[    0.220000] Primary instruction cache 32kB, 4-way, VIPT, linesize 32 bytes.
[    0.220000] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.220000] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.280000] Synchronize counters for CPU 1: done.
[    0.290000] launch: starting cpu2
[    0.290000] launch: cpu2 gone!
[    0.290000] CPU2 revision is: 0001992f (MIPS 1004Kc)
[    0.290000] Primary instruction cache 32kB, 4-way, VIPT, linesize 32 bytes.
[    0.290000] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.290000] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.350000] Synchronize counters for CPU 2: done.
[    0.360000] launch: starting cpu3
[    0.360000] launch: cpu3 gone!
[    0.360000] CPU3 revision is: 0001992f (MIPS 1004Kc)
[    0.360000] Primary instruction cache 32kB, 4-way, VIPT, linesize 32 bytes.
[    0.360000] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.360000] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.420000] Synchronize counters for CPU 3: done.
[    0.430000] Brought up 4 CPUs
[    0.430000] devtmpfs: initialized
[    0.430000] NET: Registered protocol family 16
[    0.730000] release PCIe RST: RALINK_RSTCTRL = 7000000
[    0.740000] PCIE PHY initialize
[    0.740000] ***** Xtal 40MHz *****
[    0.740000] start MT7621 PCIe register access
[    1.340000] RALINK_RSTCTRL = 7000000
[    1.340000] RALINK_CLKCFG1 = 77ffeff8
[    1.340000]
[    1.340000] *************** MT7621 PCIe RC mode *************
[    1.840000] PCIE2 no card, disable it(RST&CLK)
[    1.840000] pcie_link status = 0x3
[    1.850000] RALINK_RSTCTRL= 3000000
[    1.850000] *** Configure Device number setting of Virtual PCI-PCI bridge ***
[    1.860000] RALINK_PCI_PCICFG_ADDR = 21007f2 -> 21007f2
[    1.860000] PCIE0 enabled
[    1.860000] PCIE1 enabled
[    1.870000] interrupt enable status: 300000
[    1.870000] Port 1 N_FTS = 1b105000
[    1.870000] Port 0 N_FTS = 1b105000
[    1.880000] config reg done
[    1.880000] init_rt2880pci done
[    1.910000] bio: create slab <bio-0> at 0
[    1.910000] SCSI subsystem initialized
[    1.920000] usbcore: registered new interface driver usbfs
[    1.920000] usbcore: registered new interface driver hub
[    1.930000] usbcore: registered new device driver usb
[    1.930000] PCI host bridge to bus 0000:00
[    1.930000] pci_bus 0000:00: root bus resource [mem 0x60000000-0x6fffffff]
[    1.940000] pci_bus 0000:00: root bus resource [io  0x1e160000-0x1e16ffff]
[    1.940000] pci_bus 0000:00: No busn resource found for root bus, will use [bus 00-ff]
[    1.950000] pci 0000:00:00.0: bridge configuration invalid ([bus 00-00]), reconfiguring
[    1.950000] pci 0000:00:01.0: bridge configuration invalid ([bus 00-00]), reconfiguring
[    1.960000] pci 0000:00:00.0: BAR 0: can't assign mem (size 0x80000000)
[    1.960000] pci 0000:00:01.0: BAR 0: can't assign mem (size 0x80000000)
[    1.970000] pci 0000:00:00.0: BAR 8: assigned [mem 0x60000000-0x600fffff]
[    1.970000] pci 0000:00:01.0: BAR 8: assigned [mem 0x60100000-0x601fffff]
[    1.980000] pci 0000:00:00.0: BAR 1: assigned [mem 0x60200000-0x6020ffff]
[    1.980000] pci 0000:00:01.0: BAR 1: assigned [mem 0x60210000-0x6021ffff]
[    1.990000] pci 0000:01:00.0: BAR 0: assigned [mem 0x60000000-0x600fffff 64bit]
[    1.990000] pci 0000:00:00.0: PCI bridge to [bus 01]
[    2.000000] pci 0000:00:00.0:   bridge window [mem 0x60000000-0x600fffff]
[    2.000000] pci 0000:02:00.0: BAR 0: assigned [mem 0x60100000-0x601fffff 64bit]
[    2.010000] pci 0000:00:01.0: PCI bridge to [bus 02]
[    2.010000] pci 0000:00:01.0:   bridge window [mem 0x60100000-0x601fffff]
[    2.020000] PCI: Enabling device 0000:00:00.0 (0004 -> 0006)
[    2.020000] PCI: Enabling device 0000:00:01.0 (0004 -> 0006)
[    2.030000] BAR0 at slot 0 = 0
[    2.030000] bus=0x0, slot = 0x0
[    2.030000] res[0]->start = 0
[    2.030000] res[0]->end = 0
[    2.040000] res[1]->start = 60200000
[    2.040000] res[1]->end = 6020ffff
[    2.040000] res[2]->start = 0
[    2.040000] res[2]->end = 0
[    2.050000] res[3]->start = 0
[    2.050000] res[3]->end = 0
[    2.050000] res[4]->start = 0
[    2.050000] res[4]->end = 0
[    2.060000] res[5]->start = 0
[    2.060000] res[5]->end = 0
[    2.060000] BAR0 at slot 1 = 0
[    2.060000] bus=0x0, slot = 0x1
[    2.070000] res[0]->start = 0
[    2.070000] res[0]->end = 0
[    2.070000] res[1]->start = 60210000
[    2.070000] res[1]->end = 6021ffff
[    2.080000] res[2]->start = 0
[    2.080000] res[2]->end = 0
[    2.080000] res[3]->start = 0
[    2.080000] res[3]->end = 0
[    2.090000] res[4]->start = 0
[    2.090000] res[4]->end = 0
[    2.090000] res[5]->start = 0
[    2.090000] res[5]->end = 0
[    2.100000] bus=0x1, slot = 0x0, irq=0x4
[    2.100000] res[0]->start = 60000000
[    2.100000] res[0]->end = 600fffff
[    2.110000] res[1]->start = 0
[    2.110000] res[1]->end = 0
[    2.110000] res[2]->start = 0
[    2.110000] res[2]->end = 0
[    2.120000] res[3]->start = 0
[    2.120000] res[3]->end = 0
[    2.120000] res[4]->start = 0
[    2.120000] res[4]->end = 0
[    2.130000] res[5]->start = 0
[    2.130000] res[5]->end = 0
[    2.130000] bus=0x2, slot = 0x1, irq=0x18
[    2.130000] res[0]->start = 60100000
[    2.140000] res[0]->end = 601fffff
[    2.140000] res[1]->start = 0
[    2.140000] res[1]->end = 0
[    2.140000] res[2]->start = 0
[    2.150000] res[2]->end = 0
[    2.150000] res[3]->start = 0
[    2.150000] res[3]->end = 0
[    2.150000] res[4]->start = 0
[    2.160000] res[4]->end = 0
[    2.160000] res[5]->start = 0
[    2.160000] res[5]->end = 0
[    2.170000] Switching to clocksource MIPS
[    2.170000] cfg80211: Calling CRDA to update world regulatory domain
[    2.170000] NET: Registered protocol family 2
[    2.170000] TCP established hash table entries: 4096 (order: 3, 32768 bytes)
[    2.170000] TCP bind hash table entries: 4096 (order: 3, 32768 bytes)
[    2.170000] TCP: Hash tables configured (established 4096 bind 4096)
[    2.170000] TCP: reno registered
[    2.170000] UDP hash table entries: 256 (order: 1, 8192 bytes)
[    2.170000] UDP-Lite hash table entries: 256 (order: 1, 8192 bytes)
[    2.170000] NET: Registered protocol family 1
[    3.200000] 4 CPUs re-calibrate udelay(lpj = 2924544)
[    3.210000] bounce pool size: 64 pages
[    3.230000] squashfs: version 4.0 (2009/01/31) Phillip Lougher
[    3.240000] msgmni has been set to 868
[    3.240000] Block layer SCSI generic (bsg) driver version 0.4 loaded (major 253)
[    3.250000] io scheduler noop registered (default)
[    3.250000] MIWIFI panic notifier registeredreg_int_mask=0, INT_MASK= 0
[    3.260000] HSDMA_init
[    3.260000]
[    3.260000]  hsdma_phy_tx_ring0 = 0x01c00000, hsdma_tx_ring0 = 0xa1c00000
[    3.270000]
[    3.270000]  hsdma_phy_rx_ring0 = 0x01c04000, hsdma_rx_ring0 = 0xa1c04000
[    3.280000] TX_CTX_IDX0 = 0
[    3.280000] TX_DTX_IDX0 = 0
[    3.280000] RX_CRX_IDX0 = 3ff
[    3.290000] RX_DRX_IDX0 = 0
[    3.290000] set_fe_HSDMA_glo_cfg
[    3.290000] HSDMA_GLO_CFG = 465
[    3.300000] Serial: 8250/16550 driver, 2 ports, IRQ sharing disabled
[    3.300000] serial8250: ttyS0 at MMIO 0x1e000d00 (irq = 27) is a 16550A
[    3.310000] serial8250: ttyS1 at MMIO 0x1e000c00 (irq = 26) is a 16550A
[    3.320000] led=10, on=4000, off=1, blinks,=1, reset=1, time=4000
[    3.320000] Ralink gpio driver initialized
[    3.330000] brd: module loaded
[    3.330000] MediaTek Nand driver init, version v2.1 Fix AHB virt2phys error
[    3.340000] Allocate 16 byte aligned buffer: 827563d0
[    3.350000] Enable NFI Clock
[    3.350000] # MTK NAND # : Use HW ECC
[    3.350000] NAND ID [C8 DA 90 95 44, 00909544]
[    3.360000] Device found in MTK table, ID: c8da, EXT_ID: 909544
[    3.360000] Support this Device in MTK table! c8da
[    3.370000] NAND device: Manufacturer ID: 0xc8, Chip ID: 0xda (Unknown NAND 256MiB 3,3V 8-bit), 256MiB, page size: 2048, OOB size: 64
[    3.380000] [NAND]select ecc bit:4, sparesize :64 spare_per_sector=16
[    3.390000] Scanning device for bad blocks
[    3.680000] Signature matched and data read!
[    3.680000] load_fact_bbt success 2047
[    3.680000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.690000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.700000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.710000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.720000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.730000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.740000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.750000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.760000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.770000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.780000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.790000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.800000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.810000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.820000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.830000] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
[    3.840000] Creating 13 MTD partitions on "MT7621-NAND":
[    3.850000] 0x000000000000-0x00000ff80000 : "ALL"
[    3.850000] 0x000000000000-0x000000040000 : "Bootloader"
[    3.860000] 0x000000040000-0x000000080000 : "Config"
[    3.870000] 0x000000080000-0x0000000c0000 : "Bdata"
[    3.870000] 0x0000000c0000-0x000000100000 : "Factory"
[    3.880000] 0x000000100000-0x000000140000 : "crash"
[    3.890000] 0x000000140000-0x0000001c0000 : "crash_syslog"
[    3.890000] 0x0000001c0000-0x000000200000 : "reserved0"
[    3.900000] 0x000000200000-0x000000600000 : "kernel0"
[    3.910000] 0x000000600000-0x000000a00000 : "kernel1"
[    3.910000] 0x000000a00000-0x000003200000 : "rootfs0"
[    3.920000] 0x000003200000-0x000005a00000 : "rootfs1"
[    3.920000] 0x000005a00000-0x00000ff80000 : "overlay"
[    3.930000] [mtk_nand] probe successfully!
[    3.940000] PPP generic driver version 2.4.2
[    3.940000] PPP BSD Compression module registered
[    3.950000] PPP Deflate Compression module registered
[    3.950000] PPP MPPE Compression module registered
[    3.960000] NET: Registered protocol family 24
[    3.960000] PPTP driver version 0.8.5
[    3.960000] ps: can't get major 253
[    3.970000] GMAC1_MAC_ADRH -- : 0x0000286c
[    3.970000] GMAC1_MAC_ADRL -- : 0x07626df0
[    3.980000] Ralink APSoC Ethernet Driver Initilization. v3.1  1024 rx/tx descriptors allocated, mtu = 1500!
[    3.990000] GMAC1_MAC_ADRH -- : 0x0000286c
[    3.990000] GMAC1_MAC_ADRL -- : 0x07626df0
[    3.990000] PROC INIT OK!
[    4.000000] ehci_hcd: USB 2.0 'Enhanced' Host Controller (EHCI) Driver
[    4.000000] ehci-pci: EHCI PCI platform driver
[    4.010000] ehci-platform: EHCI generic platform driver
[    4.010000] *****run project phy.
[    4.040000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.060000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.080000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.100000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.120000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.140000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.160000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.180000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.200000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.220000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.280000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.300000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.320000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.340000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.360000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.380000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.400000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.420000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.440000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.460000] FM_OUT value: u4FmOut = 0(0x00000000)
[    4.610000] xhci-hcd xhci-hcd: xHCI Host Controller
[    4.610000] xhci-hcd xhci-hcd: new USB bus registered, assigned bus number 1
[    4.620000] xhci-hcd xhci-hcd: irq 22, io mem 0x1e1c0000
[    4.630000] hub 1-0:1.0: USB hub found
[    4.630000] hub 1-0:1.0: 2 ports detected
[    4.630000] xhci-hcd xhci-hcd: xHCI Host Controller
[    4.640000] xhci-hcd xhci-hcd: new USB bus registered, assigned bus number 2
[    4.650000] hub 2-0:1.0: USB hub found
[    4.650000] hub 2-0:1.0: 1 port detected
[    4.650000] usbcore: registered new interface driver usb-storage
[    4.660000] softdog: Software Watchdog Timer: 0.08 initialized. soft_noboot=0 soft_margin=60 sec soft_panic=0 (nowayout=0)
[    4.670000] Netfilter messages via NETLINK v0.30.
[    4.680000] nfnl_acct: registering with nfnetlink.
[    4.680000] nf_conntrack version 0.5.0 (7974 buckets, 31896 max)
[    4.690000] ipip: IPv4 over IPv4 tunneling driver
[    4.690000] gre: GRE over IPv4 demultiplexor driver
[    4.700000] ip_tables: (C) 2000-2006 Netfilter Core Team
[    4.710000] Type=Restricted Cone
[    4.710000] TCP: cubic registered
[    4.710000] NET: Registered protocol family 10
[    4.720000] NET: Registered protocol family 17
[    4.720000] l2tp_core: L2TP core driver, V2.0
[    4.730000] l2tp_ppp: PPPoL2TP kernel driver, V2.0
[    4.730000] l2tp_netlink: L2TP netlink interface
[    4.740000] 8021q: 802.1Q VLAN Support v1.8
[    4.740000] QEFOTP register done
[    4.750000] Freeing unused kernel memory: 1540K (8258f000 - 82710000)
[    4.760000] csd: CSD deadlock debugging initiated!
[    4.800000] Loading essential drivers...
[    4.810000] Press Ctrl+C to enter RAMFS...
[    5.820000] Check for USB recovery...
[    5.850000] Bringup the system...
[    5.870000] flag_boot_rootfs=0 mounting /dev/mtd10
ECC failed: 0
ECC corrected: 0
Number of bad blocks: 0
Number of bbt blocks: 0
Block size 131072, page size 2048, OOB size 64
Dumping data starting at 0x00000000 and ending at 0x00000800...
ECC failed: 0
ECC corrected: 0
Number of bad blocks: 0
Number of bbt blocks: 0
Block size 131072, page size 2048, OOB size 64
Dumping data starting at 0x00000000 and ending at 0x01422cd2...
config core 'version'
        # ROM ver
        option ROM '2.10.41'
        # channel
        option CHANNEL 'release'
        # hardware platform R1AC or R1N etc.
        option HARDWARE 'R3P'
        # CFE ver
        option UBOOT '1.0.2'
        # Linux Kernel ver
        option LINUX '2.10.41'
        # RAMFS ver
        option RAMFS '2.10.41'
        # SQUASHFS ver
        option SQAFS '2.10.41'
        # ROOTFS ver
        option ROOTFS '2.10.41'
        #build time
        option BUILDTIME 'Fri, 11 Nov 2016 11:38:29 +0800'
        #build timestamp
        option BUILDTS '1478835509'
        #build git tag
        option GTAG 'commit ab09d8eebcd0fd77d59fa593434390be270da8da'
mount: mounting proc on /proc failed: Device or resource busy
mount: mounting sysfs on /sys failed: Device or resource busy
[   13.320000] 28:6C: 7:62:6D:FFFFFFF0
[   13.320000] Raeth v3.1 (Tasklet)
[   13.330000] set CLK_CFG_0 = 0x40a00020!!!!!!!!!!!!!!!!!!1
[   13.340000] phy_free_head is 0x1c0c000!!!
[   13.340000] phy_free_tail_phy is 0x1c0dff0!!!
[   13.350000] txd_pool=a1c10000 phy_txd_pool=01C10000
[   13.350000] ei_local->skb_free start address is 0x9bf226dc.
[   13.360000] free_txd: 01c10010, ei_local->cpu_ptr: 01C10000
[   13.360000]  POOL  HEAD_PTR | DMA_PTR | CPU_PTR
[   13.370000] ----------------+---------+--------
[   13.370000]      0xa1c10000 0x01C10000 0x01C10000
[   13.380000]
[   13.380000] phy_qrx_ring = 0x01c0e000, qrx_ring = 0xa1c0e000
[   13.390000]
[   13.390000] phy_rx_ring0 = 0x01c18000, rx_ring0 = 0xa1c18000
[   13.420000] MT7530 Reset Completed!!
[   13.420000] change HW-TRAP to 0x17c8f
[   13.430000] set LAN/WAN LLLLW
[   13.440000] GMAC1_MAC_ADRH -- : 0x0000286c
[   13.440000] GMAC1_MAC_ADRL -- : 0x07626df0
[   13.450000] GDMA2_MAC_ADRH -- : 0x0000286c
[   13.450000] GDMA2_MAC_ADRL -- : 0x07626df1
[   13.460000] eth1: ===> VirtualIF_open
[   13.460000] MT7621 GE2 link rate to 1G
[   13.460000] CDMA_CSG_CFG = 81000000
[   13.470000] GDMA1_FWD_CFG = 20710000
[   13.470000] GDMA2_FWD_CFG = 20710000
- preinit -
Fri Nov 11 11:49:23 UTC 2016
- regular preinit -
[   13.750000] UBI: attaching mtd12 to ubi1
[   15.280000] UBI: scanning is finished
[   15.300000] UBI: attached mtd12 (name "overlay", size 165 MiB) to ubi1
[   15.300000] UBI: PEB size: 131072 bytes (128 KiB), LEB size: 126976 bytes
[   15.310000] UBI: min./max. I/O unit sizes: 2048/2048, sub-page size 2048
[   15.320000] UBI: VID header offset: 2048 (aligned 2048), data offset: 4096
[   15.320000] UBI: good PEBs: 1324, bad PEBs: 0, corrupted PEBs: 0
[   15.330000] UBI: user volume: 1, internal volumes: 1, max. volumes count: 128
[   15.340000] UBI: max/mean erase counter: 1/0, WL threshold: 4096, image sequence number: 1627926567
[   15.340000] UBI: available PEBs: 0, total reserved PEBs: 1324, PEBs reserved for bad PEB handling: 40
[   15.350000] UBI: background thread "ubi_bgt1d" started, PID 587
UBI device number 1, total 1324 LEBs (168116224 bytes, 160.3 MiB), available 0 LEBs (0 bytes), L[   15.370000] UBIFS: background thread "ubifs_bgt1_0" started, PID 589
EB size 126976 bytes (124.0 KiB)
[   15.440000] UBIFS: recovery needed
[   15.620000] UBIFS: recovery completed
[   15.620000] UBIFS: mounted UBI device 1, volume 0, name "data"
[   15.630000] UBIFS: LEB size: 126976 bytes (124 KiB), min./max. I/O unit sizes: 2048 bytes/2048 bytes
[   15.640000] UBIFS: FS size: 161005568 bytes (153 MiB, 1268 LEBs), journal size 8126464 bytes (7 MiB, 64 LEBs)
[   15.650000] UBIFS: reserved for root: 4952683 bytes (4836 KiB)
[   15.650000] UBIFS: media format: w4/r0 (latest is w4/r0), UUID 80DA26AE-1000-4641-ABCF-12D0ECA1E6EA, small LPT model
- init -
[   16.640000] ra2880stop()...Done
[   16.650000] eth1: ===> VirtualIF_close
[   16.650000] Free TX/RX Ring Memory!
init started: BusyBox v1.19.4 (2016-11-11 11:34:44 CST)

Please press Enter to activate this console. rcS S boot: INFO: rc script run time limit to 65 seconds.
[   18.180000] NTFS driver 2.1.30 [Flags: R/O MODULE].
[   18.360000] tntfs: module license 'Commercial. For support email ntfs-support@tuxera.com.' taints kernel.
[   18.370000] Disabling lock debugging due to kernel taint
[   18.390000] Tuxera NTFS driver 3015.1.29 [Flags: R/W MODULE].
[   18.500000] Tuxera exFAT driver version 3015.8.24 [Flags: R/W MODULE].
[   18.500000] Built against headers 3.10.14 #5 SMP Tue Aug 23 14:09:43 CST 2016 mips
[   18.510000] Running on kernel 3.10.14 #1 SMP Fri Nov 11 11:49:23 CST 2016 mips
[   18.600000] Tuxera FAT 12/16/32 driver version 3015.1.29 [Flags: R/W MODULE].
[   18.610000] Built against headers 3.10.14 #5 SMP Tue Aug 23 14:09:43 CST 2016 mips
[   18.620000] Running on kernel 3.10.14 #1 SMP Fri Nov 11 11:49:23 CST 2016 mips
[   18.620000] WARNING: Running kernel does not match configured kernel headers the driver was built with!  The driver needs to be rebuilt with the correct kernel headers!
[   18.690000] tun: Universal TUN/TAP device driver, 1.6
[   18.700000] tun: (C) 1999-2004 Max Krasnyansky <maxk@qualcomm.com>
[   18.750000] Mirror/redirect action on
[   18.770000] u32 classifier
[   18.780000]     input device check on
[   18.780000]     Actions configured
[   18.910000] ip_gre: GRE over IPv4 tunneling driver
[   18.950000] xt_time: kernel timezone is +0800
[   19.090000] ip_set: protocol 6
[   19.180000] ohci_hcd: USB 1.1 'Open' Host Controller (OHCI) Driver
[   19.290000] dev_redirect load success.
[   20.620000] ChipOpsMCUHook
[   20.940000] ChipOpsMCUHook
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: INFO: loading exist /etc/config/network.
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: #### Loopback configuration
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: config interface loopback
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option ifname      lo
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option proto       static
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option ipaddr      127.0.0.1
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option netmask     255.0.0.0
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: #### LAN configuration
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: config interface lan
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option ifname      eth0
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option type        bridge
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option proto       static
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option ipaddr      192.168.31.1
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option netmask     255.255.255.0
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: #### WAN configuration
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: config interface wan
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option ifname      eth1
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option proto       dhcp
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: #### IFB interface for MiQoS
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: config interface ifb
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: option ifname ifb0
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: #### READY configuration
Fri Nov 11 19:49:30 CST 2016 netconfig[826]: config interface ready
Fri Nov 11 19:49:31 CST 2016 netconfig[826]: option proto    static
Fri Nov 11 19:49:31 CST 2016 netconfig[826]: option ipaddr   169.254.29.1
Fri Nov 11 19:49:31 CST 2016 netconfig[826]: option netmask  255.255.255.0
[   23.090000] 28:6C: 7:62:6D:FFFFFFF0
[   23.100000] Raeth v3.1 (Tasklet)
[   23.100000] set CLK_CFG_0 = 0x40a00020!!!!!!!!!!!!!!!!!!1
[   23.110000] phy_free_head is 0x1c88000!!!
[   23.120000] phy_free_tail_phy is 0x1c89ff0!!!
[   23.120000] txd_pool=a1c90000 phy_txd_pool=01C90000
[   23.130000] ei_local->skb_free start address is 0x9bf226dc.
[   23.130000] free_txd: 01c90010, ei_local->cpu_ptr: 01C90000
[   23.140000]  POOL  HEAD_PTR | DMA_PTR | CPU_PTR
[   23.140000] ----------------+---------+--------
[   23.150000]      0xa1c90000 0x01C90000 0x01C90000
[   23.150000]
[   23.150000] phy_qrx_ring = 0x01c8a000, qrx_ring = 0xa1c8a000
[   23.160000]
[   23.160000] phy_rx_ring0 = 0x01c8c000, rx_ring0 = 0xa1c8c000
[   23.190000] MT7530 Reset Completed!!
[   23.200000] change HW-TRAP to 0x17c8f
[   23.210000] set LAN/WAN LLLLW
[   23.210000] GMAC1_MAC_ADRH -- : 0x0000286c
[   23.220000] GMAC1_MAC_ADRL -- : 0x07626df0
[   23.220000] eth1: ===> VirtualIF_open
[   23.220000] MT7621 GE2 link rate to 1G
[   23.230000] CDMA_CSG_CFG = 81000000
[   23.230000] GDMA1_FWD_CFG = 20710000
[   23.230000] GDMA2_FWD_CFG = 20710000
[   23.240000] device eth0 entered promiscuous mode
[   23.250000] br-lan: port 1(eth0) entered forwarding state
[   23.250000] br-lan: port 1(eth0) entered forwarding state
[   23.280000] eth1: ===> VirtualIF_open
[   25.250000] br-lan: port 1(eth0) entered forwarding state
[   32.810000] device wl1 entered promiscuous mode
[   32.820000] br-lan: port 2(wl1) entered forwarding state
[   32.820000] br-lan: port 2(wl1) entered forwarding state
[   34.820000] br-lan: port 2(wl1) entered forwarding state
[   40.240000] device wl0 entered promiscuous mode
[   40.250000] br-lan: port 3(wl0) entered forwarding state
[   40.250000] br-lan: port 3(wl0) entered forwarding state
[   40.360000] dev_redirect: add(+) dev redirect mapping: src:eth1->dst:ifb0
rcS S calling: /etc/rc.d/S20network boot: WARNING: EXITCODE=0, execute too slow, 19 >= 15: /etc/rc.d/S20network boot
[   42.250000] br-lan: port 3(wl0) entered forwarding state
[   44.580000] dev_redirect: add(+) dev redirect mapping: src:eth1->dst:ifb0
Fri Nov 11 19:49:55 CST 2016 boot_check[4395]: INFO: Wireless OK
[   47.160000] tcpproxy_init, succeed!
[   48.390000] Ralink HW NAT Module Enabled
[   48.400000] eth0 ifindex =3
[   48.410000] eth1 ifindex =5
[   54.470000] dev_redirect: add(+) dev redirect mapping: src:eth1->dst:ifb0
rcS S boot: INFO: rcS S boot timing 39 seconds.
Fri Nov 11 19:50:06 CST 2016 INFO: rcS S boot timing 39 seconds.
rcS S boot: system type(R3P/2): SQUASH/3
Fri Nov 11 19:50:06 CST 2016 system type(R3P/2): SQUASH/3
rcS S boot: ROOTFS: /dev/ram0 on / type squashfs (ro,relatime)
Fri Nov 11 19:50:06 CST 2016 ROOTFS: /dev/ram0 on / type squashfs (ro,relatime)
[   56.950000] led=6, on=1, off=4000, blinks,=1, reset=1, time=4000
[   56.960000] led=10, on=1, off=4000, blinks,=1, reset=1, time=4000
[   56.970000] led=8, on=4000, off=1, blinks,=1, reset=1, time=4000
mknod: /dev/gpio: File exists
Fri Nov 11 19:50:06 CST 2016 boot_check[6408]: Booting up finished.