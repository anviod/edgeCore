package network

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	mdnsMulticastRepetitions = 2
	mdnsClassCacheFlush      = 1 << 15
)

var (
	mdnsGroupIPv4 = net.IPv4(224, 0, 0, 251)
	mdnsGroupIPv6 = net.ParseIP("ff02::fb")
	mdnsAddrIPv4  = &net.UDPAddr{IP: mdnsGroupIPv4, Port: 5353}
	mdnsAddrIPv6  = &net.UDPAddr{IP: mdnsGroupIPv6, Port: 5353}
)

type mdnsServiceDef struct {
	Instance string
	Type     string
	Port     int
	TXT      []string
}

type mdnsServiceRecord struct {
	mdnsServiceDef
	domain              string
	serviceName         string
	serviceInstanceName string
	serviceTypeName     string
}

func newMDNSServiceRecord(instance, serviceType, domain string) mdnsServiceRecord {
	domain = trimMDNSDot(domain)
	if domain == "" {
		domain = "local"
	}
	serviceName := fmt.Sprintf("%s.%s.", trimMDNSDot(serviceType), domain)
	rec := mdnsServiceRecord{
		mdnsServiceDef:  mdnsServiceDef{Instance: instance, Type: serviceType},
		domain:          domain,
		serviceName:     serviceName,
		serviceTypeName: fmt.Sprintf("_services._dns-sd._udp.%s.", domain),
	}
	if instance != "" {
		rec.serviceInstanceName = fmt.Sprintf("%s.%s", trimMDNSDot(instance), serviceName)
	}
	return rec
}

type mdnsResponder struct {
	hostName string
	ips      []net.IP
	services []mdnsServiceRecord

	ipv4conn *ipv4.PacketConn
	ipv6conn *ipv6.PacketConn
	ifaces   []net.Interface

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func newMDNSResponder(hostName string, ips []string, services []mdnsServiceDef, ifaces []net.Interface) (*mdnsResponder, error) {
	if hostName == "" {
		return nil, fmt.Errorf("missing mDNS host name")
	}
	if !strings.HasSuffix(hostName, ".") {
		hostName += "."
	}
	if !strings.HasSuffix(strings.TrimSuffix(hostName, "."), ".local") {
		hostName = fmt.Sprintf("%s.local.", trimMDNSDot(hostName))
	}

	parsed := make([]net.IP, 0, len(ips))
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil || ip.To4() == nil {
			continue
		}
		parsed = append(parsed, ip)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("no IPv4 addresses for mDNS")
	}

	if len(ifaces) == 0 {
		ifaces = listMDNSMulticastInterfaces()
	}
	if len(ifaces) == 0 {
		return nil, fmt.Errorf("no multicast interfaces for mDNS")
	}

	records := make([]mdnsServiceRecord, 0, len(services))
	for _, svc := range services {
		rec := newMDNSServiceRecord(svc.Instance, svc.Type, "local.")
		rec.Port = svc.Port
		rec.TXT = svc.TXT
		if rec.Port == 0 {
			return nil, fmt.Errorf("missing port for mDNS service %s", svc.Type)
		}
		records = append(records, rec)
	}

	ipv4conn, err4 := joinMDNSIPv4(ifaces)
	ipv6conn, err6 := joinMDNSIPv6(ifaces)
	if err4 != nil && err6 != nil {
		return nil, fmt.Errorf("no suitable mDNS interface: %v / %v", err4, err6)
	}

	r := &mdnsResponder{
		hostName: hostName,
		ips:      parsed,
		services: records,
		ipv4conn: ipv4conn,
		ipv6conn: ipv6conn,
		ifaces:   ifaces,
		stopCh:   make(chan struct{}),
	}

	r.wg.Add(1)
	go r.recvLoop(r.ipv4conn, r.recv4)
	r.wg.Add(1)
	go r.recvLoop(r.ipv6conn, r.recv6)
	r.wg.Add(1)
	go r.announceLoop()

	return r, nil
}

func (r *mdnsResponder) Shutdown() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
		if r.ipv4conn != nil {
			_ = r.ipv4conn.Close()
		}
		if r.ipv6conn != nil {
			_ = r.ipv6conn.Close()
		}
	})
	r.wg.Wait()
}

func (r *mdnsResponder) recvLoop(conn interface{ Close() error }, fn func()) {
	defer r.wg.Done()
	if conn == nil {
		return
	}
	fn()
}

func (r *mdnsResponder) recv4() {
	if r.ipv4conn == nil {
		return
	}
	buf := make([]byte, 65536)
	for {
		select {
		case <-r.stopCh:
			return
		default:
		}
		n, cm, _, err := r.ipv4conn.ReadFrom(buf)
		if err != nil {
			return
		}
		ifIndex := 0
		if cm != nil {
			ifIndex = cm.IfIndex
		}
		_ = r.handlePacket(buf[:n], ifIndex)
	}
}

func (r *mdnsResponder) recv6() {
	if r.ipv6conn == nil {
		return
	}
	buf := make([]byte, 65536)
	for {
		select {
		case <-r.stopCh:
			return
		default:
		}
		n, cm, _, err := r.ipv6conn.ReadFrom(buf)
		if err != nil {
			return
		}
		ifIndex := 0
		if cm != nil {
			ifIndex = cm.IfIndex
		}
		_ = r.handlePacket(buf[:n], ifIndex)
	}
}

func (r *mdnsResponder) handlePacket(packet []byte, ifIndex int) error {
	var msg dns.Msg
	if err := msg.Unpack(packet); err != nil {
		return err
	}
	if len(msg.Ns) > 0 || msg.Opcode != dns.OpcodeQuery {
		return nil
	}

	for _, q := range msg.Question {
		resp := new(dns.Msg)
		resp.SetReply(&msg)
		resp.Compress = true
		resp.Authoritative = true
		resp.Question = nil

		if !r.handleQuestion(q, resp, ifIndex) {
			continue
		}
		if len(resp.Answer) == 0 && len(resp.Extra) == 0 {
			continue
		}

		if isMDNSUnicastQuestion(q) {
			_ = r.unicastResponse(resp, ifIndex, &msg)
		} else {
			_ = r.multicastResponse(resp, ifIndex)
		}
	}
	return nil
}

func (r *mdnsResponder) handleQuestion(q dns.Question, resp *dns.Msg, ifIndex int) bool {
	qName := strings.ToLower(q.Name)
	hostName := strings.ToLower(r.hostName)

	if qName == hostName {
		switch q.Qtype {
		case dns.TypeA:
			resp.Answer = r.appendARecords(resp.Answer, 120, false)
			return len(resp.Answer) > 0
		case dns.TypeAAAA:
			return false
		}
	}

	for _, svc := range r.services {
		switch qName {
		case strings.ToLower(svc.serviceTypeName):
			r.appendServiceType(resp, svc, 3200)
			return len(resp.Answer) > 0
		case strings.ToLower(svc.serviceName):
			r.appendBrowsingAnswers(resp, svc, ifIndex, 3200)
			return len(resp.Answer) > 0 || len(resp.Extra) > 0
		case strings.ToLower(svc.serviceInstanceName):
			r.appendLookupAnswers(resp, svc, ifIndex, 3200, false)
			return len(resp.Answer) > 0
		}
	}
	return false
}

func (r *mdnsResponder) appendServiceType(resp *dns.Msg, svc mdnsServiceRecord, ttl uint32) {
	resp.Answer = append(resp.Answer, &dns.PTR{
		Hdr: dns.RR_Header{Name: svc.serviceTypeName, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl},
		Ptr: svc.serviceName,
	})
}

func (r *mdnsResponder) appendBrowsingAnswers(resp *dns.Msg, svc mdnsServiceRecord, ifIndex int, ttl uint32) {
	resp.Answer = append(resp.Answer, &dns.PTR{
		Hdr: dns.RR_Header{Name: svc.serviceName, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl},
		Ptr: svc.serviceInstanceName,
	})
	resp.Extra = append(resp.Extra, r.serviceRecords(svc, ttl, false)...)
	resp.Extra = r.appendARecords(resp.Extra, ttl, false)
}

func (r *mdnsResponder) appendLookupAnswers(resp *dns.Msg, svc mdnsServiceRecord, ifIndex int, ttl uint32, flush bool) {
	resp.Answer = append(resp.Answer, r.serviceRecords(svc, ttl, flush)...)
	resp.Answer = append(resp.Answer, &dns.PTR{
		Hdr: dns.RR_Header{Name: svc.serviceName, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl},
		Ptr: svc.serviceInstanceName,
	})
	resp.Answer = append(resp.Answer, &dns.PTR{
		Hdr: dns.RR_Header{Name: svc.serviceTypeName, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl},
		Ptr: svc.serviceName,
	})
	resp.Answer = r.appendARecords(resp.Answer, ttl, flush)
}

func (r *mdnsResponder) serviceRecords(svc mdnsServiceRecord, ttl uint32, flush bool) []dns.RR {
	class := uint16(dns.ClassINET)
	if flush {
		class |= mdnsClassCacheFlush
	}
	return []dns.RR{
		&dns.SRV{
			Hdr:  dns.RR_Header{Name: svc.serviceInstanceName, Rrtype: dns.TypeSRV, Class: class, Ttl: ttl},
			Port: uint16(svc.Port), Target: r.hostName,
		},
		&dns.TXT{
			Hdr: dns.RR_Header{Name: svc.serviceInstanceName, Rrtype: dns.TypeTXT, Class: class, Ttl: ttl},
			Txt: svc.TXT,
		},
	}
}

func (r *mdnsResponder) appendARecords(list []dns.RR, ttl uint32, flush bool) []dns.RR {
	class := uint16(dns.ClassINET)
	if flush {
		class |= mdnsClassCacheFlush
	}
	for _, ip := range r.ips {
		if v4 := ip.To4(); v4 != nil {
			list = append(list, &dns.A{
				Hdr: dns.RR_Header{Name: r.hostName, Rrtype: dns.TypeA, Class: class, Ttl: ttl},
				A:   v4,
			})
		}
	}
	return list
}

func (r *mdnsResponder) announceLoop() {
	defer r.wg.Done()

	// Hostname A record announcements (critical for edgeCore.local resolution).
	for i := 0; i < mdnsMulticastRepetitions; i++ {
		resp := new(dns.Msg)
		resp.MsgHdr.Response = true
		resp.Authoritative = true
		resp.Answer = r.appendARecords(nil, 120, true)
		_ = r.multicastResponse(resp, 0)
		time.Sleep(time.Duration(rand.IntN(250)) * time.Millisecond)
	}

	for _, svc := range r.services {
		q := new(dns.Msg)
		q.SetQuestion(svc.serviceInstanceName, dns.TypePTR)
		q.Ns = r.serviceRecords(svc, 3200, false)
		for i := 0; i < mdnsMulticastRepetitions; i++ {
			_ = r.multicastResponse(q, 0)
			time.Sleep(time.Duration(rand.IntN(250)) * time.Millisecond)
		}
	}

	timeout := time.Second
	for i := 0; i < mdnsMulticastRepetitions; i++ {
		select {
		case <-r.stopCh:
			return
		default:
		}
		for _, iface := range r.ifaces {
			hostResp := new(dns.Msg)
			hostResp.MsgHdr.Response = true
			hostResp.Authoritative = true
			hostResp.Answer = r.appendARecords(nil, 120, true)
			_ = r.multicastResponseOnIface(hostResp, iface.Index)

			for _, svc := range r.services {
				resp := new(dns.Msg)
				resp.MsgHdr.Response = true
				resp.Authoritative = true
				r.appendLookupAnswers(resp, svc, iface.Index, 3200, true)
				_ = r.multicastResponseOnIface(resp, iface.Index)
			}
		}
		select {
		case <-r.stopCh:
			return
		case <-time.After(timeout):
		}
		timeout *= 2
	}
}

func (r *mdnsResponder) unicastResponse(resp *dns.Msg, ifIndex int, query *dns.Msg) error {
	buf, err := resp.Pack()
	if err != nil {
		return err
	}
	// Best-effort; mDNS browsers typically use multicast queries.
	_ = ifIndex
	_ = query
	_ = buf
	return nil
}

func (r *mdnsResponder) multicastResponse(resp *dns.Msg, ifIndex int) error {
	if ifIndex != 0 {
		return r.multicastResponseOnIface(resp, ifIndex)
	}
	for _, iface := range r.ifaces {
		_ = r.multicastResponseOnIface(resp, iface.Index)
	}
	return nil
}

func (r *mdnsResponder) multicastResponseOnIface(resp *dns.Msg, ifIndex int) error {
	buf, err := resp.Pack()
	if err != nil {
		return err
	}
	if r.ipv4conn != nil {
		wcm := ipv4.ControlMessage{IfIndex: ifIndex}
		if _, err := r.ipv4conn.WriteTo(buf, &wcm, mdnsAddrIPv4); err != nil {
			log.Printf("mDNS IPv4 multicast failed on ifindex %d: %v", ifIndex, err)
		}
	}
	if r.ipv6conn != nil {
		wcm := ipv6.ControlMessage{IfIndex: ifIndex}
		if _, err := r.ipv6conn.WriteTo(buf, &wcm, mdnsAddrIPv6); err != nil {
			log.Printf("mDNS IPv6 multicast failed on ifindex %d: %v", ifIndex, err)
		}
	}
	return nil
}

func joinMDNSIPv4(interfaces []net.Interface) (*ipv4.PacketConn, error) {
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("224.0.0.0"), Port: 5353})
	if err != nil {
		return nil, err
	}
	pkConn := ipv4.NewPacketConn(udpConn)
	_ = pkConn.SetControlMessage(ipv4.FlagInterface, true)

	var failed int
	for _, iface := range interfaces {
		if err := pkConn.JoinGroup(&iface, &net.UDPAddr{IP: mdnsGroupIPv4}); err != nil {
			failed++
		}
	}
	if failed == len(interfaces) {
		_ = pkConn.Close()
		return nil, fmt.Errorf("failed to join IPv4 multicast on any interface")
	}
	return pkConn, nil
}

func joinMDNSIPv6(interfaces []net.Interface) (*ipv6.PacketConn, error) {
	udpConn, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.ParseIP("ff02::"), Port: 5353})
	if err != nil {
		return nil, err
	}
	pkConn := ipv6.NewPacketConn(udpConn)
	_ = pkConn.SetControlMessage(ipv6.FlagInterface, true)

	var failed int
	for _, iface := range interfaces {
		if err := pkConn.JoinGroup(&iface, &net.UDPAddr{IP: mdnsGroupIPv6}); err != nil {
			failed++
		}
	}
	if failed == len(interfaces) {
		_ = pkConn.Close()
		return nil, fmt.Errorf("failed to join IPv6 multicast on any interface")
	}
	return pkConn, nil
}

func listMDNSMulticastInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.Interface
	for _, iface := range ifaces {
		if !isUsableMDNSInterface(iface.Name, iface.Flags) {
			continue
		}
		if iface.Flags&net.FlagMulticast != 0 {
			out = append(out, iface)
		}
	}
	return out
}

func trimMDNSDot(s string) string {
	return strings.Trim(s, ".")
}

func isMDNSUnicastQuestion(q dns.Question) bool {
	return q.Qclass&mdnsClassCacheFlush != 0
}
