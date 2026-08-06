package opcua

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/awcullen/opcua/client"
	"github.com/awcullen/opcua/ua"
	"go.uber.org/zap"
)

func TestSupportedSecurityCapabilitiesAuto(t *testing.T) {
	caps := SupportedSecurityCapabilities(model.OPCUAConfig{SecurityPolicy: "Auto"})
	if len(caps) < 10 {
		t.Fatalf("expected many Auto capabilities, got %d", len(caps))
	}
	found := false
	for _, c := range caps {
		if c.Policy == "Basic256Sha256" && c.Mode == "SignAndEncrypt" && c.Recommended {
			found = true
		}
	}
	if !found {
		t.Fatal("expected Basic256Sha256 SignAndEncrypt to be recommended")
	}
}

func TestSupportedSecurityCapabilitiesExplicitPolicy(t *testing.T) {
	caps := SupportedSecurityCapabilities(model.OPCUAConfig{SecurityPolicy: "Basic256Sha256"})
	for _, c := range caps {
		if c.Policy == "None" {
			t.Fatal("None endpoint should be disabled for explicit secure policy")
		}
		if c.Policy != "Basic256Sha256" {
			t.Fatalf("unexpected policy %s", c.Policy)
		}
	}
}

func TestGetEndpointsMultiplePolicies(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	tmpDir := testOutputDir(t)
	serverCert := filepath.Join(tmpDir, "server.crt")
	serverKey := filepath.Join(tmpDir, "server.key")
	if err := writeSelfSignedCert(serverCert, serverKey, "urn:edgeCore-gateway:MultiPolicy", "MultiPolicy"); err != nil {
		t.Fatalf("write server cert: %v", err)
	}

	const port = 4861
	sb := NewMockSouthboundManager()
	srv := NewServer(model.OPCUAConfig{
		Name:           "MultiPolicy",
		Port:           port,
		Endpoint:       "/ipp/opcua/server",
		SecurityPolicy: "Auto",
		AuthMethods:    []string{"Anonymous"},
		CertFile:       serverCert,
		KeyFile:        serverKey,
	}, sb, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Stop()
	time.Sleep(500 * time.Millisecond)

	endpointURL := "opc.tcp://127.0.0.1:" + strconv.Itoa(port) + "/ipp/opcua/server"
	res, err := client.GetEndpoints(context.Background(), &ua.GetEndpointsRequest{EndpointURL: endpointURL})
	if err != nil {
		t.Fatalf("GetEndpoints: %v", err)
	}

	want := map[string]struct{}{
		"Basic256Sha256|SignAndEncrypt":        {},
		"Basic256Sha256|Sign":                  {},
		"Aes128_Sha256_RsaOaep|SignAndEncrypt": {},
		"None|None":                            {},
	}
	for _, ep := range res.Endpoints {
		policy := strings.TrimPrefix(ep.SecurityPolicyURI, "http://opcfoundation.org/UA/SecurityPolicy#")
		key := policy + "|" + ep.SecurityMode.String()
		delete(want, key)
	}
	for key := range want {
		t.Errorf("missing endpoint combination: %s", key)
	}
}

func TestServerUserNameAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	tmpDir := testOutputDir(t)
	serverCert := filepath.Join(tmpDir, "server.crt")
	serverKey := filepath.Join(tmpDir, "server.key")
	clientCert := filepath.Join(tmpDir, "client.crt")
	clientKey := filepath.Join(tmpDir, "client.key")
	if err := writeSelfSignedCert(serverCert, serverKey, "urn:edgeCore-gateway:UserAuth", "UserAuth"); err != nil {
		t.Fatalf("write server cert: %v", err)
	}
	if err := writeSelfSignedCert(clientCert, clientKey, "urn:test:client", "TestClient"); err != nil {
		t.Fatalf("write client cert: %v", err)
	}

	const port = 4862
	sb := NewMockSouthboundManager()
	srv := NewServer(model.OPCUAConfig{
		Name:           "UserAuth",
		Port:           port,
		Endpoint:       "/",
		SecurityPolicy: "Basic256Sha256",
		AuthMethods:    []string{"UserName"},
		Users:          map[string]string{"operator": "secret"},
		CertFile:       serverCert,
		KeyFile:        serverKey,
	}, sb, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Stop()
	time.Sleep(500 * time.Millisecond)

	endpointURL := "opc.tcp://127.0.0.1:" + strconv.Itoa(port) + "/"
	ctx := context.Background()

	badCh, err := client.Dial(
		ctx,
		endpointURL,
		client.WithSecurityPolicyURI(ua.SecurityPolicyURIBasic256Sha256, ua.MessageSecurityModeSignAndEncrypt),
		client.WithClientCertificatePaths(clientCert, clientKey),
		client.WithInsecureSkipVerify(),
		client.WithUserNameIdentity("operator", "wrong"),
	)
	if err == nil {
		_ = badCh.Close(ctx)
		t.Fatal("expected auth failure with wrong password")
	}

	goodCh, err := client.Dial(
		ctx,
		endpointURL,
		client.WithSecurityPolicyURI(ua.SecurityPolicyURIBasic256Sha256, ua.MessageSecurityModeSignAndEncrypt),
		client.WithClientCertificatePaths(clientCert, clientKey),
		client.WithInsecureSkipVerify(),
		client.WithUserNameIdentity("operator", "secret"),
	)
	if err != nil {
		t.Fatalf("dial with valid credentials: %v", err)
	}
	_ = goodCh.Close(ctx)
}
