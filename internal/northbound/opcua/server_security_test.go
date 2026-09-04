package opcua

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/awcullen/opcua/client"
	"github.com/awcullen/opcua/ua"
	"go.uber.org/zap"
)

func TestServerSecureChannelBasic256Sha256(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	tmpDir := testOutputDir(t)
	serverCert := filepath.Join(tmpDir, "server.crt")
	serverKey := filepath.Join(tmpDir, "server.key")
	clientCert := filepath.Join(tmpDir, "client.crt")
	clientKey := filepath.Join(tmpDir, "client.key")

	if err := writeSelfSignedCert(serverCert, serverKey, "urn:edgeCore-gateway:SecureTest", "SecureTest"); err != nil {
		t.Fatalf("write server cert: %v", err)
	}
	if err := writeSelfSignedCert(clientCert, clientKey, "urn:test:client", "TestClient"); err != nil {
		t.Fatalf("write client cert: %v", err)
	}

	const port = 4860
	endpoint := "/ipp/opcua/server"
	sb := NewMockSouthboundManager()
	srv := NewServer(model.OPCUAConfig{
		Name:        "SecureTest",
		Port:        port,
		Endpoint:    endpoint,
		AuthMethods: []string{"Anonymous"},
		CertFile:    serverCert,
		KeyFile:     serverKey,
		// Empty TrustedCertPath reproduces the default UI configuration.
	}, sb, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Stop()

	time.Sleep(500 * time.Millisecond)

	endpointURL := "opc.tcp://127.0.0.1:" + strconv.Itoa(port) + endpoint
	ctx := context.Background()
	ch, err := client.Dial(
		ctx,
		endpointURL,
		client.WithSecurityPolicyURI(ua.SecurityPolicyURIBasic256Sha256, ua.MessageSecurityModeSignAndEncrypt),
		client.WithClientCertificatePaths(clientCert, clientKey),
		client.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("dial Basic256Sha256 SignAndEncrypt: %v", err)
	}
	defer func() {
		_ = ch.Close(ctx)
	}()

	if ch.SecurityPolicyURI() != ua.SecurityPolicyURIBasic256Sha256 {
		t.Fatalf("security policy = %s, want %s", ch.SecurityPolicyURI(), ua.SecurityPolicyURIBasic256Sha256)
	}
	if ch.SecurityMode() != ua.MessageSecurityModeSignAndEncrypt {
		t.Fatalf("security mode = %s, want %s", ch.SecurityMode(), ua.MessageSecurityModeSignAndEncrypt)
	}
}

func writeSelfSignedCert(certFile, keyFile, appURI, commonName string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	uri, err := url.Parse(appURI)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"edgeCore Gateway Test"},
			CommonName:   commonName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		URIs:                  []*url.URL{uri},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		certOut.Close()
		return err
	}
	certOut.Close()

	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		keyOut.Close()
		return err
	}
	return keyOut.Close()
}
