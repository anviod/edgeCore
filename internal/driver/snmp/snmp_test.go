package snmp

import (
	"testing"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"
)

func TestSNMPDriverRegistration(t *testing.T) {
	d, ok := driver.GetDriver("snmp")
	if !ok {
		t.Fatal("snmp driver not registered")
	}
	if d == nil {
		t.Fatal("nil driver factory result")
	}
}

func TestSNMPDriverInitHealth(t *testing.T) {
	d := NewSNMPDriver()
	if err := d.Init(modelDriverConfigV2c()); err != nil {
		t.Fatal(err)
	}
	if d.Health() != driver.HealthStatusBad {
		t.Fatalf("expected bad health before connect")
	}
}

func TestSNMPDriverInitV3(t *testing.T) {
	d := NewSNMPDriver()
	cfg := model.DriverConfig{
		Protocol: "snmp",
		Config: map[string]any{
			"snmpVersion":   "v3",
			"ip":            "192.168.1.1",
			"port":          161,
			"securityName":  "admin",
			"securityLevel": "authPriv",
			"authProtocol":  "SHA256",
			"authPassword":  "AuthPass123",
			"privProtocol":  "AES128",
			"privPassword":  "PrivPass123",
		},
	}
	if err := d.Init(cfg); err != nil {
		t.Fatal(err)
	}
	if d.Health() != driver.HealthStatusBad {
		t.Fatal("expected bad health before connect")
	}
}

func TestSNMPConnectionMetricsBeforeConnect(t *testing.T) {
	d := NewSNMPDriver()
	if err := d.Init(modelDriverConfigV2c()); err != nil {
		t.Fatal(err)
	}
	_, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if reconCount != 0 {
		t.Errorf("expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("expected empty local addr, got %q", localAddr)
	}
	if remoteAddr != "127.0.0.1:161" {
		t.Errorf("expected remote addr 127.0.0.1:161, got %q", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("expected zero last disconnect time, got %v", lastDisc)
	}
}

func modelDriverConfigV2c() model.DriverConfig {
	return model.DriverConfig{
		Protocol: "snmp",
		Config: map[string]any{
			"ip":        "127.0.0.1",
			"port":      161,
			"community": "public",
		},
	}
}

func TestBuildV3SecurityLevels(t *testing.T) {
	tests := []struct {
		name    string
		cfg     deviceConfig
		wantErr bool
	}{
		{
			name: "noAuthNoPriv",
			cfg: deviceConfig{
				SecurityName:  "user",
				SecurityLevel: "noAuthNoPriv",
			},
		},
		{
			name: "authNoPriv",
			cfg: deviceConfig{
				SecurityName:  "user",
				SecurityLevel: "authNoPriv",
				AuthProtocol:  "SHA256",
				AuthPassword:  "pass",
			},
		},
		{
			name: "authPriv",
			cfg: deviceConfig{
				SecurityName:  "user",
				SecurityLevel: "authPriv",
				AuthProtocol:  "MD5",
				AuthPassword:  "authpass",
				PrivProtocol:  "DES",
				PrivPassword:  "privpass",
			},
		},
		{
			name: "authPriv missing password",
			cfg: deviceConfig{
				SecurityName:  "user",
				SecurityLevel: "authPriv",
				AuthProtocol:  "SHA256",
				AuthPassword:  "authpass",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sec, err := buildV3Security(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if sec.params.UserName != tt.cfg.SecurityName {
				t.Fatalf("unexpected username %q", sec.params.UserName)
			}
		})
	}
}

func TestMapAuthPrivProtocols(t *testing.T) {
	authCases := map[string]gosnmp.SnmpV3AuthProtocol{
		"MD5":    gosnmp.MD5,
		"SHA1":   gosnmp.SHA,
		"SHA224": gosnmp.SHA224,
		"SHA256": gosnmp.SHA256,
		"SHA384": gosnmp.SHA384,
		"SHA512": gosnmp.SHA512,
	}
	for name, want := range authCases {
		got, err := mapAuthProtocol(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}

	privCases := map[string]gosnmp.SnmpV3PrivProtocol{
		"DES":    gosnmp.DES,
		"AES128": gosnmp.AES,
		"AES192": gosnmp.AES192,
		"AES256": gosnmp.AES256,
	}
	for name, want := range privCases {
		got, err := mapPrivProtocol(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}
}
