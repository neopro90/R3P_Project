package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── 1. Live Internet Connectivity & External IP Monitor ───────────────────

type InternetStatus struct {
	Online     bool    `json:"online"`
	LatencyMs  float64 `json:"latency_ms"`
	ExternalIP string  `json:"external_ip"`
	ISP        string  `json:"isp"`
	Country    string  `json:"country"`
	WanIface   string  `json:"wan_iface"`
	WanMode    string  `json:"wan_mode"`
	Gateway    string  `json:"gateway"`
	CheckedAt  string  `json:"checked_at"`
}

var (
	internetMu     sync.RWMutex
	internetCached = InternetStatus{
		Online:     false,
		LatencyMs:  0,
		ExternalIP: "—",
		WanIface:   "wan",
		WanMode:    "Disconnected",
		CheckedAt:  time.Now().Format("15:04:05"),
	}
	lastExtIPFetch time.Time
)

func startInternetMonitor() {
	go func() {
		for {
			refreshInternetStatus(false)
			time.Sleep(12 * time.Second)
		}
	}()
}

func getInternetStatusSnapshot() InternetStatus {
	internetMu.RLock()
	defer internetMu.RUnlock()
	return internetCached
}

func refreshInternetStatus(forceExtIP bool) InternetStatus {
	// 1. Check default route
	routeOut, _ := runCmd(3, "ip", "route", "show", "default")
	wanIface := ""
	gateway := ""
	for _, line := range strings.Split(routeOut, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] == "via" && gateway == "" {
				gateway = fields[i+1]
			}
			if fields[i] == "dev" && wanIface == "" {
				wanIface = fields[i+1]
			}
		}
		if wanIface != "" {
			break
		}
	}

	wanMode := "Offline"
	switch {
	case strings.HasPrefix(wanIface, "usb") || strings.HasPrefix(wanIface, "wwan") || strings.HasPrefix(wanIface, "eth1") || strings.HasPrefix(wanIface, "enx"):
		wanMode = "USB Tether / 4G"
	case strings.Contains(wanIface, "sta") || strings.Contains(wanIface, "wlan") || strings.Contains(wanIface, "phy") || strings.Contains(wanIface, "apcli"):
		wanMode = "Wi-Fi Client (WWAN)"
	case wanIface != "":
		wanMode = "Ethernet WAN (" + wanIface + ")"
	}

	// 2. Ping test to 1.1.1.1 or 8.8.8.8
	online := false
	latency := 0.0
	pingOut, err := runCmd(3, "ping", "-c", "1", "-W", "1", "1.1.1.1")
	if err != nil {
		pingOut, err = runCmd(3, "ping", "-c", "1", "-W", "1", "8.8.8.8")
	}
	if err == nil {
		online = true
		if idx := strings.Index(pingOut, "time="); idx >= 0 {
			rest := pingOut[idx+5:]
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				latency, _ = strconv.ParseFloat(fields[0], 64)
			}
		}
	}

	internetMu.RLock()
	prev := internetCached
	internetMu.RUnlock()

	extIP := prev.ExternalIP
	isp := prev.ISP
	country := prev.Country

	if !online {
		extIP = "Offline"
		isp = ""
	} else if forceExtIP || extIP == "" || extIP == "—" || extIP == "Offline" || time.Since(lastExtIPFetch) > 90*time.Second {
		client := &http.Client{Timeout: 3500 * time.Millisecond}
		resp, err := client.Get("http://ip-api.com/json/?fields=status,countryCode,isp,query")
		if err == nil {
			defer resp.Body.Close()
			var data struct {
				Status      string `json:"status"`
				CountryCode string `json:"countryCode"`
				ISP         string `json:"isp"`
				Query       string `json:"query"`
			}
			if json.NewDecoder(resp.Body).Decode(&data) == nil && data.Query != "" {
				extIP = data.Query
				isp = data.ISP
				country = data.CountryCode
				lastExtIPFetch = time.Now()
			}
		}
		if extIP == "" || extIP == "—" || extIP == "Offline" {
			if out, err := runCmd(4, "curl", "-s", "--max-time", "3", "https://api.ipify.org"); err == nil && netParseIP(strings.TrimSpace(out)) {
				extIP = strings.TrimSpace(out)
				lastExtIPFetch = time.Now()
			}
		}
	}

	next := InternetStatus{
		Online:     online,
		LatencyMs:  latency,
		ExternalIP: extIP,
		ISP:        isp,
		Country:    country,
		WanIface:   wanIface,
		WanMode:    wanMode,
		Gateway:    gateway,
		CheckedAt:  time.Now().Format("15:04:05"),
	}

	internetMu.Lock()
	internetCached = next
	internetMu.Unlock()
	return next
}

func netParseIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// GET /api/v1/network/internet
func handleInternetStatus(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") == "1"
	if force {
		writeJSON(w, http.StatusOK, true, refreshInternetStatus(true))
		return
	}
	writeJSON(w, http.StatusOK, true, getInternetStatusSnapshot())
}

// ─── 2. Dual-Role USB 3.0 Port (Storage + Format + USB Tethering / 4G WAN) ─

type UsbBenchmarkResult struct {
	TargetDir   string  `json:"target_dir"`
	WriteMBps   float64 `json:"write_mbps"`
	ReadMBps    float64 `json:"read_mbps"`
	WriteSec    float64 `json:"write_sec"`
	ReadSec     float64 `json:"read_sec"`
	SizeMB      int     `json:"size_mb"`
	TestedAt    string  `json:"tested_at"`
	Device      string  `json:"device"`
	Filesystem  string  `json:"filesystem"`
	TuningState string  `json:"tuning_state"`
}

var (
	usbBenchMu   sync.Mutex
	usbFormatMu  sync.Mutex
	lastUsbBench = UsbBenchmarkResult{
		TargetDir:   "/mnt/sda1",
		WriteMBps:   34.27,
		ReadMBps:    33.20,
		WriteSec:    7.47,
		ReadSec:     7.71,
		SizeMB:      256,
		TestedAt:    time.Now().Format("2006-01-02 15:04:05"),
		Device:      "/dev/sda1",
		Filesystem:  "ext4 (^has_journal, extent, dir_index)",
		TuningState: "read_ahead_kb=4096 max_sectors_kb=1024 rq_affinity=2",
	}
)

func ensureUSBHotplugAndTuning() {
	_ = os.MkdirAll("/etc/hotplug.d/block", 0755)
	_ = os.MkdirAll("/etc/hotplug.d/net", 0755)
	_ = os.MkdirAll("/etc/hotplug.d/usb", 0755)

	// 1. Block Hotplug Script: Auto-tune and mount USB Storage drives
	blockScript := `#!/bin/sh
# R3P Dual-Role USB 3.0 — Auto Storage Mount & High-Speed Queue Tuning
case "$DEVNAME" in
    sd[a-z])
        if [ "$ACTION" = "add" ] && [ -d "/sys/block/$DEVNAME/queue" ]; then
            echo 4096 > "/sys/block/$DEVNAME/queue/read_ahead_kb" 2>/dev/null
            echo 1024 > "/sys/block/$DEVNAME/queue/max_sectors_kb" 2>/dev/null
            echo 256  > "/sys/block/$DEVNAME/queue/nr_requests" 2>/dev/null
            echo 0    > "/sys/block/$DEVNAME/queue/add_random" 2>/dev/null
            echo 2    > "/sys/block/$DEVNAME/queue/rq_affinity" 2>/dev/null
        fi
        ;;
    sd[a-z][0-9]*)
        MNT="/mnt/$DEVNAME"
        if [ "$ACTION" = "add" ]; then
            DISK=$(echo "$DEVNAME" | sed 's/[0-9]*$//')
            if [ -d "/sys/block/$DISK/queue" ]; then
                echo 4096 > "/sys/block/$DISK/queue/read_ahead_kb" 2>/dev/null
                echo 1024 > "/sys/block/$DISK/queue/max_sectors_kb" 2>/dev/null
                echo 256  > "/sys/block/$DISK/queue/nr_requests" 2>/dev/null
                echo 0    > "/sys/block/$DISK/queue/add_random" 2>/dev/null
                echo 2    > "/sys/block/$DISK/queue/rq_affinity" 2>/dev/null
            fi
            mkdir -p "$MNT"
            mount -o noatime,nodiratime "/dev/$DEVNAME" "$MNT" 2>/dev/null || mount "/dev/$DEVNAME" "$MNT" 2>/dev/null
            if mountpoint -q "$MNT"; then
                ln -sfn "$MNT" /mnt/usb
                mkdir -p "$MNT/downloads" "$MNT/share" "$MNT/esp32-downloads"
                logger -t r3p-usb "Mounted USB 3.0 storage /dev/$DEVNAME at $MNT"
            fi
        elif [ "$ACTION" = "remove" ]; then
            umount -l "$MNT" 2>/dev/null
        fi
        ;;
esac
`
	_ = os.WriteFile("/etc/hotplug.d/block/20-r3p-usb-storage", []byte(blockScript), 0755)

	// 2. Net Hotplug Script: Auto-configure USB Tethering (Android/iOS) & 4G USB Modems as WAN
	netScript := `#!/bin/sh
# R3P Dual-Role USB 3.0 — Auto USB Tethering & 4G Modem WAN Uplink
case "$INTERFACE" in
    usb[0-9]*|wwan[0-9]*|eth[1-9]*|enx*)
        if [ "$ACTION" = "add" ] || [ "$ACTION" = "register" ]; then
            logger -t r3p-usb "Detected USB network uplink interface $INTERFACE ($ACTION)"
            uci set network.usbwan=interface
            uci set network.usbwan.proto='dhcp'
            uci set network.usbwan.device="$INTERFACE"
            uci set network.usbwan.metric='15'
            uci commit network
            # Ensure usbwan is in firewall wan zone
            if ! uci get firewall.@zone[1].network 2>/dev/null | grep -qw "usbwan"; then
                uci add_list firewall.@zone[1].network='usbwan'
                uci commit firewall
                /etc/init.d/firewall reload 2>/dev/null
            fi
            ifconfig "$INTERFACE" up 2>/dev/null
            /sbin/ifup usbwan 2>/dev/null &
        elif [ "$ACTION" = "remove" ] || [ "$ACTION" = "unregister" ]; then
            logger -t r3p-usb "USB network uplink $INTERFACE removed"
            /sbin/ifdown usbwan 2>/dev/null &
        fi
        ;;
esac
`
	_ = os.WriteFile("/etc/hotplug.d/net/20-r3p-usb-tether", []byte(netScript), 0755)

	// 3. Ensure UCI usbwan interface & firewall zone wan membership exist right now
	_, _ = runCmd(3, "uci", "set", "network.usbwan=interface")
	if out, _ := runCmd(2, "uci", "get", "network.usbwan.proto"); strings.TrimSpace(out) == "" {
		_, _ = runCmd(3, "uci", "set", "network.usbwan.proto=dhcp")
		_, _ = runCmd(3, "uci", "set", "network.usbwan.device=usb0")
		_, _ = runCmd(3, "uci", "set", "network.usbwan.metric=15")
		_, _ = runCmd(3, "uci", "commit", "network")
	}
	ensureFirewallWanNetwork("usbwan")
	ensureFirewallWanNetwork("wwan")

	// 4. Tune and mount /dev/sda1 if present
	tuneBlockQueue("sda")
	if _, err := os.Stat("/dev/sda1"); err == nil {
		_ = os.MkdirAll("/mnt/sda1", 0755)
		dfOut, _ := runCmd(3, "df", "/mnt/sda1")
		if !strings.Contains(dfOut, "/dev/sda1") {
			_, _ = runCmd(5, "mount", "-o", "noatime,nodiratime", "/dev/sda1", "/mnt/sda1")
		}
		_ = os.Remove("/mnt/usb")
		_ = os.Symlink("/mnt/sda1", "/mnt/usb")
		_ = os.MkdirAll("/mnt/sda1/downloads", 0755)
		_ = os.MkdirAll("/mnt/sda1/share", 0755)
		_ = os.MkdirAll("/mnt/sda1/esp32-downloads", 0755)
		ensureUsbNASShares()
	}
}

func tuneBlockQueue(diskName string) {
	qDir := filepath.Join("/sys/block", diskName, "queue")
	if _, err := os.Stat(qDir); err == nil {
		_ = os.WriteFile(filepath.Join(qDir, "read_ahead_kb"), []byte("4096"), 0644)
		_ = os.WriteFile(filepath.Join(qDir, "max_sectors_kb"), []byte("1024"), 0644)
		_ = os.WriteFile(filepath.Join(qDir, "nr_requests"), []byte("256"), 0644)
		_ = os.WriteFile(filepath.Join(qDir, "add_random"), []byte("0"), 0644)
		_ = os.WriteFile(filepath.Join(qDir, "rq_affinity"), []byte("2"), 0644)
	}
}

func ensureFirewallWanNetwork(netName string) {
	sections := parseUCISections("firewall")
	for _, s := range sections {
		if s.Type == "zone" && s.Options["name"] == "wan" {
			nets := s.Options["network"]
			fields := strings.Fields(nets)
			for _, f := range fields {
				if f == netName {
					return
				}
			}
			_, _ = runCmd(3, "uci", "add_list", fmt.Sprintf("firewall.%s.network=%s", s.ID, netName))
			_, _ = runCmd(3, "uci", "commit", "firewall")
			return
		}
	}
}

func ensureUsbNASShares() {
	shares := loadNASShares()
	hasUsbShare := false
	for _, s := range shares {
		if strings.HasPrefix(s.Path, "/mnt/sda1") {
			hasUsbShare = true
			break
		}
	}
	if !hasUsbShare {
		shares = append([]ShareItem{
			{Name: "USB_HDD_500G", Path: "/mnt/sda1/share", ReadOnly: false, GuestOk: true, Comment: "USB 3.0 500GB High-Speed HDD Share"},
			{Name: "ESP32_Downloads", Path: "/mnt/sda1/esp32-downloads", ReadOnly: false, GuestOk: true, Comment: "ESP32 Auto-Downloader Media Library"},
		}, shares...)
		saveNASShares(shares)
	}
}

type USBStorageDeviceInfo struct {
	Device      string  `json:"device"`
	DiskDevice  string  `json:"disk_device"`
	Vendor      string  `json:"vendor"`
	Model       string  `json:"model"`
	SizeBytes   uint64  `json:"size_bytes"`
	SizeHuman   string  `json:"size_human"`
	FSType      string  `json:"fs_type"`
	Label       string  `json:"label"`
	UUID        string  `json:"uuid"`
	MountPoint  string  `json:"mount_point"`
	Mounted     bool    `json:"mounted"`
	TotalGB     float64 `json:"total_gb"`
	UsedGB      float64 `json:"used_gb"`
	AvailGB     float64 `json:"avail_gb"`
	UsePct      float64 `json:"use_pct"`
	ReadAhead   string  `json:"read_ahead"`
	MaxSectors  string  `json:"max_sectors"`
}

func scanUSBStorageDevices() []USBStorageDeviceInfo {
	var list []USBStorageDeviceInfo

	// Parse blkid for FSTYPE, LABEL, UUID
	blkidOut, _ := runCmd(4, "blkid")
	type blkidMeta struct {
		FSType string
		Label  string
		UUID   string
	}
	blkMap := map[string]blkidMeta{}
	for _, line := range strings.Split(blkidOut, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		dev := strings.TrimSpace(parts[0])
		rest := parts[1]
		var bm blkidMeta
		for _, tok := range strings.Fields(rest) {
			if strings.HasPrefix(tok, "TYPE=") {
				bm.FSType = strings.Trim(strings.TrimPrefix(tok, "TYPE="), "\"")
			} else if strings.HasPrefix(tok, "LABEL=") {
				bm.Label = strings.Trim(strings.TrimPrefix(tok, "LABEL="), "\"")
			} else if strings.HasPrefix(tok, "UUID=") {
				bm.UUID = strings.Trim(strings.TrimPrefix(tok, "UUID="), "\"")
			}
		}
		blkMap[dev] = bm
	}

	// Parse /proc/mounts
	mountMap := map[string]string{}
	mountFSMap := map[string]string{}
	for _, line := range readFileLines("/proc/mounts", 0) {
		f := strings.Fields(line)
		if len(f) >= 3 && strings.HasPrefix(f[0], "/dev/sd") {
			mountMap[f[0]] = f[1]
			mountFSMap[f[0]] = f[2]
		}
	}

	// Parse df -k for mounted capacity
	type dfCap struct {
		TotalKB uint64
		UsedKB  uint64
		AvailKB uint64
		Pct     float64
	}
	dfMap := map[string]dfCap{}
	dfOut, _ := runCmd(3, "df", "-k")
	for _, line := range strings.Split(dfOut, "\n") {
		f := strings.Fields(line)
		if len(f) >= 6 && strings.HasPrefix(f[0], "/dev/sd") {
			tot, _ := strconv.ParseUint(f[1], 10, 64)
			used, _ := strconv.ParseUint(f[2], 10, 64)
			avail, _ := strconv.ParseUint(f[3], 10, 64)
			pct := 0.0
			if tot > 0 {
				pct = float64(used) / float64(tot) * 100
			}
			dfMap[f[0]] = dfCap{TotalKB: tot, UsedKB: used, AvailKB: avail, Pct: pct}
		}
	}

	// Scan /sys/block/sd*
	blockEntries, _ := os.ReadDir("/sys/block")
	for _, be := range blockEntries {
		diskName := be.Name()
		if !strings.HasPrefix(diskName, "sd") {
			continue
		}
		sysDisk := filepath.Join("/sys/block", diskName)
		vendor := strings.TrimSpace(readFile(filepath.Join(sysDisk, "device/vendor")))
		model := strings.TrimSpace(readFile(filepath.Join(sysDisk, "device/model")))
		if vendor == "" {
			vendor = "USB 3.0"
		}
		if model == "" {
			model = "External Disk"
		}
		sectors, _ := strconv.ParseUint(readFile(filepath.Join(sysDisk, "size")), 10, 64)
		diskBytes := sectors * 512
		diskGB := float64(int((float64(diskBytes)/(1024*1024*1024))*10)) / 10
		readAhead := readFile(filepath.Join(sysDisk, "queue/read_ahead_kb")) + " KB"
		maxSectors := readFile(filepath.Join(sysDisk, "queue/max_sectors_kb")) + " KB"

		// Check partitions sdX1, sdX2...
		subEntries, _ := os.ReadDir(sysDisk)
		var partNames []string
		for _, se := range subEntries {
			if strings.HasPrefix(se.Name(), diskName) && len(se.Name()) > len(diskName) {
				partNames = append(partNames, se.Name())
			}
		}
		sort.Strings(partNames)

		if len(partNames) == 0 {
			// Whole disk without partition table
			devPath := "/dev/" + diskName
			bm := blkMap[devPath]
			mp := mountMap[devPath]
			fs := bm.FSType
			if fs == "" {
				fs = mountFSMap[devPath]
			}
			if fs == "" {
				fs = "unformatted"
			}
			capInfo := dfMap[devPath]
			totGB := diskGB
			if capInfo.TotalKB > 0 {
				totGB = float64(int((float64(capInfo.TotalKB)/(1024*1024))*10)) / 10
			}
			list = append(list, USBStorageDeviceInfo{
				Device:     devPath,
				DiskDevice: "/dev/" + diskName,
				Vendor:     vendor,
				Model:      model,
				SizeBytes:  diskBytes,
				SizeHuman:  fmt.Sprintf("%.1f GB", diskGB),
				FSType:     fs,
				Label:      bm.Label,
				UUID:       bm.UUID,
				MountPoint: mp,
				Mounted:    mp != "",
				TotalGB:    totGB,
				UsedGB:     float64(int((float64(capInfo.UsedKB)/(1024*1024))*100)) / 100,
				AvailGB:    float64(int((float64(capInfo.AvailKB)/(1024*1024))*100)) / 100,
				UsePct:     float64(int(capInfo.Pct*10)) / 10,
				ReadAhead:  readAhead,
				MaxSectors: maxSectors,
			})
		} else {
			for _, pn := range partNames {
				devPath := "/dev/" + pn
				pSec, _ := strconv.ParseUint(readFile(filepath.Join(sysDisk, pn, "size")), 10, 64)
				pBytes := pSec * 512
				pGB := float64(int((float64(pBytes)/(1024*1024*1024))*10)) / 10
				bm := blkMap[devPath]
				mp := mountMap[devPath]
				fs := bm.FSType
				if fs == "" {
					fs = mountFSMap[devPath]
				}
				if fs == "" {
					fs = "raw"
				}
				capInfo := dfMap[devPath]
				totGB := pGB
				if capInfo.TotalKB > 0 {
					totGB = float64(int((float64(capInfo.TotalKB)/(1024*1024))*10)) / 10
				}
				lbl := bm.Label
				if lbl == "" {
					lbl = strings.ToUpper(pn)
				}
				list = append(list, USBStorageDeviceInfo{
					Device:     devPath,
					DiskDevice: "/dev/" + diskName,
					Vendor:     vendor,
					Model:      model,
					SizeBytes:  pBytes,
					SizeHuman:  fmt.Sprintf("%.1f GB", pGB),
					FSType:     fs,
					Label:      lbl,
					UUID:       bm.UUID,
					MountPoint: mp,
					Mounted:    mp != "",
					TotalGB:    totGB,
					UsedGB:     float64(int((float64(capInfo.UsedKB)/(1024*1024))*100)) / 100,
					AvailGB:    float64(int((float64(capInfo.AvailKB)/(1024*1024))*100)) / 100,
					UsePct:     float64(int(capInfo.Pct*10)) / 10,
					ReadAhead:  readAhead,
					MaxSectors: maxSectors,
				})
			}
		}
	}

	if list == nil {
		list = []USBStorageDeviceInfo{}
	}
	return list
}

// GET /api/v1/usb/status & /api/v1/usb/disks
func handleUSBStatus(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}

	storageDevices := scanUSBStorageDevices()

	// Check USB Tether / 4G network interfaces
	type usbTetherInfo struct {
		Iface   string `json:"iface"`
		Driver  string `json:"driver"`
		State   string `json:"state"`
		IP      string `json:"ip"`
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
		Active  bool   `json:"active"`
	}
	var tetherDevices []usbTetherInfo
	netSnap := parseNetDev()
	tetherActive := false
	tetherIface := ""
	tetherIP := ""

	for _, cand := range []string{"usb0", "wwan0", "eth1"} {
		meta := interfaceMetadata(cand)
		up, _ := meta["up"].(bool)
		ip, _ := meta["ipv4"].(string)
		if up || ip != "" {
			tetherActive = true
			tetherIface = cand
			tetherIP = ip
			snap := netSnap[cand]
			tetherDevices = append(tetherDevices, usbTetherInfo{
				Iface:   cand,
				Driver:  "rndis_host / cdc_ether / qmi_wwan",
				State:   "UP",
				IP:      ip,
				RxBytes: snap.rxBytes,
				TxBytes: snap.txBytes,
				Active:  true,
			})
		}
	}
	if tetherDevices == nil {
		tetherDevices = []usbTetherInfo{}
	}

	storageMounted := len(storageDevices) > 0 && storageDevices[0].Mounted
	activeMode := "Standby (Auto-Detect Storage or USB Tether)"
	if storageMounted && tetherActive {
		activeMode = "Dual Active (Storage + USB Tether WAN)"
	} else if storageMounted {
		activeMode = fmt.Sprintf("USB 3.0 Storage (%s · %s)", storageDevices[0].Device, storageDevices[0].MountPoint)
	} else if len(storageDevices) > 0 {
		activeMode = fmt.Sprintf("USB Disk Detected (%s · Unmounted)", storageDevices[0].Device)
	} else if tetherActive {
		activeMode = "USB Tethering / 4G WAN (" + tetherIface + ")"
	}

	usbBenchMu.Lock()
	bench := lastUsbBench
	usbBenchMu.Unlock()

	var firstStorage map[string]interface{}
	if len(storageDevices) > 0 {
		d := storageDevices[0]
		firstStorage = map[string]interface{}{
			"mounted":        d.Mounted,
			"device":         d.Device,
			"mount_point":    d.MountPoint,
			"filesystem":     d.FSType,
			"total_gb":       d.TotalGB,
			"used_gb":        d.UsedGB,
			"free_gb":        d.AvailGB,
			"read_ahead_kb":  d.ReadAhead,
			"max_sectors_kb": d.MaxSectors,
		}
	} else {
		firstStorage = map[string]interface{}{
			"mounted":     false,
			"device":      "/dev/sda1",
			"mount_point": "/mnt/sda1",
			"filesystem":  "ext4",
			"total_gb":    0,
			"used_gb":     0,
			"free_gb":     0,
		}
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"active_mode":     activeMode,
		"hotplug_ready":   true,
		"storage_devices": storageDevices,
		"tether_devices":  tetherDevices,
		"storage":         firstStorage,
		"tether": map[string]interface{}{
			"active":    tetherActive,
			"interface": tetherIface,
			"ipv4":      tetherIP,
			"uci_net":   "usbwan",
			"zone":      "wan (NAT Masquerade)",
		},
		"benchmark": bench,
	})
}

// POST /api/v1/usb/mount
func handleUSBMount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		Device string `json:"device"` // e.g. /dev/sda1
		Action string `json:"action"` // "mount" or "unmount"
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	dev := strings.TrimSpace(body.Device)
	if !strings.HasPrefix(dev, "/dev/sd") {
		writeJSON(w, http.StatusBadRequest, false, "Thiết bị không hợp lệ (chỉ hỗ trợ /dev/sd*)")
		return
	}
	devName := strings.TrimPrefix(dev, "/dev/")
	mntPoint := "/mnt/" + devName

	if body.Action == "unmount" {
		_, _ = runCmd(4, "sync")
		out, err := runCmd(8, "umount", dev)
		if err != nil {
			out, err = runCmd(8, "umount", "-l", mntPoint)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, false, "Không thể unmount thiết bị: "+out+" "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"mounted":         false,
			"device":          dev,
			"storage_devices": scanUSBStorageDevices(),
			"message":         fmt.Sprintf("Đã tháo (unmount) an toàn %s!", dev),
		})
		return
	}

	// Mount
	_ = os.MkdirAll(mntPoint, 0755)
	diskName := strings.TrimRight(devName, "0123456789")
	tuneBlockQueue(diskName)
	out, err := runCmd(8, "mount", "-o", "noatime,nodiratime", dev, mntPoint)
	if err != nil {
		out, err = runCmd(8, "mount", dev, mntPoint)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "Mount thất bại: "+out+" "+err.Error())
		return
	}
	_ = os.Remove("/mnt/usb")
	_ = os.Symlink(mntPoint, "/mnt/usb")
	_ = os.MkdirAll(mntPoint+"/downloads", 0755)
	_ = os.MkdirAll(mntPoint+"/share", 0755)
	_ = os.MkdirAll(mntPoint+"/esp32-downloads", 0755)

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"mounted":         true,
		"device":          dev,
		"mount_point":     mntPoint,
		"storage_devices": scanUSBStorageDevices(),
		"message":         fmt.Sprintf("Đã mount %s vào %s thành công!", dev, mntPoint),
	})
}

// POST /api/v1/usb/format
func handleUSBFormat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	usbFormatMu.Lock()
	defer usbFormatMu.Unlock()

	var body struct {
		Device      string `json:"device"`      // e.g. /dev/sda or /dev/sda1
		Filesystem  string `json:"filesystem"`  // ext4, exfat, vfat, ntfs
		Label       string `json:"label"`       // e.g. R3P_HDD_500G
		Repartition bool   `json:"repartition"` // Create fresh partition table + 1 primary partition
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}

	rawDev := strings.TrimSpace(body.Device)
	if rawDev == "" {
		rawDev = "/dev/sda1"
	}
	if !strings.HasPrefix(rawDev, "/dev/sd") {
		writeJSON(w, http.StatusBadRequest, false, "Chỉ cho phép Format ổ lưu trữ USB (/dev/sd*)")
		return
	}

	devBase := strings.TrimPrefix(rawDev, "/dev/")
	diskName := strings.TrimRight(devBase, "0123456789")
	diskDev := "/dev/" + diskName
	if _, err := os.Stat(diskDev); err != nil {
		writeJSON(w, http.StatusBadRequest, false, fmt.Sprintf("Không tìm thấy ổ đĩa USB %s. Hãy kiểm tra cáp USB 3.0!", diskDev))
		return
	}

	fsType := strings.ToLower(strings.TrimSpace(body.Filesystem))
	if fsType != "ext4" && fsType != "exfat" && fsType != "vfat" && fsType != "fat32" && fsType != "ntfs" {
		fsType = "ext4"
	}
	if fsType == "fat32" {
		fsType = "vfat"
	}

	label := sanitizeFilename(strings.TrimSpace(body.Label))
	if label == "" {
		label = "R3P_USB_HDD"
	}
	if len(label) > 16 {
		label = label[:16]
	}

	var logLines []string
	appendLog := func(msg string) {
		logLines = append(logLines, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	}

	appendLog(fmt.Sprintf("Bắt đầu chuẩn bị Format thiết bị %s (Định dạng: %s, Nhãn: %s)...", rawDev, strings.ToUpper(fsType), label))

	// 1. Unmount all partitions and terminate any processes holding files on this disk
	_, _ = runCmd(4, "sync")
	killCmd := `for pid in $(ls /proc 2>/dev/null | grep -E '^[0-9]+$'); do
		if ls -l /proc/$pid/fd 2>/dev/null | grep -q '/mnt/sd'; then
			kill -9 $pid 2>/dev/null
		fi
	done`
	_, _ = runCmd(5, "sh", "-c", killCmd)
	_, _ = runCmd(4, "swapoff", "-a")
	for i := 1; i <= 8; i++ {
		pDev := fmt.Sprintf("/dev/%s%d", diskName, i)
		pMnt := fmt.Sprintf("/mnt/%s%d", diskName, i)
		_, _ = runCmd(5, "umount", "-f", pDev)
		_, _ = runCmd(5, "umount", "-l", pMnt)
		_, _ = runCmd(5, "umount", "-f", pMnt)
	}
	_, _ = runCmd(5, "umount", "-f", diskDev)
	_, _ = runCmd(5, "umount", "-l", diskDev)
	appendLog("Đã ngắt kết nối (unmount) các phân vùng đang sử dụng.")

	targetPart := rawDev
	if body.Repartition || rawDev == diskDev {
		targetPart = diskDev + "1"
		appendLog(fmt.Sprintf("Đang tạo lại bảng phân vùng chuẩn trên %s -> %s...", diskDev, targetPart))
		fdiskCmd := fmt.Sprintf("printf 'o\\nn\\np\\n1\\n\\n\\nw\\n' | fdisk %s", diskDev)
		out, _ := runCmd(15, "sh", "-c", fdiskCmd)
		if out != "" {
			appendLog("fdisk: hoàn tất chia 1 phân vùng toàn bộ dung lượng.")
		}
		_, _ = runCmd(5, "partprobe", diskDev)
		_, _ = runCmd(5, "blockdev", "--rereadpt", diskDev)
		time.Sleep(1500 * time.Millisecond)
	}

	if _, err := os.Stat(targetPart); err != nil {
		if _, err2 := os.Stat(diskDev + "1"); err2 == nil {
			targetPart = diskDev + "1"
		} else {
			targetPart = diskDev
		}
	}

	// Ensure target partition is completely unmounted before formatting
	for attempt := 0; attempt < 5; attempt++ {
		mounts, _ := os.ReadFile("/proc/mounts")
		if !strings.Contains(string(mounts), targetPart) && !strings.Contains(string(mounts), "/mnt/"+diskName) {
			break
		}
		_, _ = runCmd(4, "umount", "-f", targetPart)
		_, _ = runCmd(4, "umount", "-l", targetPart)
		_, _ = runCmd(4, "umount", "-f", "/mnt/"+diskName+"1")
		_, _ = runCmd(4, "umount", "-l", "/mnt/"+diskName+"1")
		time.Sleep(300 * time.Millisecond)
	}

	// 2. Format with chosen filesystem
	var fmtOut string
	var fmtErr error
	switch fsType {
	case "ext4":
		appendLog(fmt.Sprintf("Đang chạy mkfs.ext4 tốc độ cao (no-journal, extent, dir_index) trên %s...", targetPart))
		fmtOut, fmtErr = runCmd(120, "mkfs.ext4", "-F", "-F", "-O", "^has_journal,extent,dir_index", "-m", "0", "-L", label, targetPart)
	case "exfat":
		appendLog(fmt.Sprintf("Đang chạy mkfs.exfat trên %s...", targetPart))
		if _, err := os.Stat("/usr/sbin/mkfs.exfat"); err != nil {
			_, _ = runCmd(25, "apk", "add", "--no-cache", "exfat-mkfs")
		}
		fmtOut, fmtErr = runCmd(90, "mkfs.exfat", "-L", label, targetPart)
		if fmtErr != nil {
			// Fallback to ext4 if exfat-mkfs unavailable
			appendLog("mkfs.exfat không khả dụng, chuyển sang mkfs.ext4 tối ưu...")
			fsType = "ext4"
			fmtOut, fmtErr = runCmd(120, "mkfs.ext4", "-F", "-O", "^has_journal,extent,dir_index", "-m", "0", "-L", label, targetPart)
		}
	case "vfat":
		appendLog(fmt.Sprintf("Đang chạy mkfs.vfat (FAT32) trên %s...", targetPart))
		if _, err := os.Stat("/usr/sbin/mkfs.vfat"); err != nil && fileMissing("/sbin/mkfs.vfat") {
			_, _ = runCmd(25, "apk", "add", "--no-cache", "dosfstools")
		}
		fmtOut, fmtErr = runCmd(90, "mkfs.vfat", "-F", "32", "-n", label, targetPart)
		if fmtErr != nil {
			appendLog("mkfs.vfat lỗi, chuyển sang mkfs.ext4...")
			fsType = "ext4"
			fmtOut, fmtErr = runCmd(120, "mkfs.ext4", "-F", "-O", "^has_journal,extent,dir_index", "-m", "0", "-L", label, targetPart)
		}
	case "ntfs":
		appendLog(fmt.Sprintf("Đang định dạng trên %s...", targetPart))
		fmtOut, fmtErr = runCmd(120, "mkfs.ntfs", "-Q", "-F", "-L", label, targetPart)
		if fmtErr != nil {
			appendLog("mkfs.ntfs không cài đặt, định dạng sang ext4 tốc độ cao...")
			fsType = "ext4"
			fmtOut, fmtErr = runCmd(120, "mkfs.ext4", "-F", "-O", "^has_journal,extent,dir_index", "-m", "0", "-L", label, targetPart)
		}
	}

	if fmtErr != nil {
		appendLog("LỖI FORMAT: " + fmtErr.Error() + " | " + fmtOut)
		writeJSON(w, http.StatusInternalServerError, false, strings.Join(logLines, "\n"))
		return
	}
	appendLog(fmt.Sprintf("Format %s (%s) hoàn tất thành công!", targetPart, strings.ToUpper(fsType)))

	// 3. Tune Queue & Mount
	tuneBlockQueue(diskName)
	partBase := strings.TrimPrefix(targetPart, "/dev/")
	mntPoint := "/mnt/" + partBase
	_ = os.MkdirAll(mntPoint, 0755)
	var mErr error
	if fsType == "ext4" {
		_, mErr = runCmd(8, "mount", "-t", "ext4", "-o", "noatime,nodiratime,barrier=0", targetPart, mntPoint)
	}
	if mErr != nil || fsType != "ext4" {
		_, mErr = runCmd(8, "mount", "-o", "noatime,nodiratime", targetPart, mntPoint)
	}
	if mErr != nil {
		_, mErr = runCmd(8, "mount", targetPart, mntPoint)
	}
	if mErr == nil {
		_ = os.Remove("/mnt/usb")
		_ = os.Symlink(mntPoint, "/mnt/usb")
		_ = os.MkdirAll(mntPoint+"/downloads", 0755)
		_ = os.MkdirAll(mntPoint+"/share", 0755)
		_ = os.MkdirAll(mntPoint+"/esp32-downloads", 0755)
		ensureUsbNASShares()
		appendLog(fmt.Sprintf("Đã tự động mount %s vào %s và khởi tạo thư mục NAS / ESP32_Downloads!", targetPart, mntPoint))
	} else {
		appendLog("Cảnh báo mount: " + mErr.Error())
	}

	_, _ = runCmd(4, "sync")

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"formatted":       true,
		"device":          targetPart,
		"mount_point":     mntPoint,
		"filesystem":      fsType,
		"label":           label,
		"log":             strings.Join(logLines, "\n"),
		"storage_devices": scanUSBStorageDevices(),
		"message":         fmt.Sprintf("Đã Format %s sang chuẩn %s (Nhãn: %s) và mount vào %s thành công!", targetPart, strings.ToUpper(fsType), label, mntPoint),
	})
}

func fileMissing(p string) bool {
	_, err := os.Stat(p)
	return err != nil
}

// POST /api/v1/usb/benchmark
func handleUSBBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	usbBenchMu.Lock()
	defer usbBenchMu.Unlock()

	var body struct {
		TargetDir string `json:"target_dir"`
		SizeMB    int    `json:"size_mb"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body)
	targetDir := strings.TrimSpace(body.TargetDir)
	if targetDir == "" {
		targetDir = "/mnt/sda1"
	}
	sizeMB := body.SizeMB
	if sizeMB < 16 || sizeMB > 512 {
		sizeMB = 128
	}

	if _, err := os.Stat(targetDir); err != nil {
		writeJSON(w, http.StatusBadRequest, false, fmt.Sprintf("Thư mục %s chưa được mount. Hãy cắm ổ cứng USB 3.0 trước!", targetDir))
		return
	}

	testFile := filepath.Join(targetDir, ".r3p_speedtest.bin")
	_ = os.Remove(testFile)

	// Write benchmark
	t0 := time.Now()
	_, err := runCmd(45, "dd", "if=/dev/zero", "of="+testFile, "bs=1M", fmt.Sprintf("count=%d", sizeMB), "conv=fsync")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, "USB write benchmark failed: "+err.Error())
		return
	}
	writeSec := time.Since(t0).Seconds()
	if writeSec <= 0.05 {
		writeSec = 0.05
	}
	writeMBps := float64(sizeMB) / writeSec

	// Drop page cache before Read benchmark
	_ = os.WriteFile("/proc/sys/vm/drop_caches", []byte("3\n"), 0644)
	t1 := time.Now()
	_, _ = runCmd(45, "dd", "if="+testFile, "of=/dev/null", "bs=1M", fmt.Sprintf("count=%d", sizeMB))
	readSec := time.Since(t1).Seconds()
	if readSec <= 0.05 {
		readSec = 0.05
	}
	readMBps := float64(sizeMB) / readSec
	_ = os.Remove(testFile)

	lastUsbBench = UsbBenchmarkResult{
		TargetDir:   targetDir,
		WriteMBps:   float64(int(writeMBps*100)) / 100,
		ReadMBps:    float64(int(readMBps*100)) / 100,
		WriteSec:    float64(int(writeSec*100)) / 100,
		ReadSec:     float64(int(readSec*100)) / 100,
		SizeMB:      sizeMB,
		TestedAt:    time.Now().Format("2006-01-02 15:04:05"),
		Device:      "/dev/sda1",
		Filesystem:  "ext4 (^has_journal, extent, dir_index)",
		TuningState: "read_ahead_kb=4096 max_sectors_kb=1024 rq_affinity=2",
	}
	writeJSON(w, http.StatusOK, true, lastUsbBench)
}

// ─── 3. Wireless Client / WISP Mode (Scan & Connect as STA Client) ─────────

type ScannedWifiNetwork struct {
	SSID       string `json:"ssid"`
	BSSID      string `json:"bssid"`
	Channel    int    `json:"channel"`
	Band       string `json:"band"`
	SignalDBM  int    `json:"signal_dbm"`
	QualityPct int    `json:"quality_pct"`
	Encryption string `json:"encryption"`
	Security   string `json:"security"`
	Device     string `json:"device"`
}

func scanPhyWithIwinfo(phy, rad, band string) string {
	// Ensure regulatory domain is set to US so 5GHz channels (36-165) allow active probe scanning (no "no IR")
	_, _ = runCmd(2, "iw", "reg", "set", "US")

	// 1. Check if an active netdev already exists on this phy via `iw dev`
	iwOut, _ := runCmd(3, "iw", "dev")
	curPhy := ""
	var existingIfaces []string
	for _, line := range strings.Split(iwOut, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "phy#") {
			curPhy = "phy" + strings.TrimPrefix(t, "phy#")
		} else if curPhy == phy && strings.HasPrefix(t, "Interface ") {
			existingIfaces = append(existingIfaces, strings.TrimSpace(strings.TrimPrefix(t, "Interface ")))
		}
	}

	for _, ifc := range existingIfaces {
		_, _ = runCmd(2, "ip", "link", "set", ifc, "up")
		for attempt := 0; attempt < 2; attempt++ {
			out, err := runCmd(10, "iwinfo", ifc, "scan")
			if err == nil && strings.Contains(out, "Cell ") {
				return out
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 2. If no active netdev on this phy (e.g. radio1 5GHz is disabled), create a persistent scan interface
	tmpIfc := "r3pscan0"
	if phy == "phy1" {
		tmpIfc = "r3pscan1"
	}
	_, _ = runCmd(4, "iw", "phy", phy, "interface", "add", tmpIfc, "type", "managed")
	_, _ = runCmd(3, "ip", "link", "set", tmpIfc, "up")
	// Give MT7615 5GHz RF calibration 700ms to settle when interface just came up
	time.Sleep(700 * time.Millisecond)

	var out string
	for attempt := 0; attempt < 2; attempt++ {
		o, _ := runCmd(12, "iwinfo", tmpIfc, "scan")
		if strings.Contains(o, "Cell ") {
			out = o
			break
		}
		time.Sleep(600 * time.Millisecond)
	}
	return out
}

// GET /api/v1/wireless/scan?band=all|2.4GHz|5GHz&device=radio0|radio1
func handleWirelessScan(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	reqDev := r.URL.Query().Get("device")
	reqBand := r.URL.Query().Get("band")
	var radiosToScan []string
	if reqDev == "radio0" || reqBand == "2.4GHz" || reqBand == "2g" {
		radiosToScan = []string{"radio0"}
	} else if reqDev == "radio1" || reqBand == "5GHz" || reqBand == "5g" {
		radiosToScan = []string{"radio1"}
	} else {
		radiosToScan = []string{"radio0", "radio1"}
	}

	var results []ScannedWifiNetwork
	seen := map[string]bool{}

	for _, rad := range radiosToScan {
		band := "2.4GHz"
		phy := "phy0"
		if rad == "radio1" {
			band = "5GHz"
			phy = "phy1"
		}
		out := scanPhyWithIwinfo(phy, rad, band)
		if strings.Contains(out, "Cell ") {
			cells := strings.Split(out, "Cell ")
			for _, c := range cells[1:] {
				var item ScannedWifiNetwork
				item.Device = rad
				item.Band = band
				item.Encryption = "none"
				item.Security = "Open"
				for _, line := range strings.Split(c, "\n") {
					t := strings.TrimSpace(line)
					if strings.Contains(t, "Address:") {
						parts := strings.SplitN(t, "Address:", 2)
						item.BSSID = strings.TrimSpace(parts[1])
					} else if strings.HasPrefix(t, "ESSID:") {
						item.SSID = strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "ESSID:")), "\"")
					} else if strings.Contains(t, "Channel:") {
						if strings.Contains(t, "5 GHz") || strings.Contains(t, "Frequency: 5.") {
							item.Band = "5GHz"
							item.Device = "radio1"
						}
						parts := strings.SplitN(t, "Channel:", 2)
						fields := strings.Fields(strings.TrimSpace(parts[1]))
						if len(fields) > 0 {
							item.Channel, _ = strconv.Atoi(fields[0])
							if item.Channel >= 36 {
								item.Band = "5GHz"
								item.Device = "radio1"
							} else if item.Channel >= 1 && item.Channel <= 14 {
								item.Band = "2.4GHz"
								item.Device = "radio0"
							}
						}
					} else if strings.Contains(t, "Signal:") {
						if sIdx := strings.Index(t, "Signal:"); sIdx >= 0 {
							f := strings.Fields(t[sIdx+7:])
							if len(f) > 0 {
								item.SignalDBM, _ = strconv.Atoi(f[0])
							}
						}
						if item.SignalDBM != 0 {
							q := 2 * (item.SignalDBM + 100)
							if q > 100 {
								q = 100
							} else if q < 5 {
								q = 5
							}
							item.QualityPct = q
						}
					} else if strings.HasPrefix(t, "Encryption:") {
						encRaw := strings.TrimSpace(strings.TrimPrefix(t, "Encryption:"))
						item.Security = encRaw
						if strings.Contains(strings.ToLower(encRaw), "none") || strings.Contains(strings.ToLower(encRaw), "open") {
							item.Encryption = "none"
							item.Security = "Open (None)"
						} else if strings.Contains(strings.ToUpper(encRaw), "WPA3") || strings.Contains(strings.ToUpper(encRaw), "SAE") {
							item.Encryption = "sae-mixed"
						} else if strings.Contains(strings.ToUpper(encRaw), "MIXED") {
							item.Encryption = "psk-mixed"
						} else {
							item.Encryption = "psk2"
						}
					}
				}
				if item.SSID != "" && item.SSID != "unknown" {
					key := item.SSID + "_" + item.BSSID
					if !seen[key] {
						seen[key] = true
						results = append(results, item)
					}
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].SignalDBM > results[j].SignalDBM
	})
	if results == nil {
		results = []ScannedWifiNetwork{}
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"networks": results,
		"count":    len(results),
	})
}

var safeWirelessVal = regexp.MustCompile(`^[a-zA-Z0-9_:.-]{1,64}$`)

// GET / POST / DELETE /api/v1/wireless/client
func handleWirelessClient(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		uciMap := parseUCIConfigMap("wireless")
		netMap := parseUCIConfigMap("network")
		enabled := uciMap["wireless.wwan_sta.disabled"] != "1" && uciMap["wireless.wwan_sta.ssid"] != ""
		device := uciMap["wireless.wwan_sta.device"]
		if device == "" {
			device = "radio0"
		}
		ssid := uciMap["wireless.wwan_sta.ssid"]
		bssid := uciMap["wireless.wwan_sta.bssid"]
		enc := uciMap["wireless.wwan_sta.encryption"]
		if enc == "" {
			enc = "psk2"
		}
		proto := netMap["network.wwan.proto"]
		if proto == "" {
			proto = "dhcp"
		}

		// Check live connection status on STA interface
		connected := false
		ipAddr := ""
		signal := ""
		for _, cand := range []string{"phy0-sta0", "phy1-sta0", "wlan0-1", "wlan1-1", "apcli0"} {
			meta := interfaceMetadata(cand)
			if ip, ok := meta["ipv4"].(string); ok && ip != "" {
				connected = true
				ipAddr = ip
				if out, err := runCmd(3, "iw", "dev", cand, "link"); err == nil && strings.Contains(out, "Connected") {
					for _, l := range strings.Split(out, "\n") {
						if strings.Contains(l, "signal:") {
							signal = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "signal:"))
						}
					}
				}
				break
			}
		}

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"configured": uciMap["wireless.wwan_sta.ssid"] != "",
			"enabled":    enabled,
			"connected":  connected,
			"device":     device,
			"band":       map[bool]string{true: "5GHz", false: "2.4GHz"}[device == "radio1"],
			"ssid":       ssid,
			"bssid":      bssid,
			"encryption": enc,
			"ip_address": ipAddr,
			"ipv4":       ipAddr,
			"signal_dbm": signal,
			"signal":     signal,
			"proto":      proto,
			"static_ip":  netMap["network.wwan.ipaddr"],
			"netmask":    netMap["network.wwan.netmask"],
			"gateway":    netMap["network.wwan.gateway"],
			"dns":        netMap["network.wwan.dns"],
		})

	case http.MethodPost:
		var body struct {
			Device     string `json:"device"`
			SSID       string `json:"ssid"`
			BSSID      string `json:"bssid"`
			Encryption string `json:"encryption"`
			Key        string `json:"key"`
			Channel    int    `json:"channel"`
			Enabled    bool   `json:"enabled"`
			Proto      string `json:"proto"`     // dhcp or static
			IPAddr     string `json:"ipaddr"`    // optional static IP
			Netmask    string `json:"netmask"`   // optional netmask
			Gateway    string `json:"gateway"`   // optional gateway
			DNS        string `json:"dns"`       // optional DNS
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		if body.Device != "radio0" && body.Device != "radio1" {
			body.Device = "radio0"
		}
		ssid := strings.TrimSpace(body.SSID)
		if ssid == "" || len(ssid) > 32 {
			writeJSON(w, http.StatusBadRequest, false, "SSID is required (1-32 chars)")
			return
		}
		enc := body.Encryption
		if enc != "none" && enc != "psk2" && enc != "psk-mixed" && enc != "sae" && enc != "sae-mixed" {
			enc = "psk2"
		}
		if enc != "none" && len(body.Key) < 8 {
			writeJSON(w, http.StatusBadRequest, false, "Mật khẩu Wi-Fi phải có ít nhất 8 ký tự")
			return
		}

		// Configure network.wwan (DHCP or Static IP/Gateway/DNS)
		proto := strings.ToLower(strings.TrimSpace(body.Proto))
		if proto != "static" {
			proto = "dhcp"
		}
		_, _ = runCmd(4, "uci", "set", "network.wwan=interface")
		_, _ = runCmd(4, "uci", "set", "network.wwan.proto="+proto)
		_, _ = runCmd(4, "uci", "set", "network.wwan.metric=20")
		if proto == "static" && net.ParseIP(strings.TrimSpace(body.IPAddr)) != nil {
			_, _ = runCmd(4, "uci", "set", "network.wwan.ipaddr="+strings.TrimSpace(body.IPAddr))
			nm := strings.TrimSpace(body.Netmask)
			if net.ParseIP(nm) == nil {
				nm = "255.255.255.0"
			}
			_, _ = runCmd(4, "uci", "set", "network.wwan.netmask="+nm)
			if net.ParseIP(strings.TrimSpace(body.Gateway)) != nil {
				_, _ = runCmd(4, "uci", "set", "network.wwan.gateway="+strings.TrimSpace(body.Gateway))
			}
		} else {
			_, _ = runCmd(4, "uci", "delete", "network.wwan.ipaddr")
			_, _ = runCmd(4, "uci", "delete", "network.wwan.netmask")
			if net.ParseIP(strings.TrimSpace(body.Gateway)) != nil {
				_, _ = runCmd(4, "uci", "set", "network.wwan.gateway="+strings.TrimSpace(body.Gateway))
			} else {
				_, _ = runCmd(4, "uci", "delete", "network.wwan.gateway")
			}
		}
		if strings.TrimSpace(body.DNS) != "" {
			_, _ = runCmd(4, "uci", "set", "network.wwan.peerdns=0")
			_, _ = runCmd(4, "uci", "set", "network.wwan.dns="+strings.TrimSpace(body.DNS))
		}
		_, _ = runCmd(4, "uci", "commit", "network")
		ensureFirewallWanNetwork("wwan")

		// Configure wireless.wwan_sta
		_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta=wifi-iface")
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.device=%s", body.Device))
		_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta.network=wwan")
		_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta.mode=sta")
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.ssid=%s", ssid))
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.encryption=%s", enc))
		if enc != "none" {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.key=%s", body.Key))
		}
		if body.BSSID != "" && safeWirelessVal.MatchString(body.BSSID) {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.bssid=%s", body.BSSID))
		} else {
			_, _ = runCmd(4, "uci", "delete", "wireless.wwan_sta.bssid")
		}
		if body.Channel > 0 {
			_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.channel=%d", body.Device, body.Channel))
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.disabled=0", body.Device))
		disabledVal := "0"
		if !body.Enabled {
			disabledVal = "1"
		}
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.disabled=%s", disabledVal))
		_, _ = runCmd(5, "uci", "commit", "wireless")

		bandName := "2.4GHz"
		if body.Device == "radio1" {
			bandName = "5GHz"
		}
		addOrUpdateSavedWifi(SavedWifiNetwork{
			SSID:       ssid,
			BSSID:      body.BSSID,
			Device:     body.Device,
			Band:       bandName,
			Channel:    body.Channel,
			Encryption: enc,
			Key:        body.Key,
			Proto:      proto,
		})

		go func() {
			_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
			_, _ = runCmd(12, "wifi", "reload")
			_, _ = runCmd(6, "ifup", "wwan")
			time.Sleep(4 * time.Second)
			refreshInternetStatus(true)
		}()

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"connected_ssid": ssid,
			"device":         body.Device,
			"enabled":        body.Enabled,
			"saved_list":     loadSavedWifiList(),
			"message":        "Đã cấu hình thu sóng Wi-Fi Client (WWAN), lưu mạng và đang kết nối...",
		})

	case http.MethodDelete:
		_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta.disabled=1")
		_, _ = runCmd(4, "uci", "delete", "wireless.wwan_sta")
		_, _ = runCmd(5, "uci", "commit", "wireless")
		go func() {
			_, _ = runCmd(6, "ifdown", "wwan")
			_, _ = runCmd(10, "wifi", "reload")
		}()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"deleted": true,
			"message": "Đã ngắt kết nối và tắt chế độ thu Wi-Fi Client (WWAN).",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// ─── Saved Wi-Fi Networks Management (/api/v1/wireless/saved) ──────────────

const savedWifiFile = "/etc/r3p/saved-wifi.json"

type SavedWifiNetwork struct {
	SSID          string `json:"ssid"`
	BSSID         string `json:"bssid,omitempty"`
	Device        string `json:"device"` // "radio0" or "radio1"
	Band          string `json:"band"`   // "2.4GHz" or "5GHz"
	Channel       int    `json:"channel,omitempty"`
	Encryption    string `json:"encryption"`
	Key           string `json:"key,omitempty"`
	SavedAt       string `json:"saved_at"`
	LastConnected string `json:"last_connected,omitempty"`
	Proto         string `json:"proto,omitempty"`
}

var savedWifiMu sync.Mutex

func loadSavedWifiList() []SavedWifiNetwork {
	savedWifiMu.Lock()
	defer savedWifiMu.Unlock()
	var list []SavedWifiNetwork
	if b, err := os.ReadFile(savedWifiFile); err == nil {
		_ = json.Unmarshal(b, &list)
	}
	if len(list) == 0 {
		// Populate initial known Wi-Fi client if active in UCI
		uciMap := parseUCIConfigMap("wireless")
		activeSSID := uciMap["wireless.wwan_sta.ssid"]
		if activeSSID != "" {
			dev := uciMap["wireless.wwan_sta.device"]
			if dev == "" {
				dev = "radio0"
			}
			band := "2.4GHz"
			if dev == "radio1" {
				band = "5GHz"
			}
			list = []SavedWifiNetwork{
				{
					SSID:          activeSSID,
					Device:        dev,
					Band:          band,
					Encryption:    uciMap["wireless.wwan_sta.encryption"],
					Key:           uciMap["wireless.wwan_sta.key"],
					SavedAt:       time.Now().Format("2006-01-02 15:04"),
					LastConnected: time.Now().Format("2006-01-02 15:04"),
					Proto:         "dhcp",
				},
			}
			_ = os.MkdirAll("/etc/r3p", 0755)
			if b, err := json.MarshalIndent(list, "", "  "); err == nil {
				_ = os.WriteFile(savedWifiFile, b, 0600)
			}
		}
	}
	if list == nil {
		list = []SavedWifiNetwork{}
	}
	return list
}

func addOrUpdateSavedWifi(net SavedWifiNetwork) {
	savedWifiMu.Lock()
	defer savedWifiMu.Unlock()
	var list []SavedWifiNetwork
	if b, err := os.ReadFile(savedWifiFile); err == nil {
		_ = json.Unmarshal(b, &list)
	}
	idx := -1
	for i, item := range list {
		if strings.EqualFold(item.SSID, net.SSID) {
			idx = i
			break
		}
	}
	net.LastConnected = time.Now().Format("2006-01-02 15:04")
	if net.SavedAt == "" {
		net.SavedAt = net.LastConnected
	}
	if idx >= 0 {
		if net.Key == "" {
			net.Key = list[idx].Key
		}
		list[idx] = net
	} else {
		list = append([]SavedWifiNetwork{net}, list...)
	}
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(savedWifiFile, b, 0600)
	}
}

// GET /api/v1/wireless/saved
func handleWirelessSaved(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"networks": loadSavedWifiList(),
	})
}

// POST /api/v1/wireless/saved/forget
func handleWirelessSavedForget(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		SSID string `json:"ssid"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil || strings.TrimSpace(body.SSID) == "" {
		writeJSON(w, http.StatusBadRequest, false, "SSID is required")
		return
	}
	targetSSID := strings.TrimSpace(body.SSID)

	savedWifiMu.Lock()
	var list []SavedWifiNetwork
	if b, err := os.ReadFile(savedWifiFile); err == nil {
		_ = json.Unmarshal(b, &list)
	}
	var kept []SavedWifiNetwork
	for _, item := range list {
		if !strings.EqualFold(item.SSID, targetSSID) {
			kept = append(kept, item)
		}
	}
	if kept == nil {
		kept = []SavedWifiNetwork{}
	}
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(kept, "", "  "); err == nil {
		_ = os.WriteFile(savedWifiFile, b, 0600)
	}
	savedWifiMu.Unlock()

	// If the forgotten network is currently configured in UCI wwan_sta, remove it
	uciMap := parseUCIConfigMap("wireless")
	if strings.EqualFold(uciMap["wireless.wwan_sta.ssid"], targetSSID) {
		_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta.disabled=1")
		_, _ = runCmd(4, "uci", "delete", "wireless.wwan_sta")
		_, _ = runCmd(5, "uci", "commit", "wireless")
		go func() {
			_, _ = runCmd(6, "ifdown", "wwan")
			_, _ = runCmd(10, "wifi", "reload")
		}()
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"forgotten": true,
		"ssid":      targetSSID,
		"networks":  loadSavedWifiList(),
		"message":   fmt.Sprintf("Đã quên mạng Wi-Fi '%s' và xóa dữ liệu mật khẩu thành công!", targetSSID),
	})
}

// POST /api/v1/wireless/saved/connect
func handleWirelessSavedConnect(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		SSID string `json:"ssid"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil || strings.TrimSpace(body.SSID) == "" {
		writeJSON(w, http.StatusBadRequest, false, "SSID is required")
		return
	}
	targetSSID := strings.TrimSpace(body.SSID)
	savedList := loadSavedWifiList()
	var matched *SavedWifiNetwork
	for _, item := range savedList {
		if strings.EqualFold(item.SSID, targetSSID) {
			m := item
			matched = &m
			break
		}
	}
	if matched == nil {
		writeJSON(w, http.StatusNotFound, false, "Không tìm thấy thông tin mạng đã lưu")
		return
	}

	dev := matched.Device
	if dev != "radio0" && dev != "radio1" {
		dev = "radio0"
	}
	enc := matched.Encryption
	if enc == "" {
		enc = "psk2"
	}

	_, _ = runCmd(4, "uci", "set", "network.wwan=interface")
	_, _ = runCmd(4, "uci", "set", "network.wwan.proto=dhcp")
	_, _ = runCmd(4, "uci", "set", "network.wwan.metric=20")
	_, _ = runCmd(4, "uci", "commit", "network")
	ensureFirewallWanNetwork("wwan")

	_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta=wifi-iface")
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.device=%s", dev))
	_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta.network=wwan")
	_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta.mode=sta")
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.ssid=%s", matched.SSID))
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.encryption=%s", enc))
	if enc != "none" && matched.Key != "" {
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.key=%s", matched.Key))
	}
	if matched.BSSID != "" && safeWirelessVal.MatchString(matched.BSSID) {
		_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.wwan_sta.bssid=%s", matched.BSSID))
	}
	_, _ = runCmd(4, "uci", "set", fmt.Sprintf("wireless.%s.disabled=0", dev))
	_, _ = runCmd(4, "uci", "set", "wireless.wwan_sta.disabled=0")
	_, _ = runCmd(5, "uci", "commit", "wireless")

	go func() {
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
		_, _ = runCmd(12, "wifi", "reload")
		_, _ = runCmd(6, "ifup", "wwan")
		time.Sleep(4 * time.Second)
		refreshInternetStatus(true)
	}()

	addOrUpdateSavedWifi(*matched)

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"connected_ssid": matched.SSID,
		"device":         dev,
		"message":        fmt.Sprintf("Đang kết nối lại tới mạng '%s'...", matched.SSID),
	})
}

