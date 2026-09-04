package model

import "testing"

func TestEnsureDeviceID_SanitizesSlash(t *testing.T) {
	dev := &Device{ID: "c/metrics28319"}
	if err := EnsureDeviceID(dev); err != nil {
		t.Fatalf("EnsureDeviceID error: %v", err)
	}
	if dev.ID != "c-metrics28319" {
		t.Fatalf("expected c-metrics28319, got %q", dev.ID)
	}
}

func TestEnsureDeviceID_FallsBackToNameThenSanitizes(t *testing.T) {
	dev := &Device{Name: "foo/bar baz"}
	if err := EnsureDeviceID(dev); err != nil {
		t.Fatalf("EnsureDeviceID error: %v", err)
	}
	if dev.ID != "foo-bar-baz" {
		t.Fatalf("expected foo-bar-baz, got %q", dev.ID)
	}
}

func TestEnsureDeviceID_PreservesSafeID(t *testing.T) {
	dev := &Device{ID: "bacnet-device-001"}
	if err := EnsureDeviceID(dev); err != nil {
		t.Fatalf("EnsureDeviceID error: %v", err)
	}
	if dev.ID != "bacnet-device-001" {
		t.Fatalf("expected bacnet-device-001, got %q", dev.ID)
	}
}

func TestEnsureDeviceID_EmptyReturnsError(t *testing.T) {
	dev := &Device{}
	if err := EnsureDeviceID(dev); err == nil {
		t.Fatal("expected error for empty device ID")
	}
}

func TestEnsurePointID_SanitizesSlash(t *testing.T) {
	p := &Point{ID: "ns=2;s/MyVar"}
	if err := EnsurePointID(p); err != nil {
		t.Fatalf("EnsurePointID error: %v", err)
	}
	if p.ID != "ns=2;s-MyVar" {
		t.Fatalf("expected ns=2;s-MyVar, got %q", p.ID)
	}
}

func TestEnsureChannelID_SanitizesSlash(t *testing.T) {
	ch := &Channel{ID: "ch/sub"}
	if err := EnsureChannelID(ch); err != nil {
		t.Fatalf("EnsureChannelID error: %v", err)
	}
	if ch.ID != "ch-sub" {
		t.Fatalf("expected ch-sub, got %q", ch.ID)
	}
}

func TestSanitizeID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"c/metrics28319", "c-metrics28319"},
		{"a\\b", "a-b"},
		{"foo bar", "foo-bar"},
		{"x?y#z", "x-y-z"},
		{"normal-id", "normal-id"},
		{"", ""},
		{"opc.tcp://host:4840", "opc.tcp:--host:4840"},
	}
	for _, tc := range cases {
		if got := sanitizeID(tc.in); got != tc.want {
			t.Fatalf("sanitizeID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
