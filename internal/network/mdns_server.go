package network

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

// MDNSServer manages mDNS services.
type MDNSServer struct {
	responder *mdnsResponder
	status    HostnameMDNSStatus
	mu        sync.Mutex
}

// HostnameMDNSStatus reports mDNS broadcast state for the UI/API.
type HostnameMDNSStatus struct {
	Enabled  bool     `json:"enabled"`
	Active   bool     `json:"active"`
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// NewMDNSServer creates a new MDNSServer.
func NewMDNSServer() *MDNSServer {
	return &MDNSServer{}
}

// Status returns the latest mDNS broadcast status.
func (s *MDNSServer) Status() HostnameMDNSStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func isUsableMDNSInterface(name string, flags net.Flags) bool {
	if isLoopbackInterface(name, flags) {
		return false
	}
	if flags&net.FlagUp == 0 {
		return false
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "utun"),
		strings.HasPrefix(lower, "awdl"),
		strings.HasPrefix(lower, "llw"),
		strings.HasPrefix(lower, "gif"),
		strings.HasPrefix(lower, "stf"):
		return false
	}
	return true
}

// collectIPv4Addresses returns IPv4 addresses and the interfaces they came from.
// When names is empty, all usable non-loopback up interfaces are scanned.
func collectIPv4Addresses(names []string) ([]string, []net.Interface, error) {
	var selected []net.Interface
	if len(names) > 0 {
		for _, name := range names {
			iface, err := net.InterfaceByName(name)
			if err != nil {
				log.Printf("Warning: Interface %s not found for mDNS", name)
				continue
			}
			selected = append(selected, *iface)
		}
		if len(selected) == 0 {
			return nil, nil, fmt.Errorf("no valid interfaces found for mDNS")
		}
	} else {
		allIfaces, err := net.Interfaces()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list interfaces: %w", err)
		}
		selected = allIfaces
	}

	seenIPs := make(map[string]struct{})
	var ips []string
	var usedIfaces []net.Interface

	for _, iface := range selected {
		if !isUsableMDNSInterface(iface.Name, iface.Flags) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		var ifaceIPs []string
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			ipStr := ip.String()
			ifaceIPs = append(ifaceIPs, ipStr)
		}
		if len(ifaceIPs) == 0 {
			continue
		}
		usedIfaces = append(usedIfaces, iface)
		for _, ipStr := range ifaceIPs {
			if _, ok := seenIPs[ipStr]; ok {
				continue
			}
			seenIPs[ipStr] = struct{}{}
			ips = append(ips, ipStr)
		}
	}

	if len(ips) == 0 {
		return nil, usedIfaces, fmt.Errorf("no IPv4 addresses available for mDNS broadcast")
	}

	if len(usedIfaces) == 0 {
		usedIfaces = listMDNSMulticastInterfaces()
	}

	return ips, usedIfaces, nil
}

// Start starts the mDNS services based on configuration.
func (s *MDNSServer) Start(cfg model.HostnameConfig) error {
	s.Stop()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = HostnameMDNSStatus{
		Enabled:  cfg.EnableMDNS,
		Hostname: cfg.Name,
	}

	if !cfg.EnableMDNS {
		return nil
	}

	if cfg.Name == "" {
		cfg.Name = "edgeCore"
		s.status.Hostname = cfg.Name
	}

	ips, ifaces, err := collectIPv4Addresses(cfg.Interfaces)
	if err != nil {
		s.status.Error = err.Error()
		log.Printf("Warning: %v", err)
		return err
	}
	s.status.IPs = append([]string(nil), ips...)

	hostName := fmt.Sprintf("%s.local.", cfg.Name)
	services := []mdnsServiceDef{}

	if cfg.HTTPPort > 0 {
		services = append(services, mdnsServiceDef{
			Instance: cfg.Name,
			Type:     "_http._tcp",
			Port:     cfg.HTTPPort,
			TXT:      []string{"path=/"},
		})
	}

	gwPort := cfg.HTTPPort
	if gwPort == 0 {
		gwPort = 8080
	}
	services = append(services, mdnsServiceDef{
		Instance: cfg.Name,
		Type:     "_gateway._tcp",
		Port:     gwPort,
		TXT:      []string{"model=edgeCore", "version=1.0"},
	})

	responder, err := newMDNSResponder(hostName, ips, services, ifaces)
	if err != nil {
		s.status.Error = err.Error()
		return fmt.Errorf("failed to start mDNS: %w", err)
	}

	s.responder = responder
	s.status.Active = true
	log.Printf("mDNS services started for hostname: %s (%v)", cfg.Name, ips)
	return nil
}

// Stop stops all mDNS services.
func (s *MDNSServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.responder != nil {
		s.responder.Shutdown()
		s.responder = nil
	}
	if s.status.Active {
		s.status.Active = false
	}
}
