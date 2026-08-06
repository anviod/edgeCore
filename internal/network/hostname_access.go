package network

import (
	"fmt"

	"github.com/anviod/edgeCore/internal/model"
)

// HostnameAccessStatus summarizes how the gateway can be reached on the LAN.
type HostnameAccessStatus struct {
	Name       string                 `json:"name"`
	HTTPPort   int                    `json:"http_port"`
	HTTPSPort  int                    `json:"https_port"`
	MDNS       HostnameMDNSStatus     `json:"mdns"`
	DNSProxy   HostnameDNSProxyStatus `json:"dns_proxy"`
	IPs        []string               `json:"ips"`
	DirectURLs []string               `json:"direct_urls"`
	MDNSURLs   []string               `json:"mdns_urls"`
}

// BuildHostnameAccessStatus composes access URLs and service status for the UI/API.
func BuildHostnameAccessStatus(cfg model.HostnameConfig, mdns HostnameMDNSStatus, dnsProxy HostnameDNSProxyStatus) HostnameAccessStatus {
	name := cfg.Name
	if name == "" {
		name = "edgeCore"
	}
	httpPort := cfg.HTTPPort
	if httpPort == 0 {
		httpPort = 8080
	}
	httpsPort := cfg.HTTPSPort
	if httpsPort == 0 {
		httpsPort = 443
	}

	ips := mdns.IPs
	if len(ips) == 0 {
		ips = dnsProxy.IPs
	}

	status := HostnameAccessStatus{
		Name:      name,
		HTTPPort:  httpPort,
		HTTPSPort: httpsPort,
		MDNS:      mdns,
		DNSProxy:  dnsProxy,
		IPs:       ips,
	}

	if mdns.Active {
		status.MDNSURLs = append(status.MDNSURLs, fmt.Sprintf("http://%s.local:%d", name, httpPort))
	}
	for _, ip := range ips {
		status.DirectURLs = append(status.DirectURLs, fmt.Sprintf("http://%s:%d", ip, httpPort))
	}

	return status
}
