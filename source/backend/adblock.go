package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	adblockOpMu     sync.Mutex
	adblockUpdating atomic.Bool
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
	"adjust.com", "app.adjust.com", "branch.io", "api.branch.io", "api2.branch.io",
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
	if settingsDBLoadJSON(bucketSettings, "adblock", adblockConfigFile, &cfg) {
		if cfg.UpstreamDNS == nil {
			cfg.UpstreamDNS = []string{}
		}
	} else if len(cfg.UpstreamDNS) == 0 {
		cfg.UpstreamDNS = []string{"1.1.1.1", "8.8.8.8"}
	}
	if cfg.BlockMode != "zero_ip" && cfg.BlockMode != "nxdomain" {
		cfg.BlockMode = "zero_ip"
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
	_ = settingsDBSaveJSON(bucketSettings, "adblock", adblockConfigFile, 0644, cfg)
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
	if !strings.Contains(s, ".") || strings.ContainsAny(s, " *:/\\?&$'\"<>|,=\r\n\t") || strings.Contains(s, "..") {
		return ""
	}
	return s
}

func applyAdblockRuntime(cfg *AdblockConfig, downloadRemote bool) (int, error) {
	adblockMu.Lock()
	adblockUpdating.Store(true)
	defer func() {
		adblockUpdating.Store(false)
		adblockMu.Unlock()
	}()

	_ = os.MkdirAll("/tmp/dnsmasq.d", 0755)
	_ = os.MkdirAll(adblockPersistDir, 0755)

	// 1. Configure Firewall Force-DNS redirect rule
	firewallMu.Lock()
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
	firewallMu.Unlock()

	if len(strings.Fields(strings.TrimSpace(cfg.UpdateSchedule))) != 5 {
		cfg.UpdateSchedule = "0 3 * * *"
	}

	// 2. Remove legacy unauthenticated curl cron entries if present (auto-update runs in-process)
	cronMu.Lock()
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
	cronMu.Unlock()

	if !cfg.Enabled {
		_ = os.Remove(adblockHostsFile)
		_ = os.Remove(adblockDnsmasqCfg)
		firewallMu.Lock()
		_, _ = runCmd(4, "uci", "delete", "firewall.r3p_adblock_wan_dns")
		_, _ = runCmd(4, "uci", "commit", "firewall")
		firewallMu.Unlock()
		dhcpMu.Lock()
		_, _ = runCmd(5, "uci", "delete", "dhcp.@dnsmasq[0].addnhosts")
		_, _ = runCmd(4, "uci", "set", "dhcp.@dnsmasq[0].localservice=1")
		_, _ = runCmd(5, "uci", "commit", "dhcp")
		dhcpMu.Unlock()
		_, _ = runCmd(8, "/etc/init.d/dnsmasq", "restart")
		_, _ = runCmd(8, "/etc/init.d/firewall", "reload")
		cfg.TotalBlocked = 0
		cfg.LastRebuild = time.Now().Format("2006-01-02 15:04:05")
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
		client := newPublicHTTPClient(publicClientOptions{
			Timeout:   15 * time.Second,
			AllowHTTP: true,
		})
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

	// Sort domains and write authoritative /tmp/dnsmasq.d/r3p-adblock.conf cleanly.
	// Avoid duplicating /tmp/r3p-adblock.hosts or rewriting flash persistHosts on simple toggles.
	_ = os.Remove(adblockHostsFile)
	_ = os.Remove("/tmp/hosts/r3p-adblock")
	allDomains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		if !whiteMap[d] && !isDomainWhitelisted(d, cfg.Whitelist) {
			allDomains = append(allDomains, d)
		}
	}
	sort.Strings(allDomains)

	if downloadRemote {
		if f, err := os.Create(persistHosts + ".tmp"); err == nil {
			bw := bufio.NewWriterSize(f, 32*1024)
			for _, d := range allDomains {
				_, _ = bw.WriteString(d)
				_ = bw.WriteByte('\n')
			}
			_ = bw.Flush()
			_ = f.Close()
			_ = os.Rename(persistHosts+".tmp", persistHosts)
		}
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
	_ = atomicWriteFile(adblockDnsmasqCfg, confBytes, 0644)

	dhcpMu.Lock()
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
	var validUpstream []string
	for _, srv := range cfg.UpstreamDNS {
		if t := strings.TrimSpace(srv); netParseIP(t) {
			validUpstream = append(validUpstream, t)
		}
	}
	_, _ = runCmd(4, "uci", "delete", "dhcp.@dnsmasq[0].server")
	for _, srv := range validUpstream {
		_, _ = runCmd(4, "uci", "add_list", "dhcp.@dnsmasq[0].server="+srv)
	}
	_, _ = runCmd(5, "uci", "commit", "dhcp")
	dhcpMu.Unlock()

	// Only allow Port 53 from private RFC1918 WAN subnets when explicitly opted in (never 0.0.0.0/0 public WAN)
	firewallMu.Lock()
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
	firewallMu.Unlock()

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
	if err := validatePublicHTTPURL(context.Background(), u); err != nil {
		return nil, fmt.Errorf("blocked URL: %w", err)
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

func parseAdblockScheduleHM(sched string) (int, int) {
	fields := strings.Fields(strings.TrimSpace(sched))
	if len(fields) < 2 {
		return 0, 3
	}
	m, err1 := strconv.Atoi(fields[0])
	h, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil || m < 0 || m > 59 || h < 0 || h > 23 {
		return 0, 3
	}
	return m, h
}

func initAdblockOnStartup() {
	go func() {
		time.Sleep(2 * time.Second)
		cfg := loadAdblockConfig()
		if cfg.Enabled {
			_, _ = applyAdblockRuntime(&cfg, false)
		}
		lastRunDay := ""
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for now := range ticker.C {
			c := loadAdblockConfig()
			if !c.Enabled || !c.AutoUpdate {
				continue
			}
			schedMin, schedHour := parseAdblockScheduleHM(c.UpdateSchedule)
			today := now.Format("2006-01-02")
			if now.Hour() == schedHour && now.Minute() == schedMin && lastRunDay != today {
				lastRunDay = today
				_, _ = applyAdblockRuntime(&c, true)
			}
		}
	}()
}

func countActiveAdblockConfDomains() int {
	f, err := os.Open(adblockDnsmasqCfg)
	if err != nil {
		return 0
	}
	defer f.Close()
	seen := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasSuffix(line, "/0.0.0.0") || (strings.HasPrefix(line, "server=/") && strings.HasSuffix(line, "/")) {
			seen++
		}
	}
	return seen
}

// GET /api/v1/adblock
func handleAdblockGet(w http.ResponseWriter, r *http.Request) {
	if !requireOpenWrt(w) {
		return
	}
	cfg := loadAdblockConfig()

	activeLines := countActiveAdblockConfDomains()
	if activeLines > 0 {
		cfg.TotalBlocked = activeLines
	}

	// Read recent DNS query / blocked logs from logread
	logOut, _ := runCmd(4, "logread", "-l", "180", "-e", "dnsmasq")
	var recentLogs []string
	blockedHits := 0
	for _, line := range strings.Split(logOut, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		isBlock := strings.Contains(t, "0.0.0.0") || strings.Contains(t, " is ::") || strings.Contains(t, "NXDOMAIN") || strings.Contains(t, adblockDnsmasqCfg)
		if isBlock {
			blockedHits++
		}
		if isBlock || strings.Contains(t, "query[") || strings.Contains(t, "forwarded") || strings.Contains(t, "cached") {
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
		"updating":      adblockUpdating.Load(),
		"active_rules":  activeLines,
		"blocked_hits":  blockedHits,
		"recent_logs":   recentLogs,
		"hosts_file":    adblockDnsmasqCfg,
		"config_file":   adblockDnsmasqCfg,
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

	adblockOpMu.Lock()
	defer adblockOpMu.Unlock()

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
	if body.UpstreamDNS != nil {
		var cleanDNS []string
		for _, srv := range body.UpstreamDNS {
			if t := strings.TrimSpace(srv); t != "" {
				if !netParseIP(t) {
					writeJSON(w, http.StatusBadRequest, false, "Địa chỉ DNS không hợp lệ: "+t)
					return
				}
				cleanDNS = append(cleanDNS, t)
			}
		}
		if cleanDNS == nil {
			cleanDNS = []string{}
		}
		cfg.UpstreamDNS = cleanDNS
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
	adblockOpMu.Lock()
	defer adblockOpMu.Unlock()

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
	adblockOpMu.Lock()
	defer adblockOpMu.Unlock()

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
		if name == "" {
			writeJSON(w, http.StatusBadRequest, false, "Vui lòng nhập Tên danh sách chặn")
			return
		}
		if err := validatePublicHTTPURL(r.Context(), u); err != nil {
			writeJSON(w, http.StatusBadRequest, false, "URL danh sách không hợp lệ hoặc trỏ tới IP nội bộ: "+err.Error())
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
		found := false
		for i := range cfg.Sources {
			if cfg.Sources[i].ID == body.ID {
				found = true
				cfg.Sources[i].Enabled = body.Enabled
				if !body.Enabled {
					cfg.Sources[i].Status = "Disabled"
				} else {
					cfg.Sources[i].Status = "Ready"
				}
			}
		}
		if !found {
			writeJSON(w, http.StatusNotFound, false, "Không tìm thấy nguồn danh sách cần cập nhật")
			return
		}
		saveAdblockConfig(cfg)
		writeJSON(w, http.StatusOK, true, map[string]interface{}{"config": cfg})

	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		var updated []AdblockSource
		found := false
		for _, s := range cfg.Sources {
			if s.ID != id {
				updated = append(updated, s)
			} else {
				found = true
			}
		}
		if !found {
			writeJSON(w, http.StatusNotFound, false, "Không tìm thấy nguồn danh sách cần xóa")
			return
		}
		if updated == nil {
			updated = []AdblockSource{}
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
	adblockOpMu.Lock()
	defer adblockOpMu.Unlock()

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
		if domain == "" {
			writeJSON(w, http.StatusBadRequest, false, "Thiếu tên miền cần xóa")
			return
		}
		found := false
		if listType == "whitelist" {
			var next []string
			for _, x := range cfg.Whitelist {
				if x != domain {
					next = append(next, x)
				} else {
					found = true
				}
			}
			if next == nil {
				next = []string{}
			}
			cfg.Whitelist = next
		} else {
			var next []string
			for _, x := range cfg.Blacklist {
				if x != domain {
					next = append(next, x)
				} else {
					found = true
				}
			}
			if next == nil {
				next = []string{}
			}
			cfg.Blacklist = next
		}
		if !found {
			writeJSON(w, http.StatusNotFound, false, "Không tìm thấy tên miền trong danh sách")
			return
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

func isDomainInAdblockConf(path, domain string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Match exact domain or any parent suffix wildcard present in dnsmasq conf
	candidates := []string{domain}
	parts := strings.Split(domain, ".")
	for i := 1; i+1 < len(parts); i++ {
		candidates = append(candidates, strings.Join(parts[i:], "."))
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		for _, c := range candidates {
			if strings.HasPrefix(line, "address=/"+c+"/") || line == "server=/"+c+"/" {
				return true
			}
		}
	}
	return false
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

	inConf := isDomainInAdblockConf(adblockDnsmasqCfg, d)
	out, _ := runCmd(5, "nslookup", d, "127.0.0.1")
	blocked := inConf || strings.Contains(out, "0.0.0.0") || strings.Contains(out, "NXDOMAIN")

	writeJSON(w, http.StatusOK, true, map[string]interface{}{
		"domain":      d,
		"blocked":     blocked,
		"in_hosts_db": inConf,
		"in_config":   inConf,
		"config_file": adblockDnsmasqCfg,
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

var adStreamSeenMu sync.Mutex
var adStreamSeen = map[string]struct{}{}

func syncAdblockStreamsFromLogs() {
	logOut, err := runCmd(3, "logread", "-l", "120", "-e", "dnsmasq")
	if err != nil || logOut == "" {
		return
	}
	for _, rawLine := range strings.Split(logOut, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || (!strings.Contains(line, "query[") && !strings.Contains(line, "0.0.0.0") && !strings.Contains(line, "NXDOMAIN") && !strings.Contains(line, " is ::")) {
			continue
		}
		adStreamSeenMu.Lock()
		if _, dup := adStreamSeen[line]; dup {
			adStreamSeenMu.Unlock()
			continue
		}
		if len(adStreamSeen) > 256 {
			adStreamSeen = map[string]struct{}{}
		}
		adStreamSeen[line] = struct{}{}
		adStreamSeenMu.Unlock()

		// Extract queried/blocked domain & client IP from dnsmasq syslog format:
		// "dnsmasq[...]: query[A] doubleclick.net from 192.168.2.50" OR "config ads.youtube.com is 0.0.0.0"
		fields := strings.Fields(line)
		var domain, clientIP string
		forcedBlock := strings.Contains(line, "0.0.0.0") || strings.Contains(line, "NXDOMAIN") || strings.Contains(line, " is ::")
		for i, f := range fields {
			if strings.HasPrefix(f, "query[") && i+1 < len(fields) {
				domain = normalizeDomain(fields[i+1])
			} else if f == "from" && i+1 < len(fields) {
				clientIP = fields[i+1]
			} else if (f == "config" || f == "cached" || f == "reply") && i+1 < len(fields) && domain == "" {
				domain = normalizeDomain(fields[i+1])
			}
		}
		if domain != "" {
			if forcedBlock {
				inspectDomainStreamWithBlock(domain, clientIP, true)
			} else {
				inspectDomainStreamWithBlock(domain, clientIP, false)
			}
		}
	}
}

func inspectDomainStream(domain, clientIP string) AdStreamEvent {
	return inspectDomainStreamWithBlock(domain, clientIP, false)
}

func inspectDomainStreamWithBlock(domain, clientIP string, dnsBlocked bool) AdStreamEvent {
	domainLower := strings.ToLower(domain)
	category := "Luồng Dữ Liệu Bình Thường"
	pattern := "Standard DNS Resolution"
	isAd := dnsBlocked

	for _, sig := range knownAdStreamSignatures {
		if strings.Contains(domainLower, sig.Keyword) {
			category = sig.Category
			pattern = sig.Pattern
			isAd = true
			break
		}
	}
	if dnsBlocked && category == "Luồng Dữ Liệu Bình Thường" {
		category = "DNS Rule Blocklist"
		pattern = "Dnsmasq Filtered Query"
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
		syncAdblockStreamsFromLogs()
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
		adblockOpMu.Lock()
		cfg := loadAdblockConfig()
		// Remove from whitelist if manually blocking from live stream inspector
		if len(cfg.Whitelist) > 0 {
			var filteredWhite []string
			for _, x := range cfg.Whitelist {
				if normalizeDomain(x) != d {
					filteredWhite = append(filteredWhite, x)
				}
			}
			if filteredWhite == nil {
				filteredWhite = []string{}
			}
			cfg.Whitelist = filteredWhite
		}
		exists := false
		for _, x := range cfg.Blacklist {
			if normalizeDomain(x) == d {
				exists = true
				break
			}
		}
		if !exists {
			cfg.Blacklist = append(cfg.Blacklist, d)
		}
		cfg.Enabled = true
		_, _ = applyAdblockRuntime(&cfg, false)
		adblockOpMu.Unlock()

		evt := inspectDomainStreamWithBlock(d, "manual-block", true)
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

