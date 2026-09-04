package opcua

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

func certStoreDir(channelID string) string {
	id := strings.TrimSpace(channelID)
	if id == "" {
		id = "default"
	}
	return filepath.Join("data", "certs", "opcua", id)
}

// MaterializeServerCerts writes DB-stored PEM to disk, or returns legacy file paths.
func MaterializeServerCerts(cfg model.OPCUAConfig) (certFile, keyFile string, err error) {
	certPEM := strings.TrimSpace(cfg.ServerCertPEM)
	keyPEM := strings.TrimSpace(cfg.ServerKeyPEM)
	if certPEM != "" && keyPEM != "" {
		if err := validateServerPEMPair(certPEM, keyPEM); err != nil {
			return "", "", err
		}
		dir := certStoreDir(cfg.ID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", "", fmt.Errorf("create cert dir: %w", err)
		}
		certPath := filepath.Join(dir, "server.crt")
		keyPath := filepath.Join(dir, "server.key")
		if err := os.WriteFile(certPath, []byte(normalizePEM(certPEM)), 0o600); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(keyPath, []byte(normalizePEM(keyPEM)), 0o600); err != nil {
			return "", "", err
		}
		return certPath, keyPath, nil
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		return cfg.CertFile, cfg.KeyFile, nil
	}
	return "", "", nil
}

// MaterializeTrustedCerts writes trusted client CA/certs to disk for PKI validation.
// Returns base directory containing trusted/, rejected/, crl/ subdirs, or empty if none configured.
func MaterializeTrustedCerts(cfg model.OPCUAConfig) (baseDir string, err error) {
	if len(cfg.TrustedCertsPEM) == 0 {
		if cfg.TrustedCertPath != "" {
			return cfg.TrustedCertPath, nil
		}
		return "", nil
	}

	dir := certStoreDir(cfg.ID)
	trustedDir := filepath.Join(dir, "trusted")
	rejectedDir := filepath.Join(dir, "rejected")
	crlDir := filepath.Join(dir, "crl")
	for _, d := range []string{trustedDir, rejectedDir, crlDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}

	for i, raw := range cfg.TrustedCertsPEM {
		pemText := normalizePEM(raw)
		if pemText == "" {
			continue
		}
		if _, err := parseCertificatePEM(pemText); err != nil {
			return "", fmt.Errorf("trusted cert %d: %w", i, err)
		}
		name := fmt.Sprintf("trusted_%d.pem", i)
		if err := os.WriteFile(filepath.Join(trustedDir, name), []byte(pemText), 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// ValidateServerPEMPair checks that cert and private key PEM match.
func ValidateServerPEMPair(certPEM, keyPEM string) error {
	return validateServerPEMPair(certPEM, keyPEM)
}

func validateServerPEMPair(certPEM, keyPEM string) error {
	if _, err := tls.X509KeyPair([]byte(normalizePEM(certPEM)), []byte(normalizePEM(keyPEM))); err != nil {
		return fmt.Errorf("certificate and private key do not match: %w", err)
	}
	return nil
}

func normalizePEM(s string) string {
	return strings.TrimSpace(s) + "\n"
}

func parseCertificatePEM(pemText string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("PEM block type CERTIFICATE required")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parsePrivateKeyPEM(pemText string) (any, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, fmt.Errorf("PEM decode failed")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type %q", block.Type)
	}
}
