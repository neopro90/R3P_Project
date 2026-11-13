package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildTestTarGz(t *testing.T, entries []struct {
	name     string
	typeflag byte
	linkname string
	body     string
}) string {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0644,
			Size:     int64(len(e.body)),
			Typeflag: e.typeflag,
			Linkname: e.linkname,
		}
		if e.typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if len(e.body) > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write tar body: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	return outPath
}

func TestInspectTarGzSafety(t *testing.T) {
	safePkg := buildTestTarGz(t, []struct {
		name     string
		typeflag byte
		linkname string
		body     string
	}{
		{name: "etc/config/r3p", typeflag: tar.TypeReg, body: "config r3p 'main'\n"},
		{name: "usr/share/r3p/dist/index.html", typeflag: tar.TypeReg, body: "<html></html>"},
	})
	count, err := inspectTarGzSafety(safePkg, true, nil)
	if err != nil || count != 2 {
		t.Fatalf("expected safe tar.gz to pass with 2 entries, got count=%d err=%v", count, err)
	}

	traversalPkg := buildTestTarGz(t, []struct {
		name     string
		typeflag byte
		linkname string
		body     string
	}{
		{name: "../../etc/shadow", typeflag: tar.TypeReg, body: "pwned"},
	})
	if _, err := inspectTarGzSafety(traversalPkg, true, nil); err == nil {
		t.Fatal("expected path traversal tar.gz to be rejected")
	}

	escapeLinkPkg := buildTestTarGz(t, []struct {
		name     string
		typeflag byte
		linkname string
		body     string
	}{
		{name: "etc/r3p/escape", typeflag: tar.TypeSymlink, linkname: "../../../etc/passwd"},
	})
	if _, err := inspectTarGzSafety(escapeLinkPkg, true, nil); err == nil {
		t.Fatal("expected escaping symlink in tar.gz to be rejected")
	}
}

func TestFirmwareAndGitValidationHelpers(t *testing.T) {
	validBranches := []string{"main", "release/v2.6", "feat_security-1"}
	for _, b := range validBranches {
		if !isSafeGitBranch(b) {
			t.Errorf("expected branch %q to be valid", b)
		}
	}
	badBranches := []string{"", "../main", "-bad", "main;id", "feat..bar", "branch name"}
	for _, b := range badBranches {
		if isSafeGitBranch(b) {
			t.Errorf("expected branch %q to be rejected", b)
		}
	}

	dir := t.TempDir()
	elfPath := filepath.Join(dir, "r3p-api")
	if err := os.WriteFile(elfPath, []byte{0x7f, 'E', 'L', 'F', 1, 1, 1, 0}, 0600); err != nil {
		t.Fatal(err)
	}
	if !isELFBinary(elfPath) {
		t.Fatal("expected valid ELF header to pass isELFBinary")
	}
	htmlPath := filepath.Join(dir, "notfound.bin")
	if err := os.WriteFile(htmlPath, []byte("<html>404 Not Found</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	if isELFBinary(htmlPath) {
		t.Fatal("expected HTML payload to fail isELFBinary")
	}
}

func TestNetworkAndFirewallValidators(t *testing.T) {
	validMasks := []string{"255.255.255.0", "255.255.0.0", "255.255.255.252", "255.0.0.0"}
	for _, m := range validMasks {
		if !isValidIPv4Netmask(m) {
			t.Errorf("expected netmask %q to be valid", m)
		}
	}
	badMasks := []string{"", "255.0.255.0", "255.255.255.1", "0.0.0.0", "999.0.0.0", "::1"}
	for _, m := range badMasks {
		if isValidIPv4Netmask(m) {
			t.Errorf("expected netmask %q to be rejected", m)
		}
	}

	for _, p := range []string{"80", "443", "8000-8100"} {
		if !isValidPortSpec(p) {
			t.Errorf("expected port spec %q to be valid", p)
		}
	}
	for _, p := range []string{"0", "70000", "80;id", "900-100", "abc"} {
		if isValidPortSpec(p) {
			t.Errorf("expected port spec %q to be rejected", p)
		}
	}
}

func TestSanitizersCronRepoAndDDNS(t *testing.T) {
	if !isValidCronSchedule("*/15 * * * *") {
		t.Fatal("expected standard 5-field cron to be valid")
	}
	if isValidCronSchedule("0 4 * * * # injected") || isValidCronSchedule("0 4 * *") || isValidCronSchedule("0 4 * * *\n* * * * *") {
		t.Fatal("expected malformed or multi-line cron schedule to fail")
	}

	if !isValidRepoFeedURL("https://downloads.openwrt.org/releases/25.12.5/packages/mipsel_24kc/packages/packages.adb") {
		t.Fatal("expected valid https package feed URL")
	}
	if isValidRepoFeedURL("https://example.com/feed\nevil") || isValidRepoFeedURL("file:///etc/passwd") {
		t.Fatal("expected newline or non-http feed URL to be rejected")
	}

	if got := maskDDNSToken("secret-token-12345"); strings.Contains(got, "secret") || !strings.HasPrefix(got, "••••") {
		t.Fatalf("unexpected masked token: %q", got)
	}

	if seg, ok := sanitizeGDrivePathSegment(".. "); ok || seg != "" {
		t.Fatalf("expected dot-dot segment to fail, got %q", seg)
	}
	base := t.TempDir()
	if _, ok := safeJoinUnderBase(base, "../outside.txt"); ok {
		t.Fatal("expected safeJoinUnderBase to reject parent escape")
	}
	if joined, ok := safeJoinUnderBase(base, "sub/file.jpg"); !ok || !strings.HasSuffix(joined, filepath.Join("sub", "file.jpg")) {
		t.Fatalf("expected valid child join, got ok=%t joined=%q", ok, joined)
	}

	if macShortID("aa:bb:cc:dd:ee:ff") != "ddeeff" {
		t.Fatalf("unexpected macShortID: %q", macShortID("aa:bb:cc:dd:ee:ff"))
	}
	if id := macShortID("short"); id != "short" {
		t.Fatalf("expected short mac fallback without panic, got %q", id)
	}
}

func TestNASPathAndMountValidation(t *testing.T) {
	for _, valid := range []string{"/overlay/share", "/overlay/share/movies", "/mnt/sda1/media", "/tmp/downloads"} {
		if _, err := validateNASPath(valid); err != nil {
			t.Errorf("expected valid NAS path %q to succeed, got %v", valid, err)
		}
	}
	for _, invalid := range []string{"/overlay/share_evil", "/mnt", "/media", "/etc", "/etc/passwd"} {
		if _, err := validateNASPath(invalid); err == nil {
			t.Errorf("expected invalid NAS path %q to be rejected", invalid)
		}
	}
}

func TestHandlerGuardsForServicesAutomationAndNASDelete(t *testing.T) {
	// 1. Services control refuses to stop or disable r3p-api
	for _, act := range []string{"stop", "disable"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/services/control", bytes.NewBufferString(`{"service":"r3p-api","action":"`+act+`"}`))
		rr := httptest.NewRecorder()
		handleServicesControl(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 rejecting r3p-api %s, got %d (%s)", act, rr.Code, rr.Body.String())
		}
	}

	// 2. Automation run refuses arbitrary shell execution
	req := httptest.NewRequest(http.MethodPost, "/api/v1/automation/run", bytes.NewBufferString(`{"command":"cat /etc/shadow"}`))
	rr := httptest.NewRecorder()
	handleAutomationRun(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for raw command on /automation/run, got %d", rr.Code)
	}

	// 3. NAS file delete blocks deleting mount root or /overlay/share
	for _, target := range []string{"/mnt/sda1", "/overlay/share", "/etc/shadow"} {
		delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/nas/file?path="+target, nil)
		delRR := httptest.NewRecorder()
		handleNASDeleteFile(delRR, delReq)
		if delRR.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 deleting protected path %q, got %d (%s)", target, delRR.Code, delRR.Body.String())
		}
	}
}
