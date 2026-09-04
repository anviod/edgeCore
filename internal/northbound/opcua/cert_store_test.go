package opcua

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func generateTestPEMPair(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-opcua"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM
}

func TestValidateServerPEMPair(t *testing.T) {
	certPEM, keyPEM := generateTestPEMPair(t)
	if err := ValidateServerPEMPair(certPEM, keyPEM); err != nil {
		t.Fatalf("valid pair rejected: %v", err)
	}
	_, otherKey := generateTestPEMPair(t)
	if err := ValidateServerPEMPair(certPEM, otherKey); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestMaterializeServerCerts(t *testing.T) {
	dir := testOutputDir(t)
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	certPEM, keyPEM := generateTestPEMPair(t)
	cfg := model.OPCUAConfig{ID: "ch-1", ServerCertPEM: certPEM, ServerKeyPEM: keyPEM}
	certFile, keyFile, err := MaterializeServerCerts(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(certFile, filepath.Join("data", "certs", "opcua", "ch-1")) {
		t.Fatalf("unexpected cert path: %s", certFile)
	}
	if _, err := os.Stat(certFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializeTrustedCerts(t *testing.T) {
	dir := testOutputDir(t)
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	certPEM, _ := generateTestPEMPair(t)
	cfg := model.OPCUAConfig{ID: "ch-2", TrustedCertsPEM: []string{certPEM}}
	baseDir, err := MaterializeTrustedCerts(cfg)
	if err != nil {
		t.Fatal(err)
	}
	trustedFile := filepath.Join(baseDir, "trusted", "trusted_0.pem")
	if _, err := os.Stat(trustedFile); err != nil {
		t.Fatal(err)
	}
}
