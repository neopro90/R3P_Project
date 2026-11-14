package main

import (
	"archive/tar"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMIPSLEELF32BinaryValidation(t *testing.T) {
	dir := t.TempDir()

	// Valid ELF32 Little-Endian MIPS (e_machine = 0x0008 LE)
	validHdr := make([]byte, 20)
	copy(validHdr[:4], []byte{0x7f, 'E', 'L', 'F'})
	validHdr[4] = 1 // ELFCLASS32
	validHdr[5] = 1 // ELFDATA2LSB
	validHdr[18] = 0x08
	validHdr[19] = 0x00
	validPath := filepath.Join(dir, "r3p-api-mipsle")
	if err := os.WriteFile(validPath, validHdr, 0600); err != nil {
		t.Fatal(err)
	}
	if !isMIPSLEELF32Binary(validPath) {
		t.Fatal("expected valid MIPSLE ELF32 binary to pass")
	}

	// Reject ELF64
	elf64 := append([]byte{}, validHdr...)
	elf64[4] = 2
	p64 := filepath.Join(dir, "elf64")
	_ = os.WriteFile(p64, elf64, 0600)
	if isMIPSLEELF32Binary(p64) {
		t.Fatal("expected ELF64 to be rejected")
	}

	// Reject Big-Endian ELF32
	elfBE := append([]byte{}, validHdr...)
	elfBE[5] = 2
	pBE := filepath.Join(dir, "elfbe")
	_ = os.WriteFile(pBE, elfBE, 0600)
	if isMIPSLEELF32Binary(pBE) {
		t.Fatal("expected big-endian ELF to be rejected")
	}

	// Reject x86_64/non-MIPS machine (e_machine = 62)
	elfAMD := append([]byte{}, validHdr...)
	elfAMD[18] = 62
	pAMD := filepath.Join(dir, "elfamd")
	_ = os.WriteFile(pAMD, elfAMD, 0600)
	if isMIPSLEELF32Binary(pAMD) {
		t.Fatal("expected x86-64 e_machine to be rejected")
	}
}

func TestSystemVersionComparisonAndStrictTarBundle(t *testing.T) {
	if !isNewerSystemVersion("v1.1.25", 25, "v1.1.24", 24) {
		t.Fatal("expected v1.1.25 rev 25 > v1.1.24 rev 24")
	}
	if !isNewerSystemVersion("v1.2.0", 1, "v1.1.99", 99) {
		t.Fatal("expected v1.2.0 > v1.1.99")
	}
	if isNewerSystemVersion("v1.1.25", 25, "v1.1.25", 25) {
		t.Fatal("expected identical version/rev not to be newer")
	}

	allowed := []string{"usr/", "www/", "etc/"}
	validBundle := buildTestTarGz(t, []struct {
		name     string
		typeflag byte
		linkname string
		body     string
	}{
		{name: "usr/bin/r3p-api", typeflag: tar.TypeReg, body: "elf"},
		{name: "www/index.html", typeflag: tar.TypeReg, body: "<html></html>"},
		{name: "etc/r3p/version.json", typeflag: tar.TypeReg, body: "{}"},
	})
	if cnt, err := inspectTarGzSafety(validBundle, false, allowed); err != nil || cnt != 3 {
		t.Fatalf("expected valid bundle to pass with 3 files, got cnt=%d err=%v", cnt, err)
	}

	symlinkBundle := buildTestTarGz(t, []struct {
		name     string
		typeflag byte
		linkname string
		body     string
	}{
		{name: "www/link", typeflag: tar.TypeSymlink, linkname: "index.html"},
	})
	if _, err := inspectTarGzSafety(symlinkBundle, false, allowed); err == nil {
		t.Fatal("expected symlink to be rejected when allowSafeSymlinks=false")
	}

	unlistedPrefixBundle := buildTestTarGz(t, []struct {
		name     string
		typeflag byte
		linkname string
		body     string
	}{
		{name: "root/.ssh/authorized_keys", typeflag: tar.TypeReg, body: "ssh-ed25519 AAAA"},
	})
	if _, err := inspectTarGzSafety(unlistedPrefixBundle, false, allowed); err == nil {
		t.Fatal("expected unlisted root/.ssh prefix to be rejected")
	}
}

func TestBackupExclusionsAndDDNSMasking(t *testing.T) {
	excluded := []string{
		"/etc/shadow",
		"/etc/dropbear/dropbear_rsa_host_key",
		"/etc/r3p/r3p.db",
		"/etc/r3p/r3p.db.lock",
		"/etc/r3p/api-token",
		"/etc/r3p/sessions.json",
		"/etc/r3p/ai.json",
		"/etc/r3p/ai_config.json",
		"/etc/r3p/downloader.json",
		"/etc/r3p/telegram.json",
		"/etc/r3p/gdrive.json",
		"/etc/r3p/gdrive_sync.json",
	}
	for _, p := range excluded {
		if !isExcludedBackupPath(p) {
			t.Errorf("expected secret path %q to be excluded from sanitized backup", p)
		}
	}
	for _, p := range []string{"/etc/config/network", "/etc/config/firewall", "/etc/r3p/version.json"} {
		if isExcludedBackupPath(p) {
			t.Errorf("expected config path %q NOT to be excluded", p)
		}
	}

	pub := sanitizePublicDDNS(DDNSConfig{
		Enabled:  true,
		Provider: "duckdns",
		Domain:   "r3phome",
		Token:    "abcd-secret-9999",
	})
	if pub.Token != "••••9999" || pub.Domain != "r3phome" {
		t.Fatalf("unexpected sanitized DDNS config: %+v", pub)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/firmware/apply", nil)
	rr := httptest.NewRecorder()
	handleFirmwareApply(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from handleFirmwareApply outside OpenWrt, got %d", rr.Code)
	}
}
