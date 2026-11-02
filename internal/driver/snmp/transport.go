package snmp

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gosnmp/gosnmp"
)

type SNMPTransport struct {
	cfg                deviceConfig
	client             *gosnmp.GoSNMP
	connected          atomic.Bool
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string

	getHook  func(oids []string, community string) ([]gosnmp.SnmpPDU, error)
	setHook  func(oid string, value interface{}, asnType gosnmp.Asn1BER, community string) error
	walkHook func(rootOID string, community string, walkFn func(pdu gosnmp.SnmpPDU) error) error
}

func NewSNMPTransport(cfg map[string]any) *SNMPTransport {
	return &SNMPTransport{cfg: parseDeviceConfig(cfg)}
}

func (t *SNMPTransport) SetConfig(cfg deviceConfig) {
	t.cfg = cfg
}

func (t *SNMPTransport) Connect(ctx context.Context) error {
	client, err := t.buildClient()
	if err != nil {
		return err
	}
	if err := client.Connect(); err != nil {
		return fmt.Errorf("snmp connect failed: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, t.cfg.Timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := client.Get([]string{oidSysDescr})
		done <- err
	}()
	select {
	case <-probeCtx.Done():
		_ = client.Conn.Close()
		return fmt.Errorf("snmp probe timeout: %w", probeCtx.Err())
	case err := <-done:
		if err != nil {
			_ = client.Conn.Close()
			return fmt.Errorf("snmp probe failed: %w", err)
		}
	}

	t.client = client
	t.connected.Store(true)
	t.connectTime = time.Now()
	if client.Conn != nil {
		t.localAddr = client.Conn.LocalAddr().String()
	}
	return nil
}

func (t *SNMPTransport) Disconnect() error {
	t.connected.Store(false)
	t.lastDisconnectTime = time.Now()
	if t.client != nil && t.client.Conn != nil {
		err := t.client.Conn.Close()
		t.client = nil
		return err
	}
	return nil
}

func (t *SNMPTransport) IsConnected() bool {
	return t.connected.Load() && t.client != nil
}

func (t *SNMPTransport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if !t.connectTime.IsZero() && t.connected.Load() {
		connectionSeconds = int64(time.Since(t.connectTime).Seconds())
	}
	return connectionSeconds, int64(t.reconnectCount.Load()), t.localAddr, t.cfg.remoteAddr(), t.lastDisconnectTime
}

func (t *SNMPTransport) Get(oids []string, community string) ([]gosnmp.SnmpPDU, error) {
	if t.getHook != nil {
		return t.getHook(oids, community)
	}
	if !t.IsConnected() {
		return nil, fmt.Errorf("snmp not connected")
	}
	if !t.cfg.isV3() && community != "" {
		t.client.Community = community
	}
	result, err := t.client.Get(oids)
	if err != nil {
		return nil, err
	}
	return result.Variables, nil
}

func (t *SNMPTransport) GetBulk(baseOID string, community string, maxRepetitions int) ([]gosnmp.SnmpPDU, error) {
	if !t.IsConnected() {
		return nil, fmt.Errorf("snmp not connected")
	}
	if maxRepetitions <= 0 {
		maxRepetitions = t.cfg.MaxBulkSize
	}
	if !t.cfg.isV3() && community != "" {
		t.client.Community = community
	}
	result, err := t.client.GetBulk([]string{baseOID}, 0, uint32(maxRepetitions&0xFFFFFFFF))
	if err != nil {
		return nil, err
	}
	return result.Variables, nil
}

func (t *SNMPTransport) GetNext(oid string, community string) (*gosnmp.SnmpPDU, error) {
	if !t.IsConnected() {
		return nil, fmt.Errorf("snmp not connected")
	}
	if !t.cfg.isV3() && community != "" {
		t.client.Community = community
	}
	result, err := t.client.GetNext([]string{oid})
	if err != nil {
		return nil, err
	}
	if len(result.Variables) == 0 {
		return nil, fmt.Errorf("empty GETNEXT response for %s", oid)
	}
	return &result.Variables[0], nil
}

func (t *SNMPTransport) Walk(rootOID string, community string, walkFn func(pdu gosnmp.SnmpPDU) error) error {
	if t.walkHook != nil {
		return t.walkHook(rootOID, community, walkFn)
	}
	if !t.IsConnected() {
		return fmt.Errorf("snmp not connected")
	}
	if !t.cfg.isV3() && community != "" {
		t.client.Community = community
	}
	return t.client.Walk(rootOID, walkFn)
}

func (t *SNMPTransport) Set(oid string, value interface{}, asnType gosnmp.Asn1BER, community string) error {
	if t.setHook != nil {
		return t.setHook(oid, value, asnType, community)
	}
	if !t.IsConnected() {
		return fmt.Errorf("snmp not connected")
	}
	if !t.cfg.isV3() && community != "" {
		t.client.Community = community
	}
	_, err := t.client.Set([]gosnmp.SnmpPDU{{
		Name:  oid,
		Type:  asnType,
		Value: value,
	}})
	return err
}

func (t *SNMPTransport) buildClient() (*gosnmp.GoSNMP, error) {
	client := &gosnmp.GoSNMP{
		Target:    t.cfg.TargetIP,
		Port:      uint16(t.cfg.TargetPort & 0xFFFF),
		Timeout:   t.cfg.Timeout,
		Retries:   t.cfg.Retries,
		MaxOids:   gosnmp.MaxOids,
		Community: t.cfg.Community,
	}

	if t.cfg.isV3() {
		client.Version = gosnmp.Version3
		client.ContextName = t.cfg.ContextName
		client.ContextEngineID = t.cfg.ContextEngineID
		sec, err := buildV3Security(t.cfg)
		if err != nil {
			return nil, err
		}
		client.SecurityModel = gosnmp.UserSecurityModel
		client.MsgFlags = sec.msgFlags
		client.SecurityParameters = sec.params
	} else {
		client.Version = gosnmp.Version2c
	}

	return client, nil
}

type v3Security struct {
	params   *gosnmp.UsmSecurityParameters
	msgFlags gosnmp.SnmpV3MsgFlags
}

func buildV3Security(cfg deviceConfig) (*v3Security, error) {
	if cfg.SecurityName == "" {
		return nil, fmt.Errorf("snmp v3 requires securityName")
	}

	params := &gosnmp.UsmSecurityParameters{
		UserName: cfg.SecurityName,
	}

	level := strings.ToLower(strings.TrimSpace(cfg.SecurityLevel))
	switch level {
	case "noauthnopriv", "":
		return &v3Security{params: params, msgFlags: gosnmp.NoAuthNoPriv}, nil
	case "authnopriv":
		authProto, err := mapAuthProtocol(cfg.AuthProtocol)
		if err != nil {
			return nil, err
		}
		if cfg.AuthPassword == "" {
			return nil, fmt.Errorf("authPassword required for authNoPriv")
		}
		params.AuthenticationProtocol = authProto
		params.AuthenticationPassphrase = cfg.AuthPassword
		return &v3Security{params: params, msgFlags: gosnmp.AuthNoPriv}, nil
	case "authpriv":
		authProto, err := mapAuthProtocol(cfg.AuthProtocol)
		if err != nil {
			return nil, err
		}
		privProto, err := mapPrivProtocol(cfg.PrivProtocol)
		if err != nil {
			return nil, err
		}
		if cfg.AuthPassword == "" {
			return nil, fmt.Errorf("authPassword required for authPriv")
		}
		if cfg.PrivPassword == "" {
			return nil, fmt.Errorf("privPassword required for authPriv")
		}
		params.AuthenticationProtocol = authProto
		params.AuthenticationPassphrase = cfg.AuthPassword
		params.PrivacyProtocol = privProto
		params.PrivacyPassphrase = cfg.PrivPassword
		return &v3Security{params: params, msgFlags: gosnmp.AuthPriv}, nil
	default:
		return nil, fmt.Errorf("unsupported securityLevel %q", cfg.SecurityLevel)
	}
}

func mapAuthProtocol(name string) (gosnmp.SnmpV3AuthProtocol, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "MD5":
		return gosnmp.MD5, nil
	case "SHA", "SHA1":
		return gosnmp.SHA, nil
	case "SHA224":
		return gosnmp.SHA224, nil
	case "SHA256":
		return gosnmp.SHA256, nil
	case "SHA384":
		return gosnmp.SHA384, nil
	case "SHA512":
		return gosnmp.SHA512, nil
	default:
		return gosnmp.NoAuth, fmt.Errorf("unsupported authProtocol %q", name)
	}
}

func mapPrivProtocol(name string) (gosnmp.SnmpV3PrivProtocol, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DES":
		return gosnmp.DES, nil
	case "AES", "AES128":
		return gosnmp.AES, nil
	case "AES192":
		return gosnmp.AES192, nil
	case "AES256":
		return gosnmp.AES256, nil
	default:
		return gosnmp.NoPriv, fmt.Errorf("unsupported privProtocol %q", name)
	}
}
