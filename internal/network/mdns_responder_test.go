package network

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/miekg/dns"
)

func TestMDNSResponderAnswersHostnameARecord(t *testing.T) {
	responder, err := newMDNSResponder("edgeCore.local.", []string{"192.168.1.10"}, []mdnsServiceDef{
		{Instance: "edgeCore", Type: "_http._tcp", Port: 8080, TXT: []string{"path=/"}},
	}, nil)
	if err != nil {
		t.Skipf("mDNS responder unavailable in this environment: %v", err)
	}
	defer responder.Shutdown()

	query := new(dns.Msg)
	query.SetQuestion("edgeCore.local.", dns.TypeA)

	resp := new(dns.Msg)
	resp.SetReply(query)
	if !responder.handleQuestion(query.Question[0], resp, 0) {
		t.Fatal("expected hostname A query to be handled")
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 A answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || a.A.String() != "192.168.1.10" {
		t.Fatalf("unexpected A record: %#v", resp.Answer[0])
	}
}

func TestBuildHostnameAccessStatus(t *testing.T) {
	status := BuildHostnameAccessStatus(model.HostnameConfig{
		Name:       "edgeCore",
		HTTPPort:   8080,
		EnableMDNS: true,
	}, HostnameMDNSStatus{
		Active: true,
		IPs:    []string{"192.168.1.6"},
	}, HostnameDNSProxyStatus{})

	if len(status.DirectURLs) != 1 || status.DirectURLs[0] != "http://192.168.1.6:8080" {
		t.Fatalf("unexpected direct urls: %#v", status.DirectURLs)
	}
	if len(status.MDNSURLs) != 1 || status.MDNSURLs[0] != "http://edgeCore.local:8080" {
		t.Fatalf("unexpected mdns urls: %#v", status.MDNSURLs)
	}
}
