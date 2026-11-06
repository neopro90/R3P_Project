package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateSelfSignedCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "api.crt")
	keyPath := filepath.Join(dir, "api.key")
	if err := generateSelfSignedCertificate(certPath, keyPath); err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		t.Fatalf("generated key pair is invalid: %v", err)
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read certificate: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("certificate is not PEM encoded")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if err := cert.VerifyHostname("localhost"); err != nil {
		t.Fatalf("certificate does not cover localhost: %v", err)
	}
	if runtime.GOOS != "windows" {
		keyInfo, err := os.Stat(keyPath)
		if err != nil {
			t.Fatalf("stat private key: %v", err)
		}
		if keyInfo.Mode().Perm() != 0600 {
			t.Fatalf("private key mode = %04o, want 0600", keyInfo.Mode().Perm())
		}
	}
	if err := generateSelfSignedCertificate(certPath, keyPath); err != nil {
		t.Fatalf("reusing valid key pair: %v", err)
	}
}

func TestGenerateSelfSignedCertificateRejectsPartialPair(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "api.crt")
	keyPath := filepath.Join(dir, "api.key")
	if err := os.WriteFile(certPath, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := generateSelfSignedCertificate(certPath, keyPath); err == nil {
		t.Fatal("expected partial certificate pair to be rejected")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected key file state: %v", err)
	}
}
