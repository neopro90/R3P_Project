[    0.000000] Linux version 4.14.111 (ilyas@fish) (gcc version 7.4.0 (OpenWrt GCC 7.4.0 r9514+3-302f7d57a9)) #0 SMP Fri Apr 12 16:29:18 2019
[    0.000000] SoC Type: MediaTek MT7621 ver:1 eco:3
[    0.000000] bootconsole [early0] enabled
[    0.000000] CPU0 revision is: 0001992f (MIPS 1004Kc)
[    0.000000] MIPS: machine is Xiaomi Mi Router 3 Pro
[    0.000000] Determined physical RAM map:
[    0.000000]  memory: 1c000000 @ 00000000 (usable)
[    0.000000]  memory: 04000000 @ 20000000 (usable)
[    0.000000] Initrd not found or empty - disabling initrd
[    0.000000] VPE topology {2,2} total 4
[    0.000000] Primary instruction cache 32kB, VIPT, 4-way, linesize 32 bytes.
[    0.000000] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.000000] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.000000] Zone ranges:
[    0.000000]   Normal   [mem 0x0000000000000000-0x000000000fffffff]
[    0.000000]   HighMem  [mem 0x0000000010000000-0x0000000023ffffff]
[    0.000000] Movable zone start for each node
[    0.000000] Early memory node ranges
[    0.000000]   node   0: [mem 0x0000000000000000-0x000000001bffffff]
[    0.000000]   node   0: [mem 0x0000000020000000-0x0000000023ffffff]
[    0.000000] Initmem setup node 0 [mem 0x0000000000000000-0x0000000023ffffff]
[    0.000000] On node 0 totalpages: 131072
[    0.000000] free_area_init_node: node 0, pgdat 805aa2c0, node_mem_map 81003000
[    0.000000]   Normal zone: 512 pages used for memmap
[    0.000000]   Normal zone: 0 pages reserved
[    0.000000]   Normal zone: 65536 pages, LIFO batch:15
[    0.000000]   HighMem zone: 65536 pages, LIFO batch:15
[    0.000000] random: get_random_bytes called from start_kernel+0x90/0x4a4 with crng_init=0
[    0.000000] percpu: Embedded 14 pages/cpu @81491000 s26192 r8192 d22960 u57344
[    0.000000] pcpu-alloc: s26192 r8192 d22960 u57344 alloc=14*4096
[    0.000000] pcpu-alloc: [0] 0 [0] 1 [0] 2 [0] 3 
[    0.000000] Built 1 zonelists, mobility grouping on.  Total pages: 130560
[    0.000000] Kernel command line: console=ttyS0,115200n8 rootfstype=squashfs,jffs2
[    0.000000] PID hash table entries: 1024 (order: 0, 4096 bytes)
[    0.000000] Dentry cache hash table entries: 32768 (order: 5, 131072 bytes)
[    0.000000] Inode-cache hash table entries: 16384 (order: 4, 65536 bytes)
[    0.000000] Writing ErrCtl register=00000850
[    0.000000] Readback ErrCtl register=00000850
[    0.000000] Memory: 512816K/524288K available (4552K kernel code, 234K rwdata, 1012K rodata, 268K init, 248K bss, 11472K reserved, 0K cma-reserved, 262144K highmem)
[    0.000000] SLUB: HWalign=32, Order=0-3, MinObjects=0, CPUs=4, Nodes=1
[    0.000000] Hierarchical RCU implementation.
[    0.000000] NR_IRQS: 256
[    0.000000] CPU Clock: 880MHz
[    0.000000] clocksource: GIC: mask: 0xffffffffffffffff max_cycles: 0xcaf478abb4, max_idle_ns: 440795247997 ns
[    0.000000] clocksource: MIPS: mask: 0xffffffff max_cycles: 0xffffffff, max_idle_ns: 4343773742 ns
[    0.000009] sched_clock: 32 bits at 440MHz, resolution 2ns, wraps every 4880645118ns
[    0.007811] Calibrating delay loop... 586.13 BogoMIPS (lpj=2930688)
[    0.073975] pid_max: default: 32768 minimum: 301
[    0.078744] Mount-cache hash table entries: 1024 (order: 0, 4096 bytes)
[    0.085254] Mountpoint-cache hash table entries: 1024 (order: 0, 4096 bytes)
[    0.094061] Hierarchical SRCU implementation.
[    0.099242] smp: Bringing up secondary CPUs ...
[    0.107585] Primary instruction cache 32kB, VIPT, 4-way, linesize 32 bytes.
[    0.107594] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.107606] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.107742] CPU1 revision is: 0001992f (MIPS 1004Kc)
[    0.163991] Synchronize counters for CPU 1: done.
[    0.201446] Primary instruction cache 32kB, VIPT, 4-way, linesize 32 bytes.
[    0.201455] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.201461] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.201536] CPU2 revision is: 0001992f (MIPS 1004Kc)
[    0.255160] Synchronize counters for CPU 2: done.
[    0.288752] Primary instruction cache 32kB, VIPT, 4-way, linesize 32 bytes.
[    0.288760] Primary data cache 32kB, 4-way, PIPT, no aliases, linesize 32 bytes
[    0.288768] MIPS secondary cache 256kB, 8-way, linesize 32 bytes.
[    0.288850] CPU3 revision is: 0001992f (MIPS 1004Kc)
[    0.340340] Synchronize counters for CPU 3: done.
[    0.370205] smp: Brought up 1 node, 4 CPUs
[    0.378063] clocksource: jiffies: mask: 0xffffffff max_cycles: 0xffffffff, max_idle_ns: 19112604462750000 ns
[    0.387859] futex hash table entries: 1024 (order: 3, 32768 bytes)
[    0.394158] pinctrl core: initialized pinctrl subsystem
[    0.400704] NET: Registered protocol family 16
[    0.415067] FPU Affinity set after 11720 emulations
[    0.415957] pull PCIe RST: RALINK_RSTCTRL = 4000000
[    0.721148] release PCIe RST: RALINK_RSTCTRL = 7000000
[    0.726181] ***** Xtal 40MHz *****
[    0.729551] release PCIe RST: RALINK_RSTCTRL = 7000000
[    0.734659] Port 0 N_FTS = 1b105000
[    0.738103] Port 1 N_FTS = 1b105000
[    0.741563] Port 2 N_FTS = 1b102800
[    1.896676] PCIE2 no card, disable it(RST&CLK)
[    1.901022]  -> 21007f2
[    1.903441] PCIE0 enabled
[    1.906047] PCIE1 enabled
[    1.908634] PCI host bridge /pcie@1e140000 ranges:
[    1.913394]  MEM 0x0000000060000000..0x000000006fffffff
[    1.918579]   IO 0x000000001e160000..0x000000001e16ffff
[    1.923752] PCI coherence region base: 0xbfbf8000, mask/settings: 0x60000000
[    1.939336] mt7621_gpio 1e000600.gpio: registering 32 gpios
[    1.945210] mt7621_gpio 1e000600.gpio: registering 32 gpios
[    1.950947] mt7621_gpio 1e000600.gpio: registering 32 gpios
[    1.958603] PCI host bridge to bus 0000:00
[    1.962647] pci_bus 0000:00: root bus resource [mem 0x60000000-0x6fffffff]
[    1.969471] pci_bus 0000:00: root bus resource [io  0xffffffff]
[    1.975326] pci_bus 0000:00: root bus resource [??? 0x00000000 flags 0x0]
[    1.982081] pci_bus 0000:00: No busn resource found for root bus, will use [bus 00-ff]
[    1.989977] pci 0000:00:00.0: [0e8d:0801] type 01 class 0x060400
[    1.990018] pci 0000:00:00.0: reg 0x10: [mem 0x00000000-0x7fffffff]
[    1.990032] pci 0000:00:00.0: reg 0x14: [mem 0x60200000-0x6020ffff]
[    1.990096] pci 0000:00:00.0: supports D1
[    1.990105] pci 0000:00:00.0: PME# supported from D0 D1 D3hot
[    1.990356] pci 0000:00:01.0: [0e8d:0801] type 01 class 0x060400
[    1.990396] pci 0000:00:01.0: reg 0x10: [mem 0x00000000-0x7fffffff]
[    1.990416] pci 0000:00:01.0: reg 0x14: [mem 0x60210000-0x6021ffff]
[    1.990479] pci 0000:00:01.0: supports D1
[    1.990489] pci 0000:00:01.0: PME# supported from D0 D1 D3hot
[    1.990966] pci 0000:01:00.0: [14c3:7615] type 00 class 0x000280
[    1.991028] pci 0000:01:00.0: reg 0x10: [mem 0x00000000-0x000fffff 64bit]
[    1.991358] pci_bus 0000:01: busn_res: [bus 01-ff] end is updated to 01
[    1.991620] pci 0000:02:00.0: [14c3:7615] type 00 class 0x000280
[    1.991685] pci 0000:02:00.0: reg 0x10: [mem 0x00000000-0x000fffff 64bit]
[    1.992012] pci_bus 0000:02: busn_res: [bus 02-ff] end is updated to 02
[    1.992037] pci_bus 0000:00: busn_res: [bus 00-ff] end is updated to 02
[    1.992161] pci 0000:00:00.0: BAR 0: no space for [mem size 0x80000000]
[    1.998676] pci 0000:00:00.0: BAR 0: failed to assign [mem size 0x80000000]
[    2.005607] pci 0000:00:01.0: BAR 0: no space for [mem size 0x80000000]
[    2.012150] pci 0000:00:01.0: BAR 0: failed to assign [mem size 0x80000000]
[    2.019080] pci 0000:00:00.0: BAR 8: assigned [mem 0x60000000-0x600fffff]
[    2.025810] pci 0000:00:01.0: BAR 8: assigned [mem 0x60100000-0x601fffff]
[    2.032561] pci 0000:00:00.0: BAR 1: assigned [mem 0x60200000-0x6020ffff]
[    2.039292] pci 0000:00:01.0: BAR 1: assigned [mem 0x60210000-0x6021ffff]
[    2.046051] pci 0000:01:00.0: BAR 0: assigned [mem 0x60000000-0x600fffff 64bit]
[    2.053288] pci 0000:00:00.0: PCI bridge to [bus 01]
[    2.058218] pci 0000:00:00.0:   bridge window [mem 0x60000000-0x600fffff]
[    2.064957] pci 0000:02:00.0: BAR 0: assigned [mem 0x60100000-0x601fffff 64bit]
[    2.072227] pci 0000:00:01.0: PCI bridge to [bus 02]
[    2.077128] pci 0000:00:01.0:   bridge window [mem 0x60100000-0x601fffff]
[    2.085323] clocksource: Switched to clocksource GIC
[    2.091810] NET: Registered protocol family 2
[    2.096852] TCP established hash table entries: 2048 (order: 1, 8192 bytes)
[    2.103736] TCP bind hash table entries: 2048 (order: 2, 16384 bytes)
[    2.110185] TCP: Hash tables configured (established 2048 bind 2048)
[    2.116628] UDP hash table entries: 256 (order: 1, 8192 bytes)
[    2.122393] UDP-Lite hash table entries: 256 (order: 1, 8192 bytes)
[    2.128865] NET: Registered protocol family 1
[    2.133186] PCI: CLS 80 bytes, default 32
[    2.365265] 4 CPUs re-calibrate udelay(lpj = 2924544)
[    2.371537] Crashlog allocated RAM at address 0x3f00000
[    2.376973] workingset: timestamp_bits=30 max_order=17 bucket_order=0
[    2.389466] squashfs: version 4.0 (2009/01/31) Phillip Lougher
[    2.395208] jffs2: version 2.2 (NAND) (SUMMARY) (LZMA) (RTIME) (CMODE_PRIORITY) (c) 2001-2006 Red Hat, Inc.
[    2.406843] random: fast init done
[    2.411697] bounce: pool size: 64 pages
[    2.415522] io scheduler noop registered
[    2.419346] io scheduler deadline registered (default)
[    2.425589] hsdma-mt7621 1e007000.hsdma: rx: 1, tx: 1
[    2.431107] Serial: 8250/16550 driver, 3 ports, IRQ sharing disabled
[    2.438812] console [ttyS0] disabled
[    2.442351] 1e000c00.uartlite: ttyS0 at MMIO 0x1e000c00 (irq = 18, base_baud = 3125000) is a 16550A
[    2.451361] console [ttyS0] enabled
[    2.458232] bootconsole [early0] disabled
[    2.466870] cacheinfo: Unable to detect cache hierarchy for CPU 0
[    2.473702] MediaTek Nand driver init, version v2.1 Fix AHB virt2phys error
[    2.480842] Enable NFI Clock
[    2.483714] # MTK NAND # : Use HW ECC
[    2.487391] Device found in MTK table, ID: 2cda, EXT_ID: 909506
[    2.493283] Support this Device in MTK table! 2cda 
[    2.498243] [NAND]select ecc bit:4, sparesize :64 spare_per_sector=16
[    2.504702] nand: device found, Manufacturer ID: 0x2c, Chip ID: 0xda
[    2.511034] nand: Micron NAND 256MiB 3,3V 8-bit
[    2.515556] nand: 256 MiB, SLC, erase size: 128 KiB, page size: 2048, OOB size: 64
[    2.523107] Scanning device for bad blocks
[    2.565639] Bad eraseblock 150 at 0x0000012c0000
[    3.054674] 10 fixed-partitions partitions found on MTD device MT7621-NAND
[    3.061529] Creating 10 MTD partitions on "MT7621-NAND":
[    3.066838] 0x000000000000-0x000000040000 : "Bootloader"
[    3.073087] 0x000000040000-0x000000080000 : "Config"
[    3.078903] 0x000000080000-0x0000000c0000 : "Bdata"
[    3.084631] 0x0000000c0000-0x000000100000 : "Factory"
[    3.090551] 0x000000100000-0x000000140000 : "crash"
[    3.096328] 0x000000140000-0x0000001c0000 : "crash_syslog"
[    3.102631] 0x0000001c0000-0x000000200000 : "reserved0"
[    3.108727] 0x000000200000-0x000000600000 : "kernel_stock"
[    3.115047] 0x000000600000-0x000000a00000 : "kernel"
[    3.120903] 0x000000a00000-0x00000ff80000 : "ubi"
[    3.128369] [mtk_nand] probe successfully!
[    3.133164] Signature matched and data read!
[    3.137430] load_fact_bbt success 2047
[    3.141826] libphy: Fixed MDIO Bus: probed
[    3.217887] libphy: mdio: probed
[    4.621562] mtk_soc_eth 1e100000.ethernet: loaded mt7530 driver
[    4.628236] mtk_soc_eth 1e100000.ethernet eth0: mediatek frame engine at 0xbe100000, irq 21
[    4.639140] NET: Registered protocol family 10
[    4.644960] Segment Routing with IPv6
[    4.648769] NET: Registered protocol family 17
[    4.653233] 8021q: 802.1Q VLAN Support v1.8
[    4.660470] UBI: auto-attach mtd9
[    4.663806] ubi0: attaching mtd9
[    6.000310] mtk_soc_eth 1e100000.ethernet eth0: port 1 link up
[    7.008768] ubi0: scanning is finished
[    7.030997] ubi0: attached mtd9 (name "ubi", size 245 MiB)
[    7.036500] ubi0: PEB size: 131072 bytes (128 KiB), LEB size: 126976 bytes
[    7.043340] ubi0: min./max. I/O unit sizes: 2048/2048, sub-page size 2048
[    7.050108] ubi0: VID header offset: 2048 (aligned 2048), data offset: 4096
[    7.057049] ubi0: good PEBs: 1963, bad PEBs: 1, corrupted PEBs: 0
[    7.063112] ubi0: user volume: 3, internal volumes: 1, max. volumes count: 128
[    7.070312] ubi0: max/mean erase counter: 1/0, WL threshold: 4096, image sequence number: 705533410
[    7.079340] ubi0: available PEBs: 0, total reserved PEBs: 1963, PEBs reserved for bad PEB handling: 39
[    7.088640] ubi0: background thread "ubi_bgt0d" started, PID 377
[    7.090656] block ubiblock0_1: created from ubi0:1(rootfs)
[    7.100116] ubiblock: device ubiblock0_1 (rootfs) set to be root filesystem
[    7.107074] hctosys: unable to open rtc device (rtc0)
[    7.112357] usb_vbus: disabling
[    7.122485] VFS: Mounted root (squashfs filesystem) readonly on device 254:0.
[    7.130255] Freeing unused kernel memory: 268K
[    7.134692] This architecture does not have kernel memory protection.
[    7.641373] init: Console is alive
[    7.644982] init: - watchdog -
[    8.264629] kmodloader: loading kernel modules from /etc/modules-boot.d/*
[    8.321102] usbcore: registered new interface driver usbfs
[    8.326711] usbcore: registered new interface driver hub
[    8.332203] usbcore: registered new device driver usb
[    8.345486] SCSI subsystem initialized
[    8.357645] xhci-mtk 1e1c0000.xhci: 1e1c0000.xhci supply vusb33 not found, using dummy regulator
[    8.366632] xhci-mtk 1e1c0000.xhci: xHCI Host Controller
[    8.371953] xhci-mtk 1e1c0000.xhci: new USB bus registered, assigned bus number 1
[    8.385475] xhci-mtk 1e1c0000.xhci: hcc params 0x01401198 hci version 0x96 quirks 0x0000000000210010
[    8.394642] xhci-mtk 1e1c0000.xhci: irq 20, io mem 0x1e1c0000
[    8.401377] hub 1-0:1.0: USB hub found
[    8.405196] hub 1-0:1.0: 2 ports detected
[    8.409910] xhci-mtk 1e1c0000.xhci: xHCI Host Controller
[    8.415224] xhci-mtk 1e1c0000.xhci: new USB bus registered, assigned bus number 2
[    8.422753] xhci-mtk 1e1c0000.xhci: Host supports USB 3.0  SuperSpeed
[    8.429397] usb usb2: We don't know the algorithms for LPM for this host, disabling LPM.
[    8.438347] hub 2-0:1.0: USB hub found
[    8.442169] hub 2-0:1.0: 1 port detected
[    8.453996] usbcore: registered new interface driver usb-storage
[    8.460304] kmodloader: done loading kernel modules from /etc/modules-boot.d/*
[    8.485632] init: - preinit -
[    9.185069] mtk_soc_eth 1e100000.ethernet: PPE started
[    9.260577] random: procd: uninitialized urandom read (4 bytes read)
[   12.387065] UBIFS (ubi0:2): background thread "ubifs_bgt0_2" started, PID 492
[   12.463344] UBIFS (ubi0:2): recovery needed
[   12.658619] UBIFS (ubi0:2): recovery completed
[   12.663202] UBIFS (ubi0:2): UBIFS: mounted UBI device 0, volume 2, name "rootfs_data"
[   12.671013] UBIFS (ubi0:2): LEB size: 126976 bytes (124 KiB), min./max. I/O unit sizes: 2048 bytes/2048 bytes
[   12.680900] UBIFS (ubi0:2): FS size: 74915840 bytes (71 MiB, 590 LEBs), journal size 3809280 bytes (3 MiB, 30 LEBs)
[   12.691296] UBIFS (ubi0:2): reserved for root: 3538460 bytes (3455 KiB)
[   12.697894] UBIFS (ubi0:2): media format: w4/r0 (latest is w5/r0), UUID 6F2BF812-4D5C-4860-87F3-D7F70767894B, small LPT model
[   12.714767] mount_root: switching to ubifs overlay
[   12.748811] urandom-seed: Seeding with /etc/urandom.seed
[   12.842213] mtk_soc_eth 1e100000.ethernet: 0x100 = 0x6060000c, 0x10c = 0x80818
[   12.857164] procd: - early -
[   12.860147] procd: - watchdog -
[   13.535483] procd: - watchdog -
[   13.538936] procd: - ubus -
[   13.616200] random: ubusd: uninitialized urandom read (4 bytes read)
[   13.626308] random: ubusd: uninitialized urandom read (4 bytes read)
[   13.633036] random: ubusd: uninitialized urandom read (4 bytes read)
[   13.640235] procd: - init -
[   13.953314] kmodloader: loading kernel modules from /etc/modules.d/*
[   13.973150] ip6_tables: (C) 2000-2006 Netfilter Core Team
[   13.987044] ip_tables: (C) 2000-2006 Netfilter Core Team
[   13.999166] nf_conntrack version 0.5.0 (8192 buckets, 32768 max)
[   14.043039] xt_time: kernel timezone is -0000
[   14.053497] PPP generic driver version 2.4.2
[   14.059049] NET: Registered protocol family 24
[   14.066002] kmodloader: done loading kernel modules from /etc/modules.d/*
[   15.618397] urandom_read: 5 callbacks suppressed
[   15.618409] random: jshn: uninitialized urandom read (4 bytes read)
[   20.128348] mtk_soc_eth 1e100000.ethernet: PPE started
[   20.139128] device eth0 entered promiscuous mode
[   20.144798] br-lan: port 1(eth0.1) entered blocking state
[   20.150365] br-lan: port 1(eth0.1) entered disabled state
[   20.156901] device eth0.1 entered promiscuous mode
[   20.166517] br-lan: port 1(eth0.1) entered blocking state
[   20.172024] br-lan: port 1(eth0.1) entered forwarding state
[   20.178143] IPv6: ADDRCONF(NETDEV_UP): br-lan: link is not ready
[   20.256898] random: crng init done
[   21.155821] IPv6: ADDRCONF(NETDEV_CHANGE): br-lan: link becomes ready
[   23.324737] register mt_drv
[   23.327707] bus=0x1, slot = 0x0, irq=0x0
[   23.349579] 
[   23.349579] == pAd = c0a81000, size = 3969984, Status=0 ==
[   23.356571] pAd->PciHif.CSRBaseAddress =0xc0980000, csr_addr=0xc0980000!
[   23.363252] RTMPInitPCIeDevice():device_id=0x7615
[   23.368017] mt_pci_chip_cfg(): HWVer=0x8a10, FWVer=0x8a10, pAd->ChipID=0x7615
[   23.375140] mt_pci_chip_cfg(): HIF_SYS_REV=0x76150001
[   23.380267] AP Driver version-5.0.2.0
[   23.383940] RtmpChipOpsHook(223): Not support for HIF_MT yet! MACVersion=0x0
[   23.391012] mt7615_init()-->
[   23.393895] Use 1st ePAeLNA default bin.
[   23.397862] Use 0st /lib/firmware/mt7615e.1.eeprom.bin default bin.
[   23.404149] <--mt7615_init()
[   23.411394] <-- RTMPAllocTxRxRingMemory, Status=0
[   23.416453] RtmpOSFileOpen(): Error 2 opening /etc/wireless/l1profile.dat
[   23.426076] bus=0x2, slot = 0x1, irq=0x0
[   23.427153] ra0: ===> main_virtual_if_open
[   23.434221] RtmpOSFileOpen(): Error 2 opening /etc/wireless/l1profile.dat
[   23.443309] load l1profile succeed!
[   23.447607] driver_own()::Try to Clear FW Own...
[   23.451658] 
[   23.451658] == pAd = c0f81000, size = 3969984, Status=0 ==
[   23.459341] pAd->PciHif.CSRBaseAddress =0xc0e80000, csr_addr=0xc0e80000!
[   23.466128] RTMPInitPCIeDevice():device_id=0x7615
[   23.470893] mt_pci_chip_cfg(): HWVer=0x8a10, FWVer=0x8a10, pAd->ChipID=0x7615
[   23.478119] mt_pci_chip_cfg(): HIF_SYS_REV=0x76150001
[   23.483240] AP Driver version-5.0.2.0
[   23.487035] RtmpChipOpsHook(223): Not support for HIF_MT yet! MACVersion=0x0
[   23.494141] mt7615_init()-->
[   23.497090] Use 2nd ePAeLNA default bin.
[   23.501052] Use the default /lib/firmware/mt7615e.1.eeprom.bin bin image!
[   23.507935] <--mt7615_init()
[   23.515609] <-- RTMPAllocTxRxRingMemory, Status=0
[   23.520474] RtmpOSFileOpen(): Error 2 opening /etc/wireless/l1profile.dat
[   23.822333] driver_own()::Success to clear FW Own
[   23.829215] APWdsInitialize():WdsEntry Number=4
[   23.842528] RtmpOSFileOpen(): Error 2 opening /etc/Wireless/RT2860/RT2860_5G.dat
[   23.849954] Open file "/etc/Wireless/RT2860/RT2860_5G.dat" failed!
[   23.857652] E2pAccessMode=2
[   23.861412] SSID[0]=openwrt-r3p, EdcaIdx=0
[   23.866789] BSS0 PhyMode=14
[   23.869589] wmode_band_equal(): Band Equal!
[   23.875947] BandSteering=0
[   23.878872] BndStrgBssIdx=
[   23.881918] [TxPower] BAND0: 100 
[   23.885923] Power Boost (CCK, OFDM): RTMPSetProfileParameters
[   23.885951] [PowerUpCckOfdm] BAND1: (0)-(0)-(0)-(0)-(0)-(0)-(0)
[   23.897984] Power Boost (HT20): RTMPSetProfileParameters
[   23.898305] Power Boost (HT40): RTMPSetProfileParameters
[   23.903911] Power Boost (VHT20): RTMPSetProfileParameters
[   23.909588] Power Boost (VHT40): RTMPSetProfileParameters
[   23.915340] Power Boost (VHT80): RTMPSetProfileParameters
[   23.921045] Power Boost (VHT160): RTMPSetProfileParameters
[   23.927506] [PERCENTAGEenable] BAND0: 0 
[   23.937138] [BFBACKOFFenable] BAND0: 0 
[   23.941185] CalCacheApply = 0 
[   23.946025] FragThreshold[0]=2346
[   23.957733] DfsZeroWait Support=0/0
[   23.962606] DfsZeroWaitCacTime=255/255
[   23.967889] read_etxbf: ETxBfEnCond = 0
[   23.971722] read_etxbf: BSSID[0]
[   23.974940] read_etxbf: MBSS[0] ETxBfEnCond = 0
[   24.006371] If/wds0 - PeerPhyOpMode=255
[   24.011160] APWdsInitialize():WdsEntry Number=4
[   24.015689] WDS-Enable mode=0
[   24.018862] AndesSendCmdMsg: Could not send in band command due to diablefRTMP_ADAPTER_MCU_SEND_IN_BAND_CMD
[   24.028584] AndesSendCmdMsg: Command type = ed, Extension command type = 48
[   24.039401] HT: WDEV[0] Ext Channel = ABOVE
[   24.043887] HT: greenap_cap = 1
[   24.047223] ChipI=7615, Value=0, pcie_aspm in profile=0
[   24.080352] ICapMode = 0
[   24.089407] WtcSetMaxStaNum: BssidNum:1, MaxStaNum:124 (WdsNum:4, ApcliNum:2, MaxNumChipRept:32), MinMcastWcid:125
[   24.099907] Top Init Done!
[   24.102612] Use dev_alloc_skb
[   24.106785] RX[0] DESC ae77c000 size = 16384
[   24.112499] RX[1] DESC affb0000 size = 8192
[   24.117399] cut_through_init(): ct sw token number = 4095
[   24.123215] cut_through_token_list_init(): TokenList inited done!id_head/tail=0/4096
[   24.130981] cut_through_token_list_init(): 8f783e08,8f783e08
[   24.137176] cut_through_token_list_init(): TokenList inited done!id_head/tail=0/4096
[   24.144889] cut_through_token_list_init(): 8f783e18,8f783e18
[   24.150595] Hif Init Done!
[   24.153296] ctl->txq = c0e44e6c
[   24.156441] ctl->rxq = c0e44e78
[   24.159565] ctl->ackq = c0e44e84
[   24.162772] ctl->kickq = c0e44e90
[   24.166086] ctl->tx_doneq = c0e44e9c
[   24.169643] ctl->rx_doneq = c0e44ea8
[   24.173269] Parsing patch header
[   24.176499] 	Built date: 
[   24.176506] 2
[   24.179110] 0
[   24.180674] 1
[   24.182237] 6
[   24.183801] 0
[   24.185385] 3
[   24.186952] 1
[   24.188515] 4
[   24.190078] 1
[   24.191641] 5
[   24.193205] 1
[   24.194768] 8
[   24.196353] 1
[   24.197920] 9
[   24.199483] a
[   24.201046] 
[   24.204082] 
[   24.205579] 	Platform: 
[   24.205584] A
[   24.208015] L
[   24.209579] P
[   24.211143] S
[   24.212707] 
[   24.215770] 	HW/SW version: 
[   24.215774] 0x
[   24.218645] 8a
[   24.220296] 10
[   24.221947] 8a
[   24.223597] 10
[   24.225247] 
[   24.228393] 	Patch version: 
[   24.228398] 0x
[   24.231259] 00
[   24.232908] 00
[   24.234558] 00
[   24.236230] 10
[   24.237882] 
[   24.241014] 	Target address: 0x80000, length: 3120
[   24.245984] patch is not ready && get semaphore success
[   24.251248] EventGenericEventHandler: CMD Success
[   24.256039] MtCmdPatchFinishReq
[   24.259300] EventGenericEventHandler: CMD Success
[   24.264112] release patch semaphore
[   24.267651] WfMcuHwInit: Before NICLoadFirmware, check ICapMode = 0
[   24.273899] Parsing CPU 0 fw tailer
[   24.277394] 	Chip ID: 
[   24.277412] 0x
[   24.279757] 04
[   24.281407] 
[   24.284531] 	Eco version: 
[   24.284534] 0x
[   24.287243] 00
[   24.288895] 
[   24.292020] 	Region number: 
[   24.292023] 0x
[   24.294883] 00
[   24.296558] 
[   24.299685] 	Format version: 
[   24.299688] 0x
[   24.302634] 00
[   24.304284] 
[   24.307428] 	Ram version: 
[   24.307434] _
[   24.310121] r
[   24.311684] e
[   24.313247] s
[   24.314810] e
[   24.316398] r
[   24.317965] v
[   24.319529] e
[   24.321092] d
[   24.322656] _
[   24.324219] 
[   24.327277] 	Built date: 
[   24.327281] 2
[   24.329881] 0
[   24.331444] 1
[   24.333007] 8
[   24.334570] 0
[   24.336157] 4
[   24.337724] 2
[   24.339287] 7
[   24.340850] 1
[   24.342413] 5
[   24.343976] 1
[   24.345559] 4
[   24.347125] 5
[   24.348688] 4
[   24.350251] 
[   24.351815] 
[   24.354852] 	Common crc: 0x0
[   24.357752] Parsing tailer region 0
[   24.361226] 	Feature set: 
[   24.361229] 0x
[   24.363917] 01
[   24.365586] 
[   24.368716] 	Target address: 0x84000, Download size: 409600
[   24.374256] Parsing tailer region 1
[   24.377767] 	Feature set: 
[   24.377771] 0x
[   24.380461] 01
[   24.382111] 
[   24.385236] 	Target address: 0x209c400, Download size: 51072
[   24.390990] EventGenericEventHandler: CMD Success
[   24.399971] EventGenericEventHandler: CMD Success
[   24.405174] MtCmdFwStartReq: override = 0x1, address = 0x84000
[   24.411132] EventGenericEventHandler: CMD Success
[   24.415867] Parsing CPU 1 fw tailer
[   24.419339] 	Chip ID: 
[   24.419343] 0x
[   24.421685] 04
[   24.423335] 
[   24.426489] 	Eco version: 
[   24.426493] 0x
[   24.429183] 00
[   24.430834] 
[   24.433957] 	Region number: 
[   24.433960] 0x
[   24.436859] 00
[   24.438516] 
[   24.441640] 	Format version: 
[   24.441643] 0x
[   24.444590] 00
[   24.446266] 
[   24.449394] 	Ram version: 
[   24.449398] _
[   24.452086] r
[   24.453649] e
[   24.455212] s
[   24.456796] e
[   24.458363] r
[   24.459926] v
[   24.461489] e
[   24.463052] d
[   24.464617] _
[   24.466206] 
[   24.469247] 	Built date: 
[   24.469251] 2
[   24.471851] 0
[   24.473414] 1
[   24.474977] 8
[   24.476561] 0
[   24.478128] 3
[   24.479691] 1
[   24.481254] 5
[   24.482817] 1
[   24.484380] 0
[   24.485969] 4
[   24.487536] 0
[   24.489099] 0
[   24.490662] 0
[   24.492225] 
[   24.493790] 
[   24.496851] 	Common crc: 0x0
[   24.499715] Parsing tailer region 0
[   24.503181] 	Feature set: 
[   24.503184] 0x
[   24.505897] 01
[   24.507550] 
[   24.510676] 	Target address: 0x90000000, Download size: 118976
[   24.516583] EventGenericEventHandler: CMD Success
[   24.522265] MtCmdFwStartReq: override = 0x4, address = 0x0
[   24.579507] EventGenericEventHandler: CMD Success
[   24.632277] MCU Init Done!
[   24.634980] efuse_probe: efuse = 10000212
[   24.638991] RtmpChipOpsEepromHook::e2p_type=2, inf_Type=5
[   24.644363] RtmpEepromGetDefault::e2p_dafault=1
[   24.648906] RtmpChipOpsEepromHook: E2P type(2), E2pAccessMode = 2, E2P default = 1
[   24.656469] NVM is FLASH mode. dev_idx [0] FLASH OFFSET [0x0]
[   24.667340] [34mNICReadEEPROMParameters: EEPROM 0x52 b300[m
[   32.152532] Country Region from e2p = 1
[   32.156391] mt7615_antenna_default_reset(): TxPath = 4, RxPath = 4
[   32.162547] RcRadioInit(): DbdcMode=0, ConcurrentBand=1
[   32.167778] RcRadioInit(): pRadioCtrl=8f730afc,Band=0,rfcap=3,channel=1,PhyMode=2 extCha=0xf
[   32.176214] [34mMtSingleSkuLoadParam: RF_LOCKDOWN Feature ON !!!
[   32.176214] [0m
[   32.176222] [34mMtSingleSkuLoadParam: SKU Table index = 0 
[   32.176222] [0m
[   32.185178] MtBfBackOffLoadParam: RF_LOCKDOWN Feature ON !!!
[   32.198199] MtBfBackOffLoadParam: BFBackoff Table index = 0 
[   32.204088] EEPROM Init Done!
[   32.207063] mt_mac_init()-->
[   32.209929] mt7615_init_mac_cr()-->
[   32.213406] mt7615_init_mac_cr(): TMAC_TRCR0=0x82783c8c
[   32.218664] <--mt_mac_init()
[   32.221742] CmdRxHdrTransBLUpdateRsp::EventExtCmdResult.u4Status = 0x0
[   32.228353] CmdRxHdrTransBLUpdateRsp::EventExtCmdResult.u4Status = 0x0
[   32.234933] CmdRxHdrTransBLUpdateRsp::EventExtCmdResult.u4Status = 0x0
[   32.241495] MAC Init Done!
[   32.244202] MT7615BBPInit():BBP Initialization.....
[   32.249090] 	Band 0: valid=1, isDBDC=0, Band=2, CBW=1, CentCh/PrimCh=1/1, prim_ch_idx=0, txStream=2
[   32.258115] 	Band 1: valid=0, isDBDC=0, Band=0, CBW=0, CentCh/PrimCh=0/0, prim_ch_idx=0, txStream=0
[   32.267133] MT7615BBPInit() todo
[   32.270342] PHY Init Done!
[   32.275816] MtCmdSetMacTxRx:(ret = 0)
[   32.279684] WifiFwdSet::disabled=0
[   32.283082] Main bssid = 00:00:00:00:00:00
[   32.290738] MtCmdSetMacTxRx:(ret = 0)
[   32.294556] MtCmdSetMacTxRx:(ret = 0)
[   32.298232] <==== mt_wifi_init, Status=0
[   32.302134] TxBfModuleEnCtrl:It's not DBDC mode
[   32.306690] MtCmdEDCCACtrl: BandIdx: 0, EDCCACtrl: 1
[   32.311766] The new WDS interface MAC = 00:00:00:00:00:00, MacTabMatchWCID = 0
[   32.320336] The new WDS interface MAC = 00:00:00:00:00:00, MacTabMatchWCID = 0
[   32.329217] The new WDS interface MAC = 00:00:00:00:00:00, MacTabMatchWCID = 0
[   32.337978] The new WDS interface MAC = 00:00:00:00:00:00, MacTabMatchWCID = 0
[   32.346637] Total allocated 4 WDS interfaces!
[   32.354556] WtcSetMaxStaNum: BssidNum:1, MaxStaNum:124 (WdsNum:4, ApcliNum:2, MaxNumChipRept:32), MinMcastWcid:125
[   32.412641] RedInit: set CR4/N9 RED Enable to 1.
[   32.417302] RedInit: RED Initiailize Done.
[   32.421474] cp_support_is_enabled: set CR4 CP_SUPPORT to Mode 2.
[   32.427502] RTMP_COM_IoctlHandle -> CMD_RTPRIV_IOCTL_VIRTUAL_INF_UP
[   32.433816] wifi_sys_open(), wdev idx = 0
[   32.437843] wdev_attr_update(): wdevId0 = ec:41:18:07:81:91
[   32.443536] [1;33m [RadarStateCheck] RD_NORMAL_MODE [m 
[   32.448932] phy_freq_adjust : no prim_ch value for adjust!
[   32.456990] Caller: wlan_operate_init+0xf0/0x104 [mt7615e]
[   32.462522] phy_mode=14, ch=0, wdev_type=1
[   32.466616] ht_cap->HtCapInfo: ldpc=1,ch_width=1,gf=0,sgi20=1,sgi40=1,tx_stbc=1,rx_stbc=1,amsdu_size=1
[   32.475910] ht_cap->HtCapParm: mdpu_density=5, ampdu_factor=3
[   32.481985] AP inf up for ra_0(func_idx) OmacIdx=0
[   32.486964] AsicRadioOnOffCtrl(): DbdcIdx=0 RadioOn
[   32.492081] ApAutoChannelAtBootUp----------------->
[   32.497035] ApAutoChannelAtBootUp: AutoChannelBootup = 1
[   32.502674] MtCmdSetMacTxRx:(ret = 0)
[   32.506523] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   32.515075] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   32.523659] MtCmdChannelSwitch: control_chl = 1,control_ch2=0, central_chl = 1 DBDCIdx= 0, Band= 0
[   32.532599] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   33.115422] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   33.123933] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   33.132526] MtCmdChannelSwitch: control_chl = 2,control_ch2=0, central_chl = 2 DBDCIdx= 0, Band= 0
[   33.141498] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   33.475835] :MtCmdPktBudgetCtrl: bssid(255),wcid(65535),type(0)
[   33.725346] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   33.733851] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   33.742362] MtCmdChannelSwitch: control_chl = 3,control_ch2=0, central_chl = 3 DBDCIdx= 0, Band= 0
[   33.751293] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   34.335318] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   34.343817] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   34.352315] MtCmdChannelSwitch: control_chl = 4,control_ch2=0, central_chl = 4 DBDCIdx= 0, Band= 0
[   34.361409] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   34.955320] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   34.963817] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   34.972316] MtCmdChannelSwitch: control_chl = 5,control_ch2=0, central_chl = 5 DBDCIdx= 0, Band= 0
[   34.981242] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   35.565334] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   35.573830] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   35.582327] MtCmdChannelSwitch: control_chl = 6,control_ch2=0, central_chl = 6 DBDCIdx= 0, Band= 0
[   35.591253] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   36.175311] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   36.183807] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   36.192305] MtCmdChannelSwitch: control_chl = 7,control_ch2=0, central_chl = 7 DBDCIdx= 0, Band= 0
[   36.201233] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   36.785326] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   36.793824] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   36.802324] MtCmdChannelSwitch: control_chl = 8,control_ch2=0, central_chl = 8 DBDCIdx= 0, Band= 0
[   36.811249] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   37.395407] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   37.403938] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   37.412521] MtCmdChannelSwitch: control_chl = 9,control_ch2=0, central_chl = 9 DBDCIdx= 0, Band= 0
[   37.421575] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   38.015321] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   38.023827] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   38.032328] MtCmdChannelSwitch: control_chl = 10,control_ch2=0, central_chl = 10 DBDCIdx= 0, Band= 0
[   38.041443] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   38.625316] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   38.633813] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   38.642313] MtCmdChannelSwitch: control_chl = 11,control_ch2=0, central_chl = 11 DBDCIdx= 0, Band= 0
[   38.651414] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   39.235315] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   39.243811] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   39.252315] MtCmdChannelSwitch: control_chl = 12,control_ch2=0, central_chl = 12 DBDCIdx= 0, Band= 0
[   39.261416] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   39.845316] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   39.853812] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   39.862337] MtCmdChannelSwitch: control_chl = 13,control_ch2=0, central_chl = 13 DBDCIdx= 0, Band= 0
[   39.871436] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   40.455312] ====================================================================
[   40.462690] Channel   1 : Busy Time =   7884, Skip Channel = FALSE, BwCap = TRUE
[   40.470062] Channel   2 : Busy Time =   7627, Skip Channel = FALSE, BwCap = TRUE
[   40.477433] Channel   3 : Busy Time =   8034, Skip Channel = FALSE, BwCap = TRUE
[   40.484791] Channel   4 : Busy Time =   2878, Skip Channel = FALSE, BwCap = TRUE
[   40.492159] Channel   5 : Busy Time =   1489, Skip Channel = FALSE, BwCap = TRUE
[   40.499529] Channel   6 : Busy Time =   3186, Skip Channel = FALSE, BwCap = TRUE
[   40.506899] Channel   7 : Busy Time =      6, Skip Channel = FALSE, BwCap = TRUE
[   40.514258] Channel   8 : Busy Time =    147, Skip Channel = FALSE, BwCap = TRUE
[   40.521657] Channel   9 : Busy Time =     92, Skip Channel = FALSE, BwCap = TRUE
[   40.529029] Channel  10 : Busy Time =     92, Skip Channel = FALSE, BwCap = TRUE
[   40.536399] Channel  11 : Busy Time =   3496, Skip Channel = FALSE, BwCap = TRUE
[   40.543757] Channel  12 : Busy Time =     93, Skip Channel = FALSE, BwCap = TRUE
[   40.551130] Channel  13 : Busy Time =   5830, Skip Channel = FALSE, BwCap = TRUE
[   40.558499] ====================================================================
[   40.565881] Rule 3 Channel Busy time value : Select Primary Channel 7
[   40.572288] Rule 3 Channel Busy time value : Min Channel Busy = 6
[   40.578359] Rule 3 Channel Busy time value : BW = 20
[   40.583298] [SelectClearChannelBusyTime] - band0 END
[   40.588254] ApAutoChannelAtBootUp : Auto channel selection: Selected channel = 7, IsAband = 0
[   40.596751] [41m AutoChSelUpdateChannel(): Update channel for wdev for this band PhyMode = 14, Channel = 7 [m
[   40.606800] [1;33m [RadarStateCheck] RD_NORMAL_MODE [m 
[   40.612200] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   40.620690] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   40.629231] MtCmdChannelSwitch: control_chl = 7,control_ch2=0, central_chl = 9 DBDCIdx= 0, Band= 0
[   40.638160] BW = 1,TXStream = 4, RXStream = 4, scan(0)
[   41.544279] [DfsCacNormalStart] Normal start. Enable MAC TX
[   41.563713] ApAutoChannelAtBootUp<-----------------
[   41.568636] [PMF]APPMFInit: IGTK cipher is none, initail IGTK cipher to BIP-CMAC128
[   41.576269] [PMF]APPMFInit:: apidx=0, MFPC=1, MFPR=0, SHA256=0
[   41.582081] [PMF]PMF_MakeRsnIeGMgmtCipher: Insert BIP to the group management cipher of RSNIE
[   41.590625] Enable 20/40 BSSCoex Channel Scan(BssCoex=1)
[   41.596091] MtCmdSetMacTxRx:(ret = 0)
[   41.599746] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   41.608239] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   41.616773] MtCmdChannelSwitch: control_chl = 4,control_ch2=0, central_chl = 4 DBDCIdx= 0, Band= 0
[   41.625699] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   41.813631] AP OBSS SYNC - BBP R4 to 20MHz.l
[   42.415312] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   42.423804] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   42.432302] MtCmdChannelSwitch: control_chl = 5,control_ch2=0, central_chl = 5 DBDCIdx= 0, Band= 0
[   42.441230] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   42.629138] AP OBSS SYNC - BBP R4 to 20MHz.l
[   43.235318] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   43.243812] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   43.252310] MtCmdChannelSwitch: control_chl = 6,control_ch2=0, central_chl = 6 DBDCIdx= 0, Band= 0
[   43.261236] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   43.449157] AP OBSS SYNC - BBP R4 to 20MHz.l
[   44.045333] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   44.053831] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   44.062336] MtCmdChannelSwitch: control_chl = 7,control_ch2=0, central_chl = 7 DBDCIdx= 0, Band= 0
[   44.071263] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   44.259207] AP OBSS SYNC - BBP R4 to 20MHz.l
[   44.855317] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   44.863814] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   44.872313] MtCmdChannelSwitch: control_chl = 8,control_ch2=0, central_chl = 8 DBDCIdx= 0, Band= 0
[   44.881239] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   45.069184] AP OBSS SYNC - BBP R4 to 20MHz.l
[   45.665305] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   45.673795] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   45.682294] MtCmdChannelSwitch: control_chl = 9,control_ch2=0, central_chl = 9 DBDCIdx= 0, Band= 0
[   45.691220] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   45.879185] AP OBSS SYNC - BBP R4 to 20MHz.l
[   46.475305] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   46.483790] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   46.492287] MtCmdChannelSwitch: control_chl = 10,control_ch2=0, central_chl = 10 DBDCIdx= 0, Band= 0
[   46.501385] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   46.689276] AP OBSS SYNC - BBP R4 to 20MHz.l
[   47.285312] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   47.293807] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   47.302307] MtCmdChannelSwitch: control_chl = 11,control_ch2=0, central_chl = 11 DBDCIdx= 0, Band= 0
[   47.311411] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   47.499340] AP OBSS SYNC - BBP R4 to 20MHz.l
[   48.105322] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   48.113823] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   48.122352] MtCmdChannelSwitch: control_chl = 12,control_ch2=0, central_chl = 12 DBDCIdx= 0, Band= 0
[   48.131451] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   48.319387] AP OBSS SYNC - BBP R4 to 20MHz.l
[   48.915310] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   48.923801] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   48.932300] MtCmdChannelSwitch: control_chl = 13,control_ch2=0, central_chl = 13 DBDCIdx= 0, Band= 0
[   48.941399] BW = 0,TXStream = 4, RXStream = 4, scan(1)
[   49.129350] AP OBSS SYNC - BBP R4 to 20MHz.l
[   49.725302] Channel[Idx=3, Ch=4].bEffectedChannel=0x2!
[   49.730419] needFallBack=TRUE due to OP/OT!
[   49.734576] Channel[Idx=4, Ch=5].bEffectedChannel=0x0!
[   49.739701] Channel[Idx=5, Ch=6].bEffectedChannel=0x2!
[   49.744809] needFallBack=TRUE due to OP/OT!
[   49.748979] Channel[Idx=6, Ch=7].bEffectedChannel=0x0!
[   49.754090] Channel[Idx=7, Ch=8].bEffectedChannel=0x0!
[   49.759212] Channel[Idx=8, Ch=9].bEffectedChannel=0x0!
[   49.764323] Channel[Idx=9, Ch=10].bEffectedChannel=0x2!
[   49.769556] needFallBack=TRUE due to OP/OT!
[   49.773717] Channel[Idx=10, Ch=11].bEffectedChannel=0x2!
[   49.779009] needFallBack=TRUE due to OP/OT!
[   49.783170] Channel[Idx=11, Ch=12].bEffectedChannel=0x0!
[   49.788470] Channel[Idx=12, Ch=13].bEffectedChannel=0x2!
[   49.793752] needFallBack=TRUE due to OP/OT!
[   49.797951] mt7615_apply_cal_data() : eeprom 0x52 bit 1 is 0, do runtime cal , skip RX reload
[   49.806445] mt7615_apply_cal_data() : eeprom 0x52 bit 0 is 0, do runtime cal , skip TX reload
[   49.814929] MtCmdChannelSwitch: control_chl = 7,control_ch2=0, central_chl = 9 DBDCIdx= 0, Band= 0
[   49.823855] BW = 1,TXStream = 4, RXStream = 4, scan(0)
[   50.541944] [DfsCacNormalStart] Normal start. Enable MAC TX
[   50.561542] wifi_sys_linkup(), wdev idx = 0
[   50.565760] wtc_acquire_groupkey_wcid: Found a non-occupied wtbl_idx:127 for WDEV_TYPE:1
[   50.565760]  LinkToOmacIdx = 0, LinkToWdevType = 1
[   50.693530] bssUpdateBmcMngRate (BSS_INFO_BROADCAST_INFO), CmdBssInfoBmcRate.u2BcTransmit= 0, CmdBssInfoBmcRate.u2McTransmit = 0
[   50.706212] UpdateBeaconHandler, BCN_UPDATE_INIT, OmacIdx = 0
[   50.712154] MtCmdTxPowerDropCtrl: ucPowerDrop: 100, BandIdx: 0
[   52.167234] apidx 0 for WscUUIDInit
[   52.170729] Generate UUID for apidx(0)
[   52.181919] br-lan: port 2(ra0) entered blocking state
[   52.187247] br-lan: port 2(ra0) entered disabled state
[   52.193020] device ra0 entered promiscuous mode
[   52.198078] br-lan: port 2(ra0) entered blocking state
[   52.203249] br-lan: port 2(ra0) entered forwarding state
[   52.223508] rai0: ===> main_virtual_if_open
[   52.227948] RtmpOSFileOpen(): Error 2 opening /etc/wireless/l1profile.dat
[   52.236302] load l1profile succeed!
[   52.240263] driver_own()::Try to Clear FW Own...
[   52.598052] driver_own()::Success to clear FW Own
[   52.604766] APWdsInitialize():WdsEntry Number=4
[   52.616660] RtmpOSFileOpen(): Error 2 opening /etc/Wireless/iNIC/iNIC_ap_5G.dat
[   52.623954] Open file "/etc/Wireless/iNIC/iNIC_ap_5G.dat" failed!
[   52.631059] E2pAccessMode=2
[   52.634486] Your no. of SSIDs( = 1) does not match your BssidNum( = 0)!
[   52.643447] BandSteering=0
[   52.646317] BndStrgBssIdx=
[   52.649224] [TxPower] BAND0: 100 
[   52.652953] Power Boost (CCK, OFDM): RTMPSetProfileParameters
[   52.652965] [PowerUpCckOfdm] BAND1: (0)-(0)-(0)-(0)-(0)-(0)-(0)
[   52.664785] Power Boost (HT20): RTMPSetProfileParameters
[   52.664986] Power Boost (HT40): RTMPSetProfileParameters
[   52.670481] Power Boost (VHT20): RTMPSetProfileParameters
[   52.675978] Power Boost (VHT40): RTMPSetProfileParameters
[   52.681547] Power Boost (VHT80): RTMPSetProfileParameters
[   52.687128] Power Boost (VHT160): RTMPSetProfileParameters
[   52.693144] [PERCENTAGEenable] BAND0: 0 
[   52.702653] [BFBACKOFFenable] BAND0: 0 
[   52.706630] CalCacheApply = 0 
[   52.710799] FragThreshold[0]=2346
[   52.718560] DfsZeroWait Support=0/0
[   52.722976] DfsZeroWaitCacTime=255/255
[   52.727747] read_etxbf: ETxBfEnCond = 0
[   52.758032] !!! WARNING !!! No matched PhyMode can be found, align to default 0x0
[   52.765493] If/wds0 - PeerPhyOpMode=0
[   52.770101] APWdsInitialize():WdsEntry Number=4
[   52.774604] WDS-Enable mode=0
[   52.777807] AndesSendCmdMsg: Could not send in band command due to diablefRTMP_ADAPTER_MCU_SEND_IN_BAND_CMD
[   52.787517] AndesSendCmdMsg: Command type = ed, Extension command type = 48
[   52.794452] [31mRTMPSetProfileParameters:Disable DfsZeroWait[m
[   52.804339] HT: WDEV[0] Ext Channel = ABOVE
[   52.808844] HT: greenap_cap = 1
[   52.812159] ChipI=7615, Value=0, pcie_aspm in profile=0
[   52.844858] ICapMode = 0
[   52.853940] WtcSetMaxStaNum: BssidNum:0, MaxStaNum:125 (WdsNum:4, ApcliNum:2, MaxNumChipRept:32), MinMcastWcid:126
[   52.864470] Top Init Done!
[   52.867200] Use dev_alloc_skb
[   52.871321] RX[0] DESC ae120000 size = 16384
[   52.877060] RX[1] DESC aeeea000 size = 8192
[   52.881894] cut_through_init(): ct sw token number = 4095
[   52.887838] cut_through_token_list_init(): TokenList inited done!id_head/tail=0/4096
[   52.895573] cut_through_token_list_init(): 8ea65008,8ea65008
[   52.901728] cut_through_token_list_init(): TokenList inited done!id_head/tail=0/4096
[   52.909500] cut_through_token_list_init(): 8ea65018,8ea65018
[   52.915169] Hif Init Done!
[   52.917881] ctl->txq = c1344e6c
[   52.921004] ctl->rxq = c1344e78
[   52.924124] ctl->ackq = c1344e84
[   52.927345] ctl->kickq = c1344e90
[   52.930640] ctl->tx_doneq = c1344e9c
[   52.934193] ctl->rx_doneq = c1344ea8
[   52.937832] Parsing patch header
[   52.941043] 	Built date: 
[   52.941048] 2
[   52.943648] 0
[   52.945211] 1
[   52.946787] 6
[   52.948353] 0
[   52.949916] 3
[   52.951479] 1
[   52.953042] 4
[   52.954606] 1
[   52.956183] 5
[   52.957748] 1
[   52.959311] 8
[   52.960874] 1
[   52.962437] 9
[   52.963999] a
[   52.965578] 
[   52.968618] 
[   52.970095] 	Platform: 
[   52.970098] A
[   52.972525] L
[   52.974088] P
[   52.975665] S
[   52.977231] 
[   52.980268] 	HW/SW version: 
[   52.980271] 0x
[   52.983131] 8a
[   52.984782] 10
[   52.986452] 8a
[   52.988105] 10
[   52.989756] 
[   52.992880] 	Patch version: 
[   52.992883] 0x
[   52.995756] 00
[   52.997407] 00
[   52.999058] 00
[   53.000709] 10
[   53.002358] 
[   53.005504] 	Target address: 0x80000, length: 3120
[   53.010436] patch is not ready && get semaphore success
[   53.015733] EventGenericEventHandler: CMD Success
[   53.020484] MtCmdPatchFinishReq
[   53.023699] EventGenericEventHandler: CMD Success
[   53.028511] release patch semaphore
[   53.031991] WfMcuHwInit: Before NICLoadFirmware, check ICapMode = 0
[   53.038259] Parsing CPU 0 fw tailer
[   53.041731] 	Chip ID: 
[   53.041734] 0x
[   53.044076] 04
[   53.045748] 
[   53.048876] 	Eco version: 
[   53.048878] 0x
[   53.051566] 00
[   53.053217] 
[   53.056354] 	Region number: 
[   53.056358] 0x
[   53.059221] 00
[   53.060870] 
[   53.063994] 	Format version: 
[   53.063997] 0x
[   53.066956] 00
[   53.068609] 
[   53.071733] 	Ram version: 
[   53.071737] _
[   53.074424] r
[   53.076002] e
[   53.077568] s
[   53.079131] e
[   53.080694] r
[   53.082258] v
[   53.083821] e
[   53.085397] d
[   53.086962] _
[   53.088526] 
[   53.091562] 	Built date: 
[   53.091565] 2
[   53.094165] 0
[   53.095740] 1
[   53.097306] 8
[   53.098869] 0
[   53.100431] 4
[   53.101994] 2
[   53.103557] 7
[   53.105120] 1
[   53.106695] 5
[   53.108261] 1
[   53.109823] 4
[   53.111386] 5
[   53.112949] 4
[   53.114513] 
[   53.116089] 
[   53.119129] 	Common crc: 0x0
[   53.121992] Parsing tailer region 0
[   53.125469] 	Feature set: 
[   53.125473] 0x
[   53.128163] 01
[   53.129813] 
[   53.132940] 	Target address: 0x84000, Download size: 409600
[   53.138502] Parsing tailer region 1
[   53.141970] 	Feature set: 
[   53.141973] 0x
[   53.144661] 01
[   53.146329] 
[   53.149459] 	Target address: 0x209c400, Download size: 51072
[   53.155167] EventGenericEventHandler: CMD Success
[   53.164143] EventGenericEventHandler: CMD Success
[   53.169380] MtCmdFwStartReq: override = 0x1, address = 0x84000
[   53.175241] EventGenericEventHandler: CMD Success
[   53.179966] Parsing CPU 1 fw tailer
[   53.183436] 	Chip ID: 
[   53.183441] 0x
[   53.185802] 04
[   53.187455] 
[   53.190578] 	Eco version: 
[   53.190580] 0x
[   53.193268] 00
[   53.194917] 
[   53.198054] 	Region number: 
[   53.198058] 0x
[   53.200919] 00
[   53.202570] 
[   53.205705] 	Format version: 
[   53.205709] 0x
[   53.208658] 00
[   53.210308] 
[   53.213432] 	Ram version: 
[   53.213436] _
[   53.216135] r
[   53.217700] e
[   53.219263] s
[   53.220827] e
[   53.222390] r
[   53.223953] v
[   53.225528] e
[   53.227094] d
[   53.228657] _
[   53.230220] 
[   53.233257] 	Built date: 
[   53.233261] 2
[   53.235876] 0
[   53.237442] 1
[   53.239004] 8
[   53.240567] 0
[   53.242130] 3
[   53.243694] 1
[   53.245257] 5
[   53.246833] 1
[   53.248398] 0
[   53.249961] 4
[   53.251524] 0
[   53.253087] 0
[   53.254649] 0
[   53.256225] 
[   53.257792] 
[   53.260829] 	Common crc: 0x0
[   53.263692] Parsing tailer region 0
[   53.267168] 	Feature set: 
[   53.267173] 0x
[   53.269862] 01
[   53.271511] 
[   53.274637] 	Target address: 0x90000000, Download size: 118976
[   53.280533] EventGenericEventHandler: CMD Success
[   53.286215] MtCmdFwStartReq: override = 0x4, address = 0x0
[   53.343355] EventGenericEventHandler: CMD Success
[   53.396135] MCU Init Done!
[   53.398839] efuse_probe: efuse = 10000212
[   53.402826] RtmpChipOpsEepromHook::e2p_type=2, inf_Type=5
[   53.408222] RtmpEepromGetDefault::e2p_dafault=1
[   53.412730] RtmpChipOpsEepromHook: E2P type(2), E2pAccessMode = 2, E2P default = 1
[   53.420281] NVM is FLASH mode. dev_idx [1] FLASH OFFSET [0x8000]
[   53.431382] [34mNICReadEEPROMParameters: EEPROM 0x52 b300[m