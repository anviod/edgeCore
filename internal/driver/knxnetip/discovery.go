package knxnetip

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// GatewayInfo describes a KNX IP interface discovered via SEARCH_RESPONSE.
type GatewayInfo struct {
	IP   string
	Port int
}

// DiscoverGateways sends SEARCH_REQUEST and collects SEARCH_RESPONSE results until timeout.
func DiscoverGateways(ctx context.Context, cfg transportConfig) ([]GatewayInfo, error) {
	multicastAddr, err := net.ResolveUDPAddr("udp4", cfg.discoveryMulticast)
	if err != nil {
		return nil, fmt.Errorf("invalid discovery multicast: %w", err)
	}

	var lc net.ListenConfig
	var conn *net.UDPConn
	if cfg.localIP != "" {
		laddr, err := net.ResolveUDPAddr("udp4", cfg.localIP+":0")
		if err != nil {
			return nil, err
		}
		pc, err := lc.ListenPacket(ctx, "udp4", laddr.String())
		if err != nil {
			return nil, err
		}
		conn = pc.(*net.UDPConn)
	} else {
		pc, err := lc.ListenPacket(ctx, "udp4", ":0")
		if err != nil {
			return nil, err
		}
		conn = pc.(*net.UDPConn)
	}
	defer conn.Close()

	discovery := localHPAIFromConn(conn, hostProtocolIPv4UDP)
	req := buildSearchRequest(discovery)
	if _, err := conn.WriteToUDP(req, multicastAddr); err != nil {
		return nil, fmt.Errorf("SEARCH_REQUEST send failed: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(cfg.discoveryTimeout)
	}
	_ = conn.SetReadDeadline(deadline)

	seen := make(map[string]GatewayInfo)
	var mu sync.Mutex
	buf := make([]byte, 2048)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			if ctx.Err() != nil {
				break
			}
			return nil, fmt.Errorf("SEARCH_RESPONSE read failed: %w", err)
		}
		if n < headerLen {
			continue
		}
		svc, body, err := parseHeader(buf[:n])
		if err != nil || svc != svcSearchResponse {
			continue
		}
		resp, err := parseSearchResponse(body)
		if err != nil {
			continue
		}
		ip := net.IP(resp.Control.ip[:]).String()
		port := int(resp.Control.port)
		if port == 0 {
			port = defaultPort
		}
		key := fmt.Sprintf("%s:%d", ip, port)
		mu.Lock()
		seen[key] = GatewayInfo{IP: ip, Port: port}
		mu.Unlock()
	}

	gateways := make([]GatewayInfo, 0, len(seen))
	for _, gw := range seen {
		gateways = append(gateways, gw)
	}
	if len(gateways) == 0 {
		return nil, fmt.Errorf("no KNX IP gateways responded to SEARCH")
	}

	zap.L().Info("[KNXnet/IP] gateway discovery complete",
		zap.Int("count", len(gateways)),
		zap.String("first", fmt.Sprintf("%s:%d", gateways[0].IP, gateways[0].Port)),
	)
	return gateways, nil
}

func localHPAIFromConn(conn *net.UDPConn, protocol byte) hpai {
	h := hpai{hostProtocol: protocol}
	if conn == nil || conn.LocalAddr() == nil {
		return h
	}
	host, portStr, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return h
	}
	if ip := net.ParseIP(host); ip != nil {
		copy(h.ip[:], ip.To4())
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	h.port = uint16(port & 0xFFFF)
	return h
}
