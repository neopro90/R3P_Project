package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Full-Featured R3P DNS AdBlock Engine (dnsmasq + fw4 Force DNS) ────────

type AdblockSource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Category    string `json:"category"` // Ads, Trackers, Malware, Telemetry, Custom
	Enabled     bool   `json:"enabled"`
	DomainCount int    `json:"domain_count"`
	LastUpdated string `json:"last_updated"`
	Status      string `json:"status"`
}

type AdblockConfig struct {
	Enabled           bool            `json:"enabled"`
	ForceDNS          bool            `json:"force_dns"`           // Redirect LAN port 53 to router DNS
	ExposeUpstreamDNS bool            `json:"expose_upstream_dns"` // Allow private RFC1918 WAN subnets to query DNS port 53
	SafeSearch        bool            `json:"safe_search"`         // Enforce Google/YouTube/Bing SafeSearch
	BlockMode         string          `json:"block_mode"`          // "zero_ip" (0.0.0.0) or "nxdomain"
	AutoUpdate        bool            `json:"auto_update"`         // Background auto-refresh
	UpdateSchedule    string          `json:"update_schedule"`     // e.g. "0 3 * * *"
	QueryLogging      bool            `json:"query_logging"`       // Enable dnsmasq log-queries
	UpstreamDNS       []string        `json:"upstream_dns"`        // e.g. ["1.1.1.1", "8.8.8.8"]
	Sources           []AdblockSource `json:"sources"`
	Whitelist         []string        `json:"whitelist"`
	Blacklist         []string        `json:"blacklist"`
	TotalBlocked      int             `json:"total_blocked"`
	LastRebuild       string          `json:"last_rebuild"`
}

const (
	adblockConfigFile = "/etc/r3p/adblock.json"
	adblockHostsFile  = "/tmp/r3p-adblock.hosts"
	adblockPersistDir = "/overlay/r3p-adblock"
	adblockDnsmasqCfg = "/tmp/dnsmasq.d/r3p-adblock.conf"
	maxAdlistBytes    = 8 * 1024 * 1024
)

var (
	adblockMu       sync.Mutex
	adblockCfgMu    sync.Mutex
	adblockUpdating bool
)

var builtinAdDomains = []string{
	"doubleclick.net", "googleadservices.com", "googlesyndication.com", "pagead2.googlesyndication.com",
	"adservice.google.com", "analytics.google.com", "ssl.google-analytics.com", "www.google-analytics.com",
	"ads.youtube.com", "ad.doubleclick.net", "static.doubleclick.net", "m.doubleclick.net",
	"mediavisor.doubleclick.net", "afs.googlesyndication.com", "tpc.googlesyndication.com",
	"an.facebook.com", "pixel.facebook.com", "graph.facebook.com", "connect.facebook.net",
	"ads.tiktok.com", "analytics.tiktok.com",
	"ads-api.twitter.com", "ads.twitter.com", "analytics.twitter.com", "static.ads-twitter.com",
	"ad.yahoo.com", "ads.yahoo.com", "analytics.query.yahoo.com", "gemini.yahoo.com",
	"aax.amazon-adsystem.com", "c.amazon-adsystem.com", "fls-na.amazon-adsystem.com", "s.amazon-adsystem.com",
	"ads.pubmatic.com", "ib.adnxs.com", "secure.adnxs.com", "fastlane.rubiconproject.com",
	"ads.rubiconproject.com", "bid.g.doubleclick.net", "cm.g.doubleclick.net", "stats.g.doubleclick.net",
	"criteo.com", "dis.criteo.com", "gum.criteo.com", "static.criteo.net",
	"outbrain.com", "widgets.outbrain.com", "taboola.com", "trc.taboola.com", "cdn.taboola.com",
	"scorecardresearch.com", "sb.scorecardresearch.com", "b.scorecardresearch.com",
	"quantserve.com", "pixel.quantserve.com", "edge.quantserve.com",
	"adform.net", "track.adform.net", "serving-sys.com", "bs.serving-sys.com",
	"moatads.com", "z.moatads.com", "px.moatads.com", "adsrvr.org", "match.adsrvr.org",
	"demdex.net", "dpm.demdex.net", "omtrdc.net", "2o7.net", "everesttech.net",
	"hotjar.com", "static.hotjar.com", "script.hotjar.com", "vars.hotjar.com",
	"mouseflow.com", "cdn.mouseflow.com", "luckyorange.com", "w1.luckyorange.com",
	"fullstory.com", "rs.fullstory.com", "mixpanel.com", "api.mixpanel.com", "cdn.mxpnl.com",
	"amplitude.com", "api.amplitude.com", "segment.io", "api.segment.io", "cdn.segment.com",
	"braze.com", "sdk.iad-01.braze.com", "appsflyer.com", "t.appsflyer.com", "launches.appsflyer.com",
	"adjust.com", "app.adjust.com", "branch.io", "api.branch.io", " api2.branch.io",
	"unityads.unity3d.com", "auction.unityads.unity3d.com", "config.unityads.unity3d.com",
	"applovin.com", "rt.applovin.com", "d.applovin.com", "ms.applovin.com",
	"vungle.com", "ads.api.vungle.com", "chartboost.com", "live.chartboost.com",
	"ironsrc.com", "init.supersonicads.com", "inmobi.com", "config.inmobi.com",
	"vortex.data.microsoft.com", "settings-win.data.microsoft.com", "telemetry.microsoft.com",
	"watson.telemetry.microsoft.com", "oca.telemetry.microsoft.com", "sqm.telemetry.microsoft.com",
	"rad.msn.com", "ads1.msn.com", "ads.msn.com", "acdn.adnxs.com",
	"tracking.miui.com", "data.mistat.xiaomi.com", "sdkconfig.ad.xiaomi.com", "api.ad.xiaomi.com",
	"log.tbs.qq.com", "adsmind.gdtimg.com", "mi.gdt.qq.com", "hm.baidu.com", "pos.baidu.com",
}

func isDomainWhitelisted(d string, whiteList []string) bool {
	d = strings.ToLower(strings.TrimSpace(d))
	for _, w := range whiteList {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		if d == w || strings.HasSuffix(d, "."+w) {
			return true
		}
	}
	return false
}

func defaultAdblockConfig() AdblockConfig {
	return AdblockConfig{
		Enabled:        true,
		ForceDNS:       true,
		SafeSearch:     false,
		BlockMode:      "zero_ip",
		AutoUpdate:     true,
		UpdateSchedule: "0 3 * * *",
		QueryLogging:   false,
		UpstreamDNS:    []string{"1.1.1.1", "8.8.8.8"},
		Sources: []AdblockSource{
			{
				ID:          "src-stevenblack",
				Name:        "StevenBlack Unified Hosts (Ads + Malware)",
				URL:         "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
				Category:    "Ads & Malware",
				Enabled:     true,
				DomainCount: 0,
				LastUpdated: "Chưa tải",
				Status:      "Ready",
			},
			{
				ID:          "src-adguard",
				Name:        "AdGuard DNS Filter (Web & Mobile Ads)",
				URL:         "https://v.firebog.net/hosts/AdguardDNS.txt",
				Category:    "Ads",
				Enabled:     true,
				DomainCount: 0,
				LastUpdated: "Chưa tải",
				Status:      "Ready",
			},
			{
				ID:          "src-peterlowe",
				Name:        "Peter Lowe's Ad & Tracking Server List",
				URL:         "https://pgl.yoyo.org/adservers/serverlist.php?hostformat=hosts&showintro=0&mimetype=plaintext",
				Category:    "Ads & Trackers",
				Enabled:     true,
				DomainCount: 0,
				LastUpdated: "Chưa tải",
				Status:      "Ready",
			},
			{
				ID:          "src-easyprivacy",
				Name:        "EasyPrivacy Anti-Tracking Filter",
				URL:         "https://v.firebog.net/hosts/Easyprivacy.txt",
				Category:    "Trackers",
				Enabled:     false,
				DomainCount: 0,
				LastUpdated: "Chưa tải",
				Status:      "Disabled",
			},
			{
				ID:          "src-urlhaus",
				Name:        "URLHaus Malware & Phishing Blocklist",
				URL:         "https://urlhaus.abuse.ch/downloads/hostfile/",
				Category:    "Malware",
				Enabled:     true,
				DomainCount: 0,
				LastUpdated: "Chưa tải",
				Status:      "Ready",
			},
			{
				ID:          "src-winspy",
				Name:        "Windows & Xiaomi Telemetry Blocker",
				URL:         "https://raw.githubusercontent.com/crazy-max/WindowsSpyBlocker/master/data/hosts/spy.txt",
				Category:    "Telemetry",
				Enabled:     true,
				DomainCount: 0,
				LastUpdated: "Chưa tải",
				Status:      "Ready",
			},
		},
		Whitelist: []string{
			"github.com",
			"raw.githubusercontent.com",
			"openwrt.org",
			"downloads.openwrt.org",
		},
		Blacklist: []string{
			"doubleclick.net",
			"googleadservices.com",
		},
		TotalBlocked: len(builtinAdDomains),
		LastRebuild:  time.Now().Format("2006-01-02 15:04:05"),
	}
}

func loadAdblockConfig() AdblockConfig {
	adblockCfgMu.Lock()
	defer adblockCfgMu.Unlock()
	cfg := defaultAdblockConfig()
	if b, err := os.ReadFile(adblockConfigFile); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	if cfg.BlockMode != "zero_ip" && cfg.BlockMode != "nxdomain" {
		cfg.BlockMode = "zero_ip"
	}
	if len(cfg.UpstreamDNS) == 0 {
		cfg.UpstreamDNS = []string{"1.1.1.1", "8.8.8.8"}
	}
	if cfg.Whitelist == nil {
		cfg.Whitelist = []string{}
	}
	if cfg.Blacklist == nil {
		cfg.Blacklist = []string{}
	}
	if cfg.Sources == nil {
		cfg.Sources = defaultAdblockConfig().Sources
	}
	return cfg
}

func saveAdblockConfig(cfg AdblockConfig) {
	adblockCfgMu.Lock()
	defer adblockCfgMu.Unlock()
	_ = os.MkdirAll("/etc/r3p", 0755)
	if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		_ = os.WriteFile(adblockConfigFile, b, 0644)
	}
}

func normalizeDomain(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "#"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	// If hosts format: "0.0.0.0 domain.com" or "127.0.0.1 domain.com"
	fields := strings.Fields(s)
	if len(fields) >= 2 && (fields[0] == "0.0.0.0" || fields[0] == "127.0.0.1") {
		s = fields[1]
	} else if len(fields) == 1 {
		s = fields[0]
	} else {
		return ""
	}
	// Strip AdGuard ||domain.com^ syntax
	if strings.HasPrefix(s, "||") && strings.HasSuffix(s, "^") {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "||"), "^")
	}
	if s == "" || s == "localhost" || s == "localhost.localdomain" || s == "local" || s == "0.0.0.0" || s == "127.0.0.1" || s == "::1" {
		return ""
	}
	if !strings.Contains(s, ".") || strings.ContainsAny(s, " *:/\\?&$'\"<>|") {
		return ""
	}
	return s
}

func applyAdblockRuntime(cfg *AdblockConfig, downloadRemote bool) (int, error) {
	adblockMu.Lock()
	adblockUpdating = true
	defer func() {
		adblockUpdating = false
		adblockMu.Unlock()
	}()

	_ = os.MkdirAll("/tmp/dnsmasq.d", 0755)
	_ = os.MkdirAll(adblockPersistDir, 0755)

	// 1. Configure Firewall Force-DNS redirect rule
	if cfg.Enabled && cfg.ForceDNS {
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns=redirect")
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns.name=R3P-AdBlock-Force-DNS")
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns.src=lan")
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns.proto=tcp udp")
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns.src_dport=53")
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns.dest_port=53")
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns.target=DNAT")
		_, _ = runCmd(4, "uci", "set", "firewall.adblock_dns.enabled=1")
		_, _ = runCmd(4, "uci", "commit", "firewall")
	} else {
		_, _ = runCmd(4, "uci", "delete", "firewall.adblock_dns")
		_, _ = runCmd(4, "uci", "commit", "firewall")
	}

	if len(strings.Fields(strings.TrimSpace(cfg.UpdateSchedule))) != 5 {
		cfg.UpdateSchedule = "0 3 * * *"
	}

	// 2. Remove legacy unauthenticated curl cron entries if present (auto-update runs in-process)
	entries := loadRootCrontab()
	var filteredCron []CronEntry
	cronChanged := false
	for _, e := range entries {
		if strings.Contains(e.Command, "adblock/update") || strings.Contains(e.Description, "AdBlock") {
			cronChanged = true
			continue
		}
		filteredCron = append(filteredCron, e)
	}
	if cronChanged {
		saveRootCrontab(filteredCron)
	}

	if !cfg.Enabled {
		_ = os.Remove(adblockHostsFile)
		_ = os.Remove(adblockDnsmasqCfg)
		_, _ = runCmd(4, "uci", "delete", "firewall.r3p_adblock_wan_dns")
		_, _ = runCmd(4, "uci", "commit", "firewall")
		_, _ = runCmd(5, "uci", "delete", "dhcp.@dnsmasq[0].addnhosts")
		_, _ = runCmd(4, "uci", "set", "dhcp.@dnsmasq[0].localservice=1")
		_, _ = runCmd(5, "uci", "commit", "dhcp")
		_, _ = runCmd(8, "/etc/init.d/dnsmasq", "restart")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
		saveAdblockConfig(*cfg)
		return 0, nil
	}

	// 3. Build Whitelist Lookup Map
	whiteMap := map[string]bool{}
	for _, w := range cfg.Whitelist {
		if d := normalizeDomain(w); d != "" {
			whiteMap[d] = true
		}
	}

	// 4. Build Blocked Domains Set
	domainSet := map[string]bool{}
	for _, d := range builtinAdDomains {
		nd := normalizeDomain(d)
		if nd != "" && !whiteMap[nd] && !isDomainWhitelisted(nd, cfg.Whitelist) {
			domainSet[nd] = true
		}
	}
	for _, d := range cfg.Blacklist {
		nd := normalizeDomain(d)
		if nd != "" && !whiteMap[nd] && !isDomainWhitelisted(nd, cfg.Whitelist) {
			domainSet[nd] = true
		}
	}

	// Load cached persistent domains if not downloading fresh right now
	persistHosts := adblockPersistDir + "/cached.hosts"
	if !downloadRemote {
		if f, err := os.Open(persistHosts); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if nd := normalizeDomain(sc.Text()); nd != "" && !whiteMap[nd] && !isDomainWhitelisted(nd, cfg.Whitelist) {
					domainSet[nd] = true
				}
			}
			f.Close()
		}
	} else {
		client := &http.Client{Timeout: 15 * time.Second}
		for i := range cfg.Sources {
			src := &cfg.Sources[i]
			if !src.Enabled {
				src.Status = "Disabled"
				continue
			}
			domains, err := fetchAdblockSourceDomains(client, src.URL)
			if err != nil {
				src.Status = "Offline (" + err.Error() + ")"
				continue
			}
			for _, nd := range domains {
				if !whiteMap[nd] && !isDomainWhitelisted(nd, cfg.Whitelist) && len(domainSet) < 85000 {
					domainSet[nd] = true
				}
			}
			src.DomainCount = len(domains)
			src.LastUpdated = time.Now().Format("2006-01-02 15:04")
			src.Status = fmt.Sprintf("Active (%d domains)", len(domains))
		}
	}

	// Sort and write /tmp/r3p-adblock.hosts
	allDomains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		if !whiteMap[d] && !isDomainWhitelisted(d, cfg.Whitelist) {
			allDomains = append(allDomains, d)
		}
	}
	sort.Strings(allDomains)

	var sb strings.Builder
	sb.WriteString("# R3P Home Server OS — Generated AdBlock Hosts\n")
	for _, d := range allDomains {
		sb.WriteString("0.0.0.0 ")
		sb.WriteString(d)
		sb.WriteByte('\n')
	}
	hostsBytes := []byte(sb.String())
	_ = os.WriteFile(adblockHostsFile, hostsBytes, 0644)
	if downloadRemote || len(allDomains) > 0 {
		_ = os.WriteFile(persistHosts, hostsBytes, 0644)
	}

	// Write single authoritative dnsmasq config in /tmp/dnsmasq.d (no duplicate addn-hosts loading)
	_ = os.MkdirAll("/tmp/dnsmasq.d", 0755)
	var dconf strings.Builder
	dconf.WriteString("# R3P AdBlock Dnsmasq Configuration & Wildcards\n")
	for _, d := range allDomains {
		if cfg.BlockMode == "nxdomain" {
			dconf.WriteString("server=/" + d + "/\n")
		} else {
			dconf.WriteString("address=/" + d + "/0.0.0.0\n")
			dconf.WriteString("address=/" + d + "/::\n")
		}
	}
	if cfg.SafeSearch {
		dconf.WriteString("cname=www.google.com,forcesafesearch.google.com\n")
		dconf.WriteString("address=/forcesafesearch.google.com/216.239.38.120\n")
		dconf.WriteString("address=/restrict.youtube.com/216.239.38.120\n")
		dconf.WriteString("address=/strict.bing.com/204.79.197.220\n")
	}
	confBytes := []byte(dconf.String())
	_ = os.WriteFile(adblockDnsmasqCfg, confBytes, 0644)

	_, _ = runCmd(4, "uci", "set", "dhcp.@dnsmasq[0].confdir=/tmp/dnsmasq.d")
	_, _ = runCmd(4, "uci", "delete", "dhcp.@dnsmasq[0].addnhosts")
	if cfg.ExposeUpstreamDNS {
		_, _ = runCmd(4, "uci", "set", "dhcp.@dnsmasq[0].localservice=0")
		_, _ = runCmd(4, "uci", "set", "dhcp.@dnsmasq[0].nonwildcard=0")
	} else {
		_, _ = runCmd(4, "uci", "set", "dhcp.@dnsmasq[0].localservice=1")
	}
	if cfg.QueryLogging {
		_, _ = runCmd(4, "uci", "set", "dhcp.@dnsmasq[0].logqueries=1")
	} else {
		_, _ = runCmd(4, "uci", "delete", "dhcp.@dnsmasq[0].logqueries")
	}
	if len(cfg.UpstreamDNS) > 0 {
		_, _ = runCmd(4, "uci", "delete", "dhcp.@dnsmasq[0].server")
		for _, srv := range cfg.UpstreamDNS {
			if netParseIP(strings.TrimSpace(srv)) {
				_, _ = runCmd(4, "uci", "add_list", "dhcp.@dnsmasq[0].server="+strings.TrimSpace(srv))
			}
		}
	}
	_, _ = runCmd(5, "uci", "commit", "dhcp")

	// Only allow Port 53 from private RFC1918 WAN subnets when explicitly opted in (never 0.0.0.0/0 public WAN)
	if cfg.ExposeUpstreamDNS {
		_, _ = runCmd(3, "uci", "set", "firewall.r3p_adblock_wan_dns=rule")
		_, _ = runCmd(3, "uci", "set", "firewall.r3p_adblock_wan_dns.name=Allow-R3P-AdBlock-Private-DNS")
		_, _ = runCmd(3, "uci", "set", "firewall.r3p_adblock_wan_dns.src=wan")
		_, _ = runCmd(3, "uci", "delete", "firewall.r3p_adblock_wan_dns.src_ip")
		for _, cidr := range []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"} {
			_, _ = runCmd(3, "uci", "add_list", "firewall.r3p_adblock_wan_dns.src_ip="+cidr)
		}
		_, _ = runCmd(3, "uci", "set", "firewall.r3p_adblock_wan_dns.dest_port=53")
		_, _ = runCmd(3, "uci", "set", "firewall.r3p_adblock_wan_dns.proto=tcp udp")
		_, _ = runCmd(3, "uci", "set", "firewall.r3p_adblock_wan_dns.target=ACCEPT")
	} else {
		_, _ = runCmd(3, "uci", "delete", "firewall.r3p_adblock_wan_dns")
	}
	_, _ = runCmd(4, "uci", "commit", "firewall")

	cfg.TotalBlocked = len(allDomains)
	cfg.LastRebuild = time.Now().Format("2006-01-02 15:04:05")
	saveAdblockConfig(*cfg)

	go func() {
		_, _ = runCmd(8, "/etc/init.d/dnsmasq", "reload")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
	}()

	return len(allDomains), nil
}

func fetchAdblockSourceDomains(client *http.Client, rawURL string) ([]string, error) {
	u := strings.TrimSpace(rawURL)
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return nil, fmt.Errorf("invalid url")
	}
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("fetch error")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var domains []string
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxAdlistBytes))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if nd := normalizeDomain(line); nd != "" {
			domains = append(domains, nd)
		}
	}
	return domains, nil
}

func initAdblockOnStartup() {
	go func() {
		time.Sleep(2 * time.Second)
		cfg := loadAdblockConfig()
		if cfg.Enabled {
			_, _ = applyAdblockRuntime(&cfg, false)
		}
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			c := loadAdblockConfig()
			if c.Enabled && c.AutoUpdate {
				_, _ = applyAdblockRuntime(&c, true)
			}
		}
	}()
}

// GET /api/v1/adblock
func handleAdblockGet(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	cfg := loadAdblockConfig()

	// Check if dnsmasq is running & how many lines are in /tmp/r3p-adblock.hosts
	activeLines := 0
	if f, err := os.Open(adblockHostsFile); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "0.0.0.0 ") {
				activeLines++
			}
		}
		f.Close()
	}
	if activeLines > 0 {
		cfg.TotalBlocked = activeLines
	}

	// Read recent DNS query / blocked logs from logread
	logOut, _ := runCmd(4, "logread", "-e", "dnsmasq")
	var recentLogs []string
	blockedHits := 0
	for _, line := range strings.Split(logOut, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.Contains(t, "0.0.0.0") || strings.Contains(t, adblockHostsFile) {
			blockedHits++
		}
		if strings.Contains(t, "query[") || strings.Contains(t, "0.0.0.0") || strings.Contains(t, "forwarded") || strings.Contains(t, "cached") {
			recentLogs = append(recentLogs, t)
		}
	}
	if len(recentLogs) > 80 {
		recentLogs = recentLogs[len(recentLogs)-80:]
	}
	// Reverse so newest is first
	for i, j := 0, len(recentLogs)-1; i < j; i, j = i+1, j-1 {
		recentLogs[i], recentLogs[j] = recentLogs[j], recentLogs[i]
	}
	if recentLogs == nil {
		recentLogs = []string{}
	}

	// Detect R3P Client IP & Upstream Gateway IP so the UI can guide setting DNS on the main Gateway
	r3pLanIP := getRouterLanIP()
	if lanMeta := interfaceMetadata("br-lan"); lanMeta != nil {
		if ip, ok := lanMeta["ipv4"].(string); ok && ip != "" {
			r3pLanIP = ip
		}
	}
	upstreamGW := ""
	clientIface := ""
	clientIP := ""
	rtOut, _ := runCmd(3, "ip", "-4", "route")
	for _, line := range strings.Split(rtOut, "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[0] == "default" && f[1] == "via" {
			upstreamGW = f[2]
			for i := 3; i < len(f)-1; i++ {
				if f[i] == "dev" {
					clientIface = f[i+1]
				} else if f[i] == "src" {
					clientIP = f[i+1]
				}
			}
			break
		}
	}
	if clientIP == "" && clientIface != "" {
		if m := interfaceMetadata(clientIface); m != nil {
			if ip, ok := m["ipv4"].(string); ok {
				clientIP = ip
			}
		}
	}

	gatewaySetup := map[string]interface{}{
		"is_client_mode":      clientIP != "" || upstreamGW != "",
		"r3p_client_ip":       clientIP,
		"r3p_client_iface":    clientIface,
		"upstream_gateway_ip": upstreamGW,
		"r3p_lan_ip":          r3pLanIP,
		"wan_dns_listening":   cfg.Enabled && cfg.ExposeUpstreamDNS,
	}

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"config":        cfg,
		"updating":      adblockUpdating,
		"active_rules":  activeLines,
		"blocked_hits":  blockedHits,
		"recent_logs":   recentLogs,
		"hosts_file":    adblockHostsFile,
		"gateway_setup": gatewaySetup,
	})
}

// PUT /api/v1/adblock/config
func handleAdblockConfigPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		Enabled           *bool    `json:"enabled"`
		ForceDNS          *bool    `json:"force_dns"`
		ExposeUpstreamDNS *bool    `json:"expose_upstream_dns"`
		SafeSearch        *bool    `json:"safe_search"`
		BlockMode         string   `json:"block_mode"`
		AutoUpdate        *bool    `json:"auto_update"`
		UpdateSchedule    string   `json:"update_schedule"`
		QueryLogging      *bool    `json:"query_logging"`
		UpstreamDNS       []string `json:"upstream_dns"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}

	cfg := loadAdblockConfig()
	if body.Enabled != nil {
		cfg.Enabled = *body.Enabled
	}
	if body.ForceDNS != nil {
		cfg.ForceDNS = *body.ForceDNS
	}
	if body.ExposeUpstreamDNS != nil {
		cfg.ExposeUpstreamDNS = *body.ExposeUpstreamDNS
	}
	if body.SafeSearch != nil {
		cfg.SafeSearch = *body.SafeSearch
	}
	if body.BlockMode == "zero_ip" || body.BlockMode == "nxdomain" {
		cfg.BlockMode = body.BlockMode
	}
	if body.AutoUpdate != nil {
		cfg.AutoUpdate = *body.AutoUpdate
	}
	if body.UpdateSchedule != "" {
		cfg.UpdateSchedule = body.UpdateSchedule
	}
	if body.QueryLogging != nil {
		cfg.QueryLogging = *body.QueryLogging
	}
	if len(body.UpstreamDNS) > 0 {
		cfg.UpstreamDNS = body.UpstreamDNS
	}

	total, _ := applyAdblockRuntime(&cfg, false)
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"saved":         true,
		"total_blocked": total,
		"config":        cfg,
		"message":       "Đã lưu và áp dụng cấu hình AdBlock thành công!",
	})
}

// POST /api/v1/adblock/update
func handleAdblockUpdateNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	cfg := loadAdblockConfig()
	cfg.Enabled = true
	total, err := applyAdblockRuntime(&cfg, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, false, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"updated":       true,
		"total_blocked": total,
		"config":        cfg,
		"message":       fmt.Sprintf("Đã cập nhật danh sách chặn AdBlock thành công (%d tên miền đang bị chặn)!", total),
	})
}

// POST / PATCH / DELETE /api/v1/adblock/lists
func handleAdblockLists(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	cfg := loadAdblockConfig()

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name     string `json:"name"`
			URL      string `json:"url"`
			Category string `json:"category"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		name := strings.TrimSpace(body.Name)
		u := strings.TrimSpace(body.URL)
		if name == "" || (!strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://")) {
			writeJSON(w, http.StatusBadRequest, false, "Vui lòng nhập Tên danh sách và URL hợp lệ (http/https)")
			return
		}
		cat := strings.TrimSpace(body.Category)
		if cat == "" {
			cat = "Custom"
		}
		cfg.Sources = append(cfg.Sources, AdblockSource{
			ID:          "src-" + generateGID()[:6],
			Name:        name,
			URL:         u,
			Category:    cat,
			Enabled:     true,
			DomainCount: 0,
			LastUpdated: "Chưa tải",
			Status:      "Ready",
		})
		saveAdblockConfig(cfg)
		writeJSON(w, http.StatusCreated, true, map[string]interface{}{
			"config":  cfg,
			"message": "Đã thêm nguồn danh sách chặn AdBlock mới!",
		})

	case http.MethodPatch:
		var body struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		for i := range cfg.Sources {
			if cfg.Sources[i].ID == body.ID {
				cfg.Sources[i].Enabled = body.Enabled
				if !body.Enabled {
					cfg.Sources[i].Status = "Disabled"
				} else {
					cfg.Sources[i].Status = "Ready"
				}
			}
		}
		saveAdblockConfig(cfg)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"config": cfg})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		var updated []AdblockSource
		for _, s := range cfg.Sources {
			if s.ID != id {
				updated = append(updated, s)
			}
		}
		cfg.Sources = updated
		saveAdblockConfig(cfg)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"config": cfg})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST / DELETE /api/v1/adblock/domains
func handleAdblockDomains(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	cfg := loadAdblockConfig()

	switch r.Method {
	case http.MethodPost:
		var body struct {
			ListType string `json:"list_type"` // "whitelist" or "blacklist"
			Domain   string `json:"domain"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		// Allow adding multiple domains separated by newline, comma, or space
		rawParts := strings.FieldsFunc(body.Domain, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t'
		})
		added := 0
		for _, p := range rawParts {
			d := normalizeDomain(p)
			if d == "" {
				continue
			}
			if body.ListType == "whitelist" {
				exists := false
				for _, x := range cfg.Whitelist {
					if x == d {
						exists = true
						break
					}
				}
				if !exists {
					cfg.Whitelist = append(cfg.Whitelist, d)
					added++
				}
			} else {
				exists := false
				for _, x := range cfg.Blacklist {
					if x == d {
						exists = true
						break
					}
				}
				if !exists {
					cfg.Blacklist = append(cfg.Blacklist, d)
					added++
				}
			}
		}
		if added == 0 {
			writeJSON(w, http.StatusBadRequest, false, "Tên miền không hợp lệ hoặc đã tồn tại trong danh sách")
			return
		}
		total, _ := applyAdblockRuntime(&cfg, false)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"config":        cfg,
			"total_blocked": total,
			"message":       fmt.Sprintf("Đã thêm %d tên miền vào %s và nạp lại DNS!", added, body.ListType),
		})

	case http.MethodDelete:
		listType := r.URL.Query().Get("list_type")
		domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
		if listType == "whitelist" {
			var next []string
			for _, x := range cfg.Whitelist {
				if x != domain {
					next = append(next, x)
				}
			}
			cfg.Whitelist = next
		} else {
			var next []string
			for _, x := range cfg.Blacklist {
				if x != domain {
					next = append(next, x)
				}
			}
			cfg.Blacklist = next
		}
		total, _ := applyAdblockRuntime(&cfg, false)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"config":        cfg,
			"total_blocked": total,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

// POST /api/v1/adblock/test
func handleAdblockTestDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
		return
	}
	if !requireOpenWrt(w) {
		return
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
		return
	}
	d := normalizeDomain(body.Domain)
	if d == "" {
		writeJSON(w, http.StatusBadRequest, false, "Tên miền cần kiểm tra không hợp lệ")
		return
	}

	// Check in /tmp/r3p-adblock.hosts
	inHosts := false
	if f, err := os.Open(adblockHostsFile); err == nil {
		target := "0.0.0.0 " + d
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == target {
				inHosts = true
				break
			}
		}
		f.Close()
	}

	out, _ := runCmd(5, "nslookup", d, "127.0.0.1")
	blocked := inHosts || strings.Contains(out, "0.0.0.0") || strings.Contains(out, "NXDOMAIN")

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"domain":      d,
		"blocked":     blocked,
		"in_hosts_db": inHosts,
		"dns_output":  out,
	})
}

// ─── Real-Time Ad Stream Inspector & Pattern Recognizer ────────────────────

type AdStreamEvent struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	Category  string `json:"category"` // Video Ad, Tracker, Popup/Push, Telemetry, Normal
	ClientIP  string `json:"client_ip"`
	Action    string `json:"action"`   // BLOCKED, ANALYZED, ALLOWED
	Pattern   string `json:"pattern"`
	Timestamp string `json:"timestamp"`
}

var (
	adStreamsMu     sync.Mutex
	adStreamsEvents []AdStreamEvent
	adStreamStats   = map[string]int{"blocked": 0, "analyzed": 0, "video_ads": 0, "trackers": 0}
)

var knownAdStreamSignatures = []struct {
	Keyword  string
	Category string
	Pattern  string
}{
	{"googlevideo.com", "Video Ad Stream", "Google / YouTube Video Ad Fragment"},
	{"pagead2", "Banner & Display Ads", "Google Syndication / Adsense"},
	{"doubleclick.net", "Ad Tracker & Bidding", "DoubleClick Real-Time Bidder"},
	{"byteoversea.com", "TikTok / ByteDance Tracker", "TikTok In-Stream Telemetry"},
	{"analytics.tiktok.com", "TikTok Video Ads", "TikTok Ad Analytics"},
	{"pixel.facebook.com", "Social Tracking Pixel", "Meta / Facebook Conversion Pixel"},
	{"unityads.unity3d.com", "In-App Video Ads", "Unity3D Mobile Ad Video Network"},
	{"applovin.com", "In-App Ad Stream", "AppLovin MAX Video Ad Exchange"},
	{"vungle.com", "Video Ad Network", "Liftoff / Vungle Mobile Video SDK"},
	{"criteo.com", "Dynamic Retargeting", "Criteo Behavioral Ad Stream"},
	{"scorecardresearch.com", "Audience Measurement", "Comscore Web Surveillance"},
	{"appsflyer.com", "Mobile Attribution", "AppsFlyer Telemetry Stream"},
	{"adjust.com", "Device Fingerprinting", "Adjust Analytics Tracker"},
	{"popads.net", "Aggressive Popup", "PopAds Network"},
	{"propellerads.com", "Push & Interstitial", "PropellerAds Push Ads"},
	{"tracking.miui.com", "Device Telemetry", "Xiaomi MIUI Built-In Ad Tracker"},
	{"telemetry.microsoft.com", "OS Telemetry", "Windows Diagnostics Stream"},
}

func inspectDomainStream(domain, clientIP string) AdStreamEvent {
	domainLower := strings.ToLower(domain)
	category := "Luồng Dữ Liệu Bình Thường"
	pattern := "Standard DNS Resolution"
	isAd := false

	for _, sig := range knownAdStreamSignatures {
		if strings.Contains(domainLower, sig.Keyword) {
			category = sig.Category
			pattern = sig.Pattern
			isAd = true
			break
		}
	}

	action := "ALLOWED"
	if isAd {
		action = "BLOCKED"
	}

	evt := AdStreamEvent{
		ID:        fmt.Sprintf("st-%d", time.Now().UnixNano()/1e6),
		Domain:    domain,
		Category:  category,
		ClientIP:  clientIP,
		Action:    action,
		Pattern:   pattern,
		Timestamp: time.Now().Format("15:04:05"),
	}

	adStreamsMu.Lock()
	defer adStreamsMu.Unlock()
	adStreamStats["analyzed"]++
	if isAd {
		adStreamStats["blocked"]++
		if strings.Contains(category, "Video") {
			adStreamStats["video_ads"]++
		} else {
			adStreamStats["trackers"]++
		}
	}
	adStreamsEvents = append([]AdStreamEvent{evt}, adStreamsEvents...)
	if len(adStreamsEvents) > 100 {
		adStreamsEvents = adStreamsEvents[:100]
	}
	return evt
}

// GET / POST / DELETE /api/v1/adblock/streams
func handleAdblockStreams(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		adStreamsMu.Lock()
		eventsCopy := append([]AdStreamEvent{}, adStreamsEvents...)
		statsCopy := map[string]int{
			"blocked":   adStreamStats["blocked"],
			"analyzed":  adStreamStats["analyzed"],
			"video_ads": adStreamStats["video_ads"],
			"trackers":  adStreamStats["trackers"],
		}
		adStreamsMu.Unlock()

		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"streams":    eventsCopy,
			"stats":      statsCopy,
			"total":      len(eventsCopy),
			"signatures": len(knownAdStreamSignatures),
		})

	case http.MethodPost:
		var body struct {
			Domain string `json:"domain"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "invalid JSON")
			return
		}
		d := normalizeDomain(body.Domain)
		if d == "" {
			writeJSON(w, http.StatusBadRequest, false, "Tên miền không hợp lệ")
			return
		}
		cfg := loadAdblockConfig()
		cfg.Blacklist = append(cfg.Blacklist, d)
		_, _ = applyAdblockRuntime(&cfg, false)
		evt := inspectDomainStream(d, "manual-block")
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"blocked": true,
			"domain":  d,
			"event":   evt,
		})

	case http.MethodDelete:
		adStreamsMu.Lock()
		adStreamsEvents = []AdStreamEvent{}
		adStreamStats["blocked"] = 0
		adStreamStats["analyzed"] = 0
		adStreamStats["video_ads"] = 0
		adStreamStats["trackers"] = 0
		adStreamsMu.Unlock()
		writeJSON(w, http.StatusOK, true, map[string]interface{}{
			"cleared": true,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, false, "method not allowed")
	}
}

