package network

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"syscall"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/miekg/dns"
)

// DNSProxy handles DNS requests for bare hostname access on port 53.
// Clients must use this device as their DNS server for bare hostnames to resolve.
type DNSProxy struct {
	serverUDP *dns.Server
	serverTCP *dns.Server
	mu        sync.Mutex
	config    model.HostnameConfig
	ips       []net.IP
	status    HostnameDNSProxyStatus
}

// HostnameDNSProxyStatus reports DNS proxy state for the UI/API.
type HostnameDNSProxyStatus struct {
	Enabled  bool     `json:"enabled"`
	Active   bool     `json:"active"`
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips,omitempty"`
	Error    string   `json:"error,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// NewDNSProxy creates a new DNSProxy.
func NewDNSProxy() *DNSProxy {
	return &DNSProxy{}
}

// Status returns the latest DNS proxy status.
func (d *DNSProxy) Status() HostnameDNSProxyStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

// Start starts the DNS proxy.
func (d *DNSProxy) Start(cfg model.HostnameConfig) error {
	d.Stop()
	d.mu.Lock()
	defer d.mu.Unlock()

	d.status = HostnameDNSProxyStatus{
		Enabled:  cfg.EnableBare,
		Hostname: cfg.Name,
		Note:     "Bare hostname requires client DNS to point at this device IP (port 53). Prefer http://<name>.local:<port> via mDNS.",
	}

	if !cfg.EnableBare {
		return nil
	}

	if cfg.Name == "" {
		cfg.Name = "edgeCore"
		d.status.Hostname = cfg.Name
	}

	d.config = cfg
	d.ips = nil

	ifaces, err := net.Interfaces()
	if err != nil {
		d.status.Error = err.Error()
		return err
	}

	for _, iface := range ifaces {
		validIface := len(cfg.Interfaces) == 0
		if !validIface {
			for _, name := range cfg.Interfaces {
				if iface.Name == name {
					validIface = true
					break
				}
			}
		}
		if !validIface || !isUsableMDNSInterface(iface.Name, iface.Flags) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				d.ips = append(d.ips, ip)
				d.status.IPs = append(d.status.IPs, ip.String())
			}
		}
	}

	if len(d.ips) == 0 {
		err := fmt.Errorf("no IPv4 addresses available for DNS proxy")
		d.status.Error = err.Error()
		return err
	}

	if err := probeDNSPort(); err != nil {
		d.status.Error = err.Error()
		if errors.Is(err, syscall.EADDRINUSE) {
			d.status.Note = "Port 53 is already in use (common on macOS). Bare hostname access is unavailable; use http://" + cfg.Name + ".local or http://<device-ip> instead."
		}
		log.Printf("DNS proxy unavailable for hostname %s: %v", cfg.Name, err)
		return err
	}

	d.serverUDP = &dns.Server{Addr: ":53", Net: "udp", Handler: d, ReusePort: true}
	d.serverTCP = &dns.Server{Addr: ":53", Net: "tcp", Handler: d, ReusePort: true}

	go func() {
		if err := d.serverUDP.ListenAndServe(); err != nil {
			log.Printf("DNS proxy UDP stopped: %v", err)
		}
	}()
	go func() {
		if err := d.serverTCP.ListenAndServe(); err != nil {
			log.Printf("DNS proxy TCP stopped: %v", err)
		}
	}()

	d.status.Active = true
	log.Printf("DNS proxy started for hostname: %s", cfg.Name)
	return nil
}

func probeDNSPort() error {
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 53})
	if err != nil {
		return err
	}
	_ = udpConn.Close()

	tcpConn, err := net.Listen("tcp", ":53")
	if err != nil {
		return err
	}
	_ = tcpConn.Close()
	return nil
}

// Stop stops the DNS proxy.
func (d *DNSProxy) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.serverUDP != nil {
		_ = d.serverUDP.Shutdown()
		d.serverUDP = nil
	}
	if d.serverTCP != nil {
		_ = d.serverTCP.Shutdown()
		d.serverTCP = nil
	}
	if d.status.Active {
		d.status.Active = false
	}
}

// ServeDNS implements the dns.Handler interface.
func (d *DNSProxy) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	switch r.Opcode {
	case dns.OpcodeQuery:
		for _, q := range m.Question {
			name := strings.TrimSuffix(strings.ToLower(q.Name), ".")
			host := strings.ToLower(d.config.Name)
			if name != host && name != host+".local" {
				m.Rcode = dns.RcodeNameError
				continue
			}
			if q.Qtype == dns.TypeA {
				for _, ip := range d.ips {
					rr := &dns.A{
						Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
						A:   ip,
					}
					m.Answer = append(m.Answer, rr)
				}
			}
		}
	}

	_ = w.WriteMsg(m)
}
