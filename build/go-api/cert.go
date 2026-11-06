package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func generateSelfSignedCertificate(certPath, keyPath string) error {
	if certPath == "" || keyPath == "" || filepath.Clean(certPath) == filepath.Clean(keyPath) {
		return errors.New("certificate and key must use distinct non-empty paths")
	}

	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	switch {
	case certErr == nil && keyErr == nil:
		_, err := tls.LoadX509KeyPair(certPath, keyPath)
		return err
	case certErr == nil || keyErr == nil:
		return errors.New("only one TLS certificate/key file exists; refusing to replace partial credentials")
	case !errors.Is(certErr, os.ErrNotExist):
		return fmt.Errorf("inspect TLS certificate: %w", certErr)
	case !errors.Is(keyErr, os.ErrNotExist):
		return fmt.Errorf("inspect TLS private key: %w", keyErr)
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return fmt.Errorf("create certificate directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return fmt.Errorf("create private key directory: %w", err)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate TLS private key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return fmt.Errorf("generate TLS certificate serial: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "R3P-Router"
	}
	now := time.Now()
	notBefore := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2036, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !now.Before(notAfter) {
		notBefore = now.Add(-time.Hour)
		notAfter = now.AddDate(1, 0, 0)
	}
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: hostname},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", hostname},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create self-signed TLS certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("encode TLS private key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := writeNewFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("write TLS certificate: %w", err)
	}
	if err := writeNewFile(keyPath, keyPEM, 0600); err != nil {
		_ = os.Remove(certPath)
		return fmt.Errorf("write TLS private key: %w", err)
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		_ = os.Remove(keyPath)
		_ = os.Remove(certPath)
		return fmt.Errorf("validate generated TLS key pair: %w", err)
	}
	return nil
}

func writeNewFile(path string, contents []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
