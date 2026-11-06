Xiaomi Mi Router 3 Pro flash layout
Layer0	raw NAND flash memory chip 256 MiB
Layer1	mtd0 All 256MiB
Layer2	mtd1 Bootloader 256 KiB	mtd2 Config 256 KiB	mtd3 Bdata 256 KiB	mtd4 Factory 256 KiB	mtd5 crash 256 KiB	mtd6 crash_syslog 512 KiB	mtd7 reserved0 256 KiB	mtd8 kernel0 4 MiB	mtd9 kernel1 4 MiB	mtd10 rootfs0 40 MiB	mtd11 rootfs1 40 MiB	mtd12 overlay
mountpoint										/
filesystem										OverlayFS
Size in KiB	256 KiB	256 KiB	256 KiB	256 KiB	256 KiB	512 KiB	256 KiB	4 MiB	4 MiB	40 MiB	40 MiB	166 MiB
Name	Bootloader	Config	Bdata	Factory	crash	crash_syslog	reserved0	kernel0	kernel1	rootfs0	rootfs1	Overlay
mountpoint	none	none	none	none	none	none	none	none	none	/rom	/rom	/overlay
filesystem	none	none	none	none	none	none	none	none	none	SquashFS	SquashFS	UBIFS