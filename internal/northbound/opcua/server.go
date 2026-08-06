package opcua

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/awcullen/opcua/server"
	"github.com/awcullen/opcua/ua"
	"go.uber.org/zap"
)

// VirtualShadowLister supplies virtual shadow device configs for OPC UA address space.
type VirtualShadowLister interface {
	List() []model.VirtualShadowDeviceConfig
}

// Server is the OPC UA Server implementation
type Server struct {
	config           model.OPCUAConfig
	sb               model.SouthboundManager
	virtualShadows   VirtualShadowLister
	srv              *server.Server
	mu               sync.RWMutex
	lifecycleMu      sync.Mutex
	nodeMap          map[string]*server.VariableNode
	virtualDeviceIDs map[string]struct{}
	gatewayID        string
	stats            Stats
	ctx              context.Context
	cancel           context.CancelFunc
	writeHistory     []WriteHistoryItem

	// Node ID mapping system for compact node IDs
	idMapper *NodeIDMapper
}

// NewServer creates a new OPC UA Server
func NewServer(cfg model.OPCUAConfig, sb model.SouthboundManager, virtualShadows VirtualShadowLister) *Server {
	return &Server{
		config:           cfg,
		sb:               sb,
		virtualShadows:   virtualShadows,
		nodeMap:          make(map[string]*server.VariableNode),
		virtualDeviceIDs: make(map[string]struct{}),
		gatewayID:        "Gateway",
		idMapper:         NewNodeIDMapper(),
	}
}

// NodeIDMapper manages the mapping between compact numeric IDs and full string paths
// This reduces OPC UA node ID length significantly:
//
//	Original:  s=Gateway/Channels/44amyf4grh5oquzc/Devices/slave-1/Points/hr_40000 (70+ chars)
//	Compacted: ns=2;s=Channel001.Device001.Temperature
type NodeIDMapper struct {
	mu sync.RWMutex

	// Namespace index for compact node IDs
	namespace uint16

	// Point mappings: channelID:deviceID:pointID -> nodeID string (ns=X;s=Channel.Device.Point)
	pointMap map[string]string

	// Reverse mappings: nodeID -> fullPath
	nodeIDToPath map[string]string

	// folderMap maps folder keys (ch:ID / dev:ch:ID) to numeric folder IDs
	folderMap map[string]uint32

	// Next available folder node ID (starts from 1001 for folders only)
	nextFolderID uint32

	// Reverse mapping: shortID (ns=X;s=Y) -> fullPath
	reverseShortPath map[string]string
}

// NewNodeIDMapper creates a new NodeIDMapper
func NewNodeIDMapper() *NodeIDMapper {
	return &NodeIDMapper{
		namespace:        2, // OPC UA namespace index for custom nodes
		pointMap:         make(map[string]string),
		nodeIDToPath:     make(map[string]string),
		folderMap:        make(map[string]uint32),
		nextFolderID:     1001, // Start from 1001 for folder IDs only
		reverseShortPath: make(map[string]string),
	}
}

// GenerateCompactNodeID generates a compact node ID in OPC UA standard format.
// Format: ns=2;s={channelID}.{deviceID}.{pointID}
// Channel ID is included so NodeIDs stay globally unique across channels.
func (m *NodeIDMapper) GenerateCompactNodeID(channelID, deviceID, pointID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", channelID, deviceID, pointID)
	if nodeID, ok := m.pointMap[key]; ok {
		return nodeID
	}

	fullPath := fmt.Sprintf("Gateway/Channels/%s/Devices/%s/Points/%s", channelID, deviceID, pointID)
	shortID := fmt.Sprintf("ns=%d;s=%s.%s.%s", m.namespace, channelID, deviceID, pointID)

	m.pointMap[key] = shortID
	m.nodeIDToPath[shortID] = fullPath
	m.reverseShortPath[shortID] = fullPath

	return shortID
}

// ParseCompactNodeID parses a compact node ID and returns the full path
// Handles format: ns=2;s={deviceID}.{pointID} or legacy numeric formats
// Returns ("", false) if not a valid compact ID
func (m *NodeIDMapper) ParseCompactNodeID(shortID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for string format: ns=X;s=Y
	if strings.HasPrefix(shortID, "ns=") && strings.Contains(shortID, ";s=") {
		if fullPath, ok := m.reverseShortPath[shortID]; ok {
			return fullPath, true
		}
		return "", false
	}

	// Legacy format (all digits)
	if isCompactNodeID(shortID) {
		if fullPath, ok := m.reverseShortPath[shortID]; ok {
			return fullPath, true
		}
	}

	return "", false
}

// GetOriginalIDs parses a compact node ID and returns the original IDs
// Handles format: ns=2;s={deviceID}.{pointID} or legacy formats
// Returns (channelID, deviceID, pointID, true) or ("", "", "", false) if invalid
func (m *NodeIDMapper) GetOriginalIDs(shortID string) (channelID, deviceID, pointID string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try to parse full path from reverseShortPath first
	if fullPath, exists := m.reverseShortPath[shortID]; exists {
		return parseFullPath(fullPath)
	}

	// Parse string format: ns=X;s={channelID}.{deviceID}.{pointID}
	// Legacy fallback: ns=X;s={deviceID}.{pointID}
	if strings.HasPrefix(shortID, "ns=") && strings.Contains(shortID, ";s=") {
		parts := strings.Split(shortID, ";s=")
		if len(parts) == 2 {
			valuePart := parts[1]
			if segs := strings.Split(valuePart, "."); len(segs) == 3 {
				return segs[0], segs[1], segs[2], true
			}
			if idx := strings.LastIndex(valuePart, "."); idx > 0 {
				deviceID = valuePart[:idx]
				pointID = valuePart[idx+1:]
				for _, fullPath := range m.reverseShortPath {
					if c, d, p, found := parseFullPath(fullPath); found {
						if d == deviceID && p == pointID {
							return c, d, p, true
						}
					}
				}
			}
		}
	}

	// Legacy format: {ch}.{dev}.{pt}
	if strings.Contains(shortID, ".") {
		parts := strings.Split(shortID, ".")
		if len(parts) == 3 {
			// Legacy format also stored in reverseShortPath
			if fullPath, exists := m.reverseShortPath[shortID]; exists {
				return parseFullPath(fullPath)
			}
		}
	}

	// Legacy numeric format (e.g., 111)
	if isCompactNodeID(shortID) {
		if fullPath, exists := m.reverseShortPath[shortID]; exists {
			return parseFullPath(fullPath)
		}
	}

	return "", "", "", false
}

// parseFullPath extracts channelID, deviceID, pointID from full path
func parseFullPath(fullPath string) (channelID, deviceID, pointID string, ok bool) {
	parts := strings.Split(fullPath, "/")
	if len(parts) >= 7 && parts[0] == "Gateway" && parts[1] == "Channels" && parts[3] == "Devices" && parts[5] == "Points" {
		return parts[2], parts[4], parts[6], true
	}
	return "", "", "", false
}

// isCompactNodeID checks if the ID is in compact format (all digits)
func isCompactNodeID(id string) bool {
	if len(id) < 3 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// GenerateCompactFolderID generates compact node ID for folder nodes
// Format: ns=2;i={numericID} for folder types only
// Uses auto-incrementing numeric IDs for folders
func (m *NodeIDMapper) GenerateCompactFolderID(channelID string, deviceID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var key string
	if deviceID == "" {
		// Channel folder
		key = fmt.Sprintf("ch:%s", channelID)
	} else {
		// Device folder
		key = fmt.Sprintf("dev:%s:%s", channelID, deviceID)
	}

	if folderID, ok := m.folderMap[key]; ok {
		return fmt.Sprintf("ns=%d;i=%d", m.namespace, folderID)
	}

	folderID := m.nextFolderID
	m.nextFolderID++
	m.folderMap[key] = folderID

	return fmt.Sprintf("ns=%d;i=%d", m.namespace, folderID)
}

// GetAllMappings returns all current mappings for debugging/serialization
func (m *NodeIDMapper) GetAllMappings() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mappings := make(map[string]any)
	mappings["namespace"] = m.namespace
	mappings["nextFolderID"] = m.nextFolderID
	mappings["points"] = m.pointMap
	mappings["reverseShortPath"] = m.reverseShortPath

	return mappings
}

// Size returns the number of point mappings
func (m *NodeIDMapper) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.pointMap)
}

// Start starts the OPC UA Server
func (s *Server) Start() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.startLocked()
}

func (s *Server) startLocked() error {
	zap.L().Info("Starting OPC UA Server...",
		zap.String("name", s.config.Name),
		zap.Int("port", s.config.Port),
		zap.String("component", "opcua-server"),
	)

	s.ctx, s.cancel = context.WithCancel(context.Background())

	endpoint := fmt.Sprintf("opc.tcp://0.0.0.0:%d%s", s.config.Port, s.config.Endpoint)
	// Sanitize name for URI (remove spaces)
	safeName := strings.ReplaceAll(s.config.Name, " ", "")
	appURI := fmt.Sprintf("urn:edgeCore-gateway:%s", safeName)

	// Resolve server certificate (DB PEM → disk, legacy path, or auto-generated)
	certFile, keyFile, err := MaterializeServerCerts(s.config)
	if err != nil {
		return fmt.Errorf("failed to materialize server certificate: %w", err)
	}
	if certFile == "" {
		if s.config.Name == "Test Server" {
			certFile = "server_test.crt"
			keyFile = "server_test.key"
		} else {
			certFile = "server.crt"
			keyFile = "server.key"
		}
	}

	if err := s.ensureCert(certFile, keyFile, appURI); err != nil {
		return fmt.Errorf("failed to ensure certificate: %v", err)
	}

	trustBaseDir, err := MaterializeTrustedCerts(s.config)
	if err != nil {
		return fmt.Errorf("failed to materialize trusted certificates: %w", err)
	}
	strictClientPKI := trustBaseDir != "" && len(s.config.TrustedCertsPEM) > 0

	appDesc := ua.ApplicationDescription{
		ApplicationURI:  appURI,
		ProductURI:      "http://github.com/awcullen/opcua",
		ApplicationName: ua.LocalizedText{Text: s.config.Name, Locale: "en"},
		ApplicationType: ua.ApplicationTypeServer,
		DiscoveryURLs:   []string{endpoint},
	}

	// Configure User Tokens
	// var userTokens []ua.UserTokenPolicy
	// ... logic to build tokens ...
	// Note: server.WithUserTokenPolicies seems to be unavailable or named differently.
	// We rely on Authenticator functions to implicitly support tokens if applicable.

	// Configure Authenticator & security policies (see security.go)
	opts := appendSecurityOptions(s.config, []server.Option{}, strictClientPKI)

	if trustBaseDir != "" {
		opts = append(opts,
			server.WithTrustedCertificatesPaths(
				filepath.Join(trustBaseDir, "trusted"),
				filepath.Join(trustBaseDir, "crl"),
			),
			server.WithRejectedCertificatesPath(filepath.Join(trustBaseDir, "rejected")),
		)
	}

	s.srv, err = server.New(
		appDesc,
		certFile,
		keyFile,
		endpoint,
		opts...,
	)

	if err != nil {
		return fmt.Errorf("failed to create server: %v", err)
	}

	// Build Address Space
	if err := s.buildAddressSpace(); err != nil {
		return fmt.Errorf("failed to build address space: %v", err)
	}

	// Start Listener
	listener := s.srv
	loopCtx := s.ctx
	go func() {
		if err := listener.ListenAndServe(); err != nil {
			zap.L().Error("OPC UA Server error",
				zap.String("name", s.config.Name),
				zap.Error(err),
				zap.String("component", "opcua-server"),
			)
		}
	}()

	go s.systemInfoLoop(loopCtx)

	zap.L().Info("OPC UA Server started",
		zap.String("name", s.config.Name),
		zap.String("endpoint", endpoint),
		zap.String("component", "opcua-server"),
	)
	return nil
}

func (s *Server) ensureCert(certFile, keyFile, appURI string) error {
	regenerate := false
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			// Check if certificate has correct URI
			certPEM, err := os.ReadFile(certFile)
			if err == nil {
				block, _ := pem.Decode(certPEM)
				if block != nil {
					cert, err := x509.ParseCertificate(block.Bytes)
					if err == nil {
						foundURI := false
						foundCN := false
						for _, u := range cert.URIs {
							if u.String() == appURI {
								foundURI = true
								break
							}
						}

						// Check CommonName
						if cert.Subject.CommonName == s.config.Name {
							foundCN = true
						}

						if !foundURI || !foundCN {
							zap.L().Warn("Existing certificate mismatch (URI or CN), regenerating...",
								zap.String("expected_uri", appURI),
								zap.String("expected_cn", s.config.Name),
								zap.String("component", "opcua-server"),
							)
							regenerate = true
						}
					}
				}
			}
			if !regenerate {
				return nil
			}
		}
	}

	zap.L().Info("Generating self-signed certificate...", zap.String("component", "opcua-server"))

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	uri, err := url.Parse(appURI)
	if err != nil {
		return fmt.Errorf("failed to parse application URI: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"edgeCore Gateway"},
			CommonName:   s.config.Name,
			Country:      []string{"CN"},
			Locality:     []string{"Beijing"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 10 * 24 * time.Hour), // 10 years

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageDataEncipherment | x509.KeyUsageCertSign | x509.KeyUsageContentCommitment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")},
		URIs:                  []*url.URL{uri},
		SignatureAlgorithm:    x509.SHA256WithRSA,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()

	return nil
}

func (s *Server) stopLocked() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

// Stop stops the OPC UA Server. Safe to call multiple times.
func (s *Server) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	wasRunning := s.srv != nil || s.cancel != nil
	s.stopLocked()
	if !wasRunning {
		return
	}
	zap.L().Info("OPC UA Server stopped",
		zap.String("name", s.config.Name),
		zap.String("component", "opcua-server"),
	)
}

func (s *Server) UpdateConfig(cfg model.OPCUAConfig) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if RequiresServerRestart(s.config, cfg) {
		s.stopLocked()
		s.config = cfg
		return s.startLocked()
	}

	s.config = cfg
	if s.srv == nil {
		return s.startLocked()
	}
	return s.rebuildAddressSpaceInPlaceLocked()
}

// SyncAddressSpace refreshes OPC UA nodes from southbound topology without
// stopping the TCP listener. Structural config changes still require UpdateConfig restart.
func (s *Server) SyncAddressSpace() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.srv == nil {
		if !s.config.Enable {
			return nil
		}
		return s.startLocked()
	}
	return s.rebuildAddressSpaceInPlaceLocked()
}

// RequiresServerRestart reports whether northbound OPC UA must Stop+Start
// (port/security/endpoint/auth/certs). Address-space / device-mapping changes do not.
func RequiresServerRestart(oldCfg, newCfg model.OPCUAConfig) bool {
	if oldCfg.Port != newCfg.Port {
		return true
	}
	if oldCfg.Endpoint != newCfg.Endpoint {
		return true
	}
	if oldCfg.SecurityPolicy != newCfg.SecurityPolicy {
		return true
	}
	if oldCfg.SecurityMode != newCfg.SecurityMode {
		return true
	}
	if oldCfg.TrustedCertPath != newCfg.TrustedCertPath {
		return true
	}
	if oldCfg.ServerCertPEM != newCfg.ServerCertPEM || oldCfg.ServerKeyPEM != newCfg.ServerKeyPEM {
		return true
	}
	if oldCfg.CertFile != newCfg.CertFile || oldCfg.KeyFile != newCfg.KeyFile {
		return true
	}
	if !stringSlicesEqual(oldCfg.AuthMethods, newCfg.AuthMethods) {
		return true
	}
	if !stringMapsEqual(oldCfg.Users, newCfg.Users) {
		return true
	}
	if !stringSlicesEqual(oldCfg.TrustedCertsPEM, newCfg.TrustedCertsPEM) {
		return true
	}
	return false
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (s *Server) rebuildAddressSpaceInPlaceLocked() error {
	nsURI := "http://edgeCore-gateway.com/opcua"
	nsIndex := s.srv.NamespaceManager().Add(nsURI)
	gatewayID := ua.ParseNodeID(fmt.Sprintf("ns=%d;s=G", nsIndex))
	if node, ok := s.srv.NamespaceManager().FindNode(gatewayID); ok {
		if err := s.srv.NamespaceManager().DeleteNode(node, true); err != nil {
			zap.L().Warn("OPC UA delete gateway tree before rebuild",
				zap.Error(err),
				zap.String("component", "opcua-server"),
			)
		}
	}

	s.mu.Lock()
	s.nodeMap = make(map[string]*server.VariableNode)
	s.virtualDeviceIDs = make(map[string]struct{})
	s.mu.Unlock()

	if err := s.buildAddressSpace(); err != nil {
		return err
	}
	zap.L().Info("OPC UA address space synced in-place",
		zap.String("name", s.config.Name),
		zap.String("component", "opcua-server"),
	)
	return nil
}

func (s *Server) Update(v model.Value) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, isVirtual := s.virtualDeviceIDs[v.DeviceID]
	if isVirtual {
		if len(s.config.VirtualDevices) > 0 && !s.config.VirtualDevices.AllowsDevice(v.DeviceID) {
			return
		}
	} else if len(s.config.Devices) > 0 && !s.config.Devices.AllowsDevice(v.DeviceID) {
		return
	}

	key := fmt.Sprintf("%s/%s/%s", v.ChannelID, v.DeviceID, v.PointID)

	if node, ok := s.nodeMap[key]; ok {
		status := uint32(0) // Good
		if v.Quality != "Good" {
			status = 0x80000000 // Bad
		}

		// zap.L().Debug("OPC UA Node Update",
		// 	zap.String("point_id", v.PointID),
		// 	zap.Any("value", v.Value),
		// 	zap.String("quality", v.Quality),
		// 	zap.String("component", "opcua-server"),
		// )

		node.SetValue(ua.DataValue{
			Value:           v.Value,
			StatusCode:      ua.StatusCode(status),
			SourceTimestamp: v.TS,
			ServerTimestamp: time.Now(),
		})
	}
}

func (s *Server) buildAddressSpace() error {
	nsURI := "http://edgeCore-gateway.com/opcua"
	// Get namespace index from the server
	nsIndex := s.srv.NamespaceManager().Add(nsURI)
	//zap.L().Info("OPC UA Namespace Added", zap.String("uri", nsURI), zap.Uint16("index", nsIndex))

	// Create a new mapper for this address space build
	// This ensures clean state and correct ID assignment
	s.idMapper = NewNodeIDMapper()
	s.mu.Lock()
	s.virtualDeviceIDs = make(map[string]struct{})
	s.mu.Unlock()

	createFolder := func(parentID ua.NodeID, id string, name string) ua.NodeID {
		nodeID := ua.ParseNodeID(fmt.Sprintf("ns=%d;s=%s", nsIndex, id))
		organizes := ua.ParseNodeID("i=35")
		node := server.NewObjectNode(
			s.srv,
			nodeID,
			ua.QualifiedName{NamespaceIndex: nsIndex, Name: name},
			ua.LocalizedText{Text: name, Locale: "en"},
			ua.LocalizedText{},
			nil,
			[]ua.Reference{
				{ReferenceTypeID: organizes, IsInverse: true, TargetID: ua.ExpandedNodeID{NodeID: parentID}},
			},
			0,
		)
		if err := s.srv.NamespaceManager().AddNode(node); err != nil {
			zap.L().Error("Failed to add OPC UA Object Node", zap.String("node_id", fmt.Sprintf("%v", nodeID)), zap.Error(err))
		}
		return nodeID
	}

	// Helper to create Variable with string node ID
	createVar := func(parentID ua.NodeID, id string, name string, val interface{}, typeID ua.NodeID, accessLevel byte, writeHandler func(sess *server.Session, req ua.WriteValue) (ua.DataValue, ua.StatusCode)) *server.VariableNode {
		nodeID := ua.ParseNodeID(fmt.Sprintf("ns=%d;s=%s", nsIndex, id))
		hasComponent := ua.ParseNodeID("i=47")

		// Create VariableNode
		v := server.NewVariableNode(
			s.srv,
			nodeID,
			ua.QualifiedName{NamespaceIndex: nsIndex, Name: name},
			ua.LocalizedText{Text: name, Locale: "en"},
			ua.LocalizedText{},
			nil,
			[]ua.Reference{
				{ReferenceTypeID: hasComponent, IsInverse: true, TargetID: ua.ExpandedNodeID{NodeID: parentID}},
			},
			ua.DataValue{Value: val},
			typeID,
			-1,
			nil,
			accessLevel,
			0.0,
			false,
			nil, // Historian
		)

		if err := s.srv.NamespaceManager().AddNode(v); err != nil {
			zap.L().Error("Failed to add OPC UA Variable Node", zap.String("node_id", fmt.Sprintf("%v", nodeID)), zap.Error(err))
		}

		if writeHandler != nil {
			v.SetWriteValueHandler(writeHandler)
		}

		return v
	}

	objectsFolder := ua.ParseNodeID("i=85")

	// Create Gateway root folder (always "G" as first element of compact IDs)
	gatewayID := createFolder(objectsFolder, "G", "Gateway")

	infoID := createFolder(gatewayID, "G/Info", "Info")

	s.mu.Lock()
	s.nodeMap["System/CPUUsage"] = createVar(infoID, "G/Info/CPUUsage", "CPUUsage", 0.0, s.getDataTypeID("double"), 1, nil)
	s.nodeMap["System/MemoryUsage"] = createVar(infoID, "G/Info/MemoryUsage", "MemoryUsage", 0.0, s.getDataTypeID("double"), 1, nil)
	s.nodeMap["System/Goroutines"] = createVar(infoID, "G/Info/Goroutines", "Goroutines", int32(0), s.getDataTypeID("int32"), 1, nil)
	s.nodeMap["System/Uptime"] = createVar(infoID, "G/Info/Uptime", "Uptime", int64(0), s.getDataTypeID("int64"), 1, nil)
	s.nodeMap["System/ClientCount"] = createVar(infoID, "G/Info/ClientCount", "ClientCount", int32(0), s.getDataTypeID("int32"), 1, nil)
	s.nodeMap["System/SubscriptionCount"] = createVar(infoID, "G/Info/SubscriptionCount", "SubscriptionCount", int32(0), s.getDataTypeID("int32"), 1, nil)
	s.nodeMap["System/WriteCount"] = createVar(infoID, "G/Info/WriteCount", "WriteCount", int64(0), s.getDataTypeID("int64"), 1, nil)
	s.mu.Unlock()

	// Create Channels folder
	channelsID := createFolder(gatewayID, "G/Channels", "Channels")

	channels := s.sb.GetChannels()
	sort.Slice(channels, func(i, j int) bool { return channels[i].ID < channels[j].ID })
	//zap.L().Info("Building OPC UA Address Space", zap.Int("channel_count", len(channels)))

	for _, ch := range channels {
		// Generate compact channel node ID: G/{channelNum}
		chCompactID := s.idMapper.GenerateCompactFolderID(ch.ID, "")

		// Use original ID as BrowseName for human readability
		chNodeID := createFolder(channelsID, chCompactID, ch.ID)

		// Protocol and Status under channel
		createVar(chNodeID, chCompactID+"/Protocol", "Protocol", ch.Protocol, s.getDataTypeID("string"), 1, nil)
		createVar(chNodeID, chCompactID+"/Status", "Status", "Running", s.getDataTypeID("string"), 1, nil)

		// Create Devices folder with compact ID: G/{channelNum}/D
		devsCompactID := chCompactID + "/D"
		devsNodeID := createFolder(chNodeID, devsCompactID, "Devices")

		//zap.L().Info("Processing Devices for Channel", zap.String("channel_id", ch.ID), zap.Int("device_count", len(ch.Devices)))

		for _, dev := range ch.Devices {
			//zap.L().Info("Processing Device", zap.String("device_id", dev.ID), zap.String("device_name", dev.Name), zap.Int("point_count", len(dev.Points)))

			// Check if device is enabled in config
			// If config.Devices is empty, we assume "Allow All" for better UX.
			// If config.Devices is populated, we apply strict filtering.
			if len(s.config.Devices) > 0 {
				if !s.config.Devices.AllowsDevice(dev.ID) {
					zap.L().Debug("OPC UA device excluded by mapping",
						zap.String("device_id", dev.ID),
						zap.String("channel_id", ch.ID),
					)
					continue
				}
			}

			// Generate compact device node ID: G/{channelNum}/D/{deviceNum}
			dCompactID := s.idMapper.GenerateCompactFolderID(ch.ID, dev.ID)

			//			zap.L().Info("Adding OPC UA Device Node", zap.String("device_id", dev.ID), zap.String("device_name", dev.Name))
			// Use original ID as BrowseName
			dNodeID := createFolder(devsNodeID, dCompactID, dev.ID)

			createVar(dNodeID, dCompactID+"/Vendor", "Vendor", getString(dev.Config, "vendor_name"), s.getDataTypeID("string"), 1, nil)
			createVar(dNodeID, dCompactID+"/Model", "Model", getString(dev.Config, "model_name"), s.getDataTypeID("string"), 1, nil)

			// Create Points folder with compact ID: G/{channelNum}/D/{deviceNum}/P
			pointsCompactID := dCompactID + "/P"
			pointsNodeID := createFolder(dNodeID, pointsCompactID, "Points")

			//			zap.L().Info("Adding OPC UA Points for Device", zap.String("device_id", dev.ID), zap.Int("point_count", len(dev.Points)))

			for _, p := range dev.Points {
				// Use full path as internal key for nodeMap (for Update/WriteViaOPCUA lookups)
				pKey := fmt.Sprintf("%s/%s/%s", ch.ID, dev.ID, p.ID)

				// Generate string node ID: ns=2;s={channelID}.{deviceID}.{pointID}
				// Channel prefix keeps NodeIDs globally unique across channels.
				_ = s.idMapper.GenerateCompactNodeID(ch.ID, dev.ID, p.ID)
				stringID := fmt.Sprintf("%s.%s.%s", ch.ID, dev.ID, p.ID)

				accessLevel := byte(1)
				if strings.Contains(strings.ToUpper(p.ReadWrite), "W") {
					accessLevel |= 2
				}

				dataTypeID := s.getDataTypeID(p.DataType)

				var writeHandler func(sess *server.Session, req ua.WriteValue) (ua.DataValue, ua.StatusCode)
				if accessLevel&2 != 0 {
					cid, did, pid := ch.ID, dev.ID, p.ID
					pType := p.DataType
					writeHandler = func(sess *server.Session, req ua.WriteValue) (ua.DataValue, ua.StatusCode) {
						// Only allow writing to Value attribute
						if req.AttributeID != ua.AttributeIDValue {
							zap.L().Warn("OPC UA Write Rejected: Not Value Attribute", zap.Uint32("attr_id", req.AttributeID))
							return ua.DataValue{}, ua.StatusCode(0x80730000) // BadWriteNotSupported
						}

						// Extract and convert value to expected type
						val := convertToType(req.Value.Value, pType)

						zap.L().Info("OPC UA Write Request Received",
							zap.String("channel_id", cid),
							zap.String("device_id", did),
							zap.String("point_id", pid),
							zap.Any("value", val),
							zap.String("component", "opcua-server"),
						)

						// Update stats
						s.mu.Lock()
						s.stats.WriteCount++
						writeCount := s.stats.WriteCount
						s.mu.Unlock()

						// Update system node (must be done outside of lock to avoid deadlock with updateSystemNode's internal RLock)
						s.updateSystemNode("WriteCount", writeCount)

						// Call Southbound Write
						err := s.sb.WritePoint(cid, did, pid, val)
						if err != nil {
							zap.L().Error("OPC UA Write Failed (SB)",
								zap.String("channel_id", cid),
								zap.String("device_id", did),
								zap.String("point_id", pid),
								zap.Error(err),
								zap.String("component", "opcua-server"),
							)
							// Change to BadInternalError (0x80020000) to distinguish from Access Denied
							return ua.DataValue{}, ua.StatusCode(0x80020000)
						}

						zap.L().Info("OPC UA Write Success (SB)",
							zap.String("point_id", pid),
							zap.Any("value", val),
						)

						// Return the value so the server updates the node
						// Ensure the returned value has the correct type
						return ua.DataValue{
							Value:           val,
							StatusCode:      ua.StatusCode(0),
							SourceTimestamp: time.Now(),
							ServerTimestamp: time.Now(),
						}, ua.StatusCode(0)
					}
				}

				// Create variable node with STRING node ID (ns=2;s=DeviceID.PointID)
				// Use original point name as BrowseName for readability
				// 预加载影子设备实时快照作为初始值，避免客户端首次连接看到零值
				initialVal := s.getZeroValue(p.DataType)
				if sp, err := s.sb.GetShadowPoint(ch.ID, dev.ID, p.ID); err == nil && sp != nil {
					initialVal = convertToType(sp.Value, p.DataType)
				}
				vNode := createVar(pointsNodeID, stringID, p.Name, initialVal, dataTypeID, accessLevel, writeHandler)

				// 设置 ReadHandler：第三方客户端 Read/Subscribe 时从影子设备实时快照读取
				cid, did, pid := ch.ID, dev.ID, p.ID
				pType := p.DataType
				vNode.SetReadValueHandler(func(sess *server.Session, req ua.ReadValueID) ua.DataValue {
					if sp, err := s.sb.GetShadowPoint(cid, did, pid); err == nil && sp != nil {
						status := uint32(0) // Good
						if sp.Quality != "good" && sp.Quality != "Good" {
							status = 0x80000000 // Bad
						}
						return ua.DataValue{
							Value:           convertToType(sp.Value, pType),
							StatusCode:      ua.StatusCode(status),
							SourceTimestamp: sp.CollectedAt,
							ServerTimestamp: time.Now(),
						}
					}
					// 影子设备无数据时回退到节点存储值
					return vNode.Value()
				})

				s.mu.Lock()
				s.nodeMap[pKey] = vNode
				s.mu.Unlock()
				//				zap.L().Info("Added OPC UA Point Node", zap.String("node_id", pNodeID), zap.String("point_id", p.ID), zap.String("point_name", p.Name), zap.String("data_type", p.DataType))
			}
		}

		s.buildVirtualDevicesForChannel(ch, devsNodeID, createFolder, createVar)
	}

	zap.L().Info("OPC UA Address Space built with compact node IDs",
		zap.Int("total_mappings", s.idMapper.Size()),
		zap.String("component", "opcua-server"),
	)

	return nil
}

func (s *Server) buildVirtualDevicesForChannel(
	ch model.Channel,
	devsNodeID ua.NodeID,
	createFolder func(parentID ua.NodeID, id string, name string) ua.NodeID,
	createVar func(parentID ua.NodeID, id string, name string, val interface{}, typeID ua.NodeID, accessLevel byte, writeHandler func(sess *server.Session, req ua.WriteValue) (ua.DataValue, ua.StatusCode)) *server.VariableNode,
) {
	if s.virtualShadows == nil {
		return
	}

	for _, vcfg := range s.virtualShadows.List() {
		if !vcfg.Enable || len(vcfg.Points) == 0 {
			continue
		}
		if model.InferVirtualShadowChannel(vcfg.Points) != ch.ID {
			continue
		}
		if len(s.config.VirtualDevices) > 0 && !s.config.VirtualDevices.AllowsDevice(vcfg.ID) {
			continue
		}

		dCompactID := s.idMapper.GenerateCompactFolderID(ch.ID, vcfg.ID)
		dNodeID := createFolder(devsNodeID, dCompactID, vcfg.ID)
		displayName := vcfg.Name
		if displayName == "" {
			displayName = vcfg.ID
		}
		createVar(dNodeID, dCompactID+"/Name", "Name", displayName, s.getDataTypeID("string"), 1, nil)
		createVar(dNodeID, dCompactID+"/Virtual", "Virtual", true, s.getDataTypeID("bool"), 1, nil)

		pointsCompactID := dCompactID + "/P"
		pointsNodeID := createFolder(dNodeID, pointsCompactID, "Points")

		s.mu.Lock()
		s.virtualDeviceIDs[vcfg.ID] = struct{}{}
		s.mu.Unlock()

		for _, pt := range vcfg.Points {
			pointID := pt.PointID
			if pointID == "" {
				continue
			}
			pKey := fmt.Sprintf("%s/%s/%s", ch.ID, vcfg.ID, pointID)
			stringID := fmt.Sprintf("%s.%s.%s", ch.ID, vcfg.ID, pointID)
			pointName := pt.Name
			if pointName == "" {
				pointName = pointID
			}

			dataType := "double"
			initialVal := s.getZeroValue(dataType)
			if sp, err := s.sb.GetShadowPoint(ch.ID, vcfg.ID, pointID); err == nil && sp != nil {
				dataType = inferShadowPointDataType(sp.Value, dataType)
				initialVal = convertToType(sp.Value, dataType)
			}

			dataTypeID := s.getDataTypeID(dataType)
			cid, vid, pid := ch.ID, vcfg.ID, pointID
			pType := dataType
			vNode := createVar(pointsNodeID, stringID, pointName, initialVal, dataTypeID, 1, nil)
			vNode.SetReadValueHandler(func(sess *server.Session, req ua.ReadValueID) ua.DataValue {
				if sp, err := s.sb.GetShadowPoint(cid, vid, pid); err == nil && sp != nil {
					status := uint32(0)
					if sp.Quality != "good" && sp.Quality != "Good" {
						status = 0x80000000
					}
					return ua.DataValue{
						Value:           convertToType(sp.Value, pType),
						StatusCode:      ua.StatusCode(status),
						SourceTimestamp: sp.CollectedAt,
						ServerTimestamp: time.Now(),
					}
				}
				return vNode.Value()
			})

			s.mu.Lock()
			s.nodeMap[pKey] = vNode
			s.mu.Unlock()
		}
	}
}

func inferShadowPointDataType(val any, fallback string) string {
	switch val.(type) {
	case bool:
		return "bool"
	case int8:
		return "int8"
	case uint8:
		return "uint8"
	case int16:
		return "int16"
	case uint16:
		return "uint16"
	case int32:
		return "int32"
	case uint32:
		return "uint32"
	case int64:
		return "int64"
	case uint64:
		return "uint64"
	case float32:
		return "float32"
	case float64:
		return "double"
	case string:
		return "string"
	case []byte:
		return "bytestring"
	default:
		return fallback
	}
}

func (s *Server) systemInfoLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	startTime := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			s.updateSystemNode("MemoryUsage", float64(mem.Alloc)/1024/1024)
			s.updateSystemNode("Goroutines", int32(runtime.NumGoroutine()))

			uptime := int64(time.Since(startTime).Seconds())
			s.updateSystemNode("Uptime", uptime)

			// Update Client Count
			clientCount := s.getClientCount()
			s.updateSystemNode("ClientCount", int32(clientCount))

			// Update internal stats
			s.mu.Lock()
			s.stats.ClientCount = clientCount
			s.stats.Uptime = uptime
			// WriteCount and SubscriptionCount are currently 0 or updated elsewhere (if we could hook them)
			s.mu.Unlock()
		}
	}
}

func (s *Server) getClientCount() int {
	portStr := fmt.Sprintf(":%d", s.config.Port)
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		// netstat -an | find ":PORT" | find "ESTABLISHED" /c
		cmd = exec.Command("cmd", "/C", fmt.Sprintf("netstat -an | find \"%s\" | find \"ESTABLISHED\" /c", portStr))
	} else {
		// netstat -an | grep :PORT | grep ESTABLISHED | wc -l
		cmd = exec.Command("sh", "-c", fmt.Sprintf("netstat -an | grep '%s' | grep ESTABLISHED | wc -l", portStr))
	}

	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	countStr := strings.TrimSpace(string(out))
	count, _ := strconv.Atoi(countStr)
	return count
}

// Stats represents the monitoring statistics
type Stats struct {
	ClientCount       int                  `json:"client_count"`
	SubscriptionCount int                  `json:"subscription_count"`
	WriteCount        int64                `json:"write_count"`
	Uptime            int64                `json:"uptime"`
	SecurityPolicy    string               `json:"security_policy"`
	SecurityMode      string               `json:"security_mode"`
	AuthMethods       []string             `json:"auth_methods"`
	EndpointCount     int                  `json:"endpoint_count"`
	SecurityCaps      []SecurityCapability `json:"security_capabilities"`
}

// GetStats returns the current statistics
func (s *Server) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.SecurityPolicy = policyDisplayName(s.config.SecurityPolicy)
	if stats.SecurityPolicy == "" {
		stats.SecurityPolicy = "Auto"
	}
	stats.SecurityMode = preferredSecurityMode(s.config)
	stats.AuthMethods = append([]string(nil), s.config.AuthMethods...)
	stats.SecurityCaps = SupportedSecurityCapabilities(s.config)
	if s.srv != nil {
		stats.EndpointCount = len(s.srv.Endpoints())
	}
	return stats
}

func (s *Server) updateSystemNode(name string, value interface{}) {
	s.mu.RLock()
	node, ok := s.nodeMap["System/"+name]
	s.mu.RUnlock()
	if ok {
		node.SetValue(ua.DataValue{
			Value:           value,
			StatusCode:      ua.StatusCode(0),
			SourceTimestamp: time.Now(),
			ServerTimestamp: time.Now(),
		})
	}
}

func (s *Server) getDataTypeID(dtype string) ua.NodeID {
	id := 11
	switch strings.ToLower(dtype) {
	case "bool", "boolean":
		id = 1
	case "int8", "sbyte":
		id = 2
	case "uint8", "byte":
		id = 3
	case "int16":
		id = 4
	case "uint16":
		id = 5
	case "int32":
		id = 6
	case "uint32":
		id = 7
	case "int64":
		id = 8
	case "uint64":
		id = 9
	case "float32":
		id = 10
	case "float64", "double":
		id = 11
	case "string":
		id = 12
	case "bytestring":
		id = 15
	}
	nid := ua.ParseNodeID(fmt.Sprintf("i=%d", id))
	return nid
}

func (s *Server) getZeroValue(dtype string) interface{} {
	if strings.ToLower(dtype) == "bytestring" {
		return []byte{}
	}
	return getZeroValueForType(dtype)
}

// getZeroValueForType returns the zero value for a given data type
func getZeroValueForType(dtype string) interface{} {
	switch strings.ToLower(dtype) {
	case "bool", "boolean":
		return false
	case "int8", "sbyte":
		return int8(0)
	case "uint8", "byte":
		return uint8(0)
	case "int16":
		return int16(0)
	case "uint16":
		return uint16(0)
	case "int32":
		return int32(0)
	case "uint32":
		return uint32(0)
	case "int64":
		return int64(0)
	case "uint64":
		return uint64(0)
	case "float32":
		return float32(0)
	case "float64", "double":
		return float64(0)
	case "string":
		return ""
	default:
		return float64(0)
	}
}

// convertToType converts a value to the specified OPC UA data type
func convertToType(val any, dtype string) interface{} {
	if val == nil {
		return getZeroValueForType(dtype)
	}

	// If already the correct type, return as-is
	switch dtype {
	case "bool", "boolean":
		if _, ok := val.(bool); ok {
			return val
		}
	case "int8", "sbyte":
		if _, ok := val.(int8); ok {
			return val
		}
	case "uint8", "byte":
		if _, ok := val.(uint8); ok {
			return val
		}
	case "int16":
		if _, ok := val.(int16); ok {
			return val
		}
	case "uint16":
		if _, ok := val.(uint16); ok {
			return val
		}
	case "int32":
		if _, ok := val.(int32); ok {
			return val
		}
	case "uint32":
		if _, ok := val.(uint32); ok {
			return val
		}
	case "int64":
		if _, ok := val.(int64); ok {
			return val
		}
	case "uint64":
		if _, ok := val.(uint64); ok {
			return val
		}
	case "float32":
		if _, ok := val.(float32); ok {
			return val
		}
	case "float64", "double":
		if _, ok := val.(float64); ok {
			return val
		}
	case "string":
		if _, ok := val.(string); ok {
			return val
		}
	}

	// Convert string to target type
	strVal := fmt.Sprintf("%v", val)

	switch strings.ToLower(dtype) {
	case "bool", "boolean":
		if b, err := strconv.ParseBool(strVal); err == nil {
			return b
		}
		return false
	case "int8", "sbyte":
		if v, err := strconv.ParseInt(strVal, 10, 8); err == nil {
			return int8(v)
		}
		return int8(0)
	case "uint8", "byte":
		if v, err := strconv.ParseUint(strVal, 10, 8); err == nil {
			return uint8(v)
		}
		return uint8(0)
	case "int16":
		if v, err := strconv.ParseInt(strVal, 10, 16); err == nil {
			return int16(v)
		}
		return int16(0)
	case "uint16":
		if v, err := strconv.ParseUint(strVal, 10, 16); err == nil {
			return uint16(v)
		}
		return uint16(0)
	case "int32":
		if v, err := strconv.ParseInt(strVal, 10, 32); err == nil {
			return int32(v)
		}
		return int32(0)
	case "uint32":
		if v, err := strconv.ParseUint(strVal, 10, 32); err == nil {
			return uint32(v)
		}
		return uint32(0)
	case "int64":
		if v, err := strconv.ParseInt(strVal, 10, 64); err == nil {
			return v
		}
		return int64(0)
	case "uint64":
		if v, err := strconv.ParseUint(strVal, 10, 64); err == nil {
			return v
		}
		return uint64(0)
	case "float32":
		if v, err := strconv.ParseFloat(strVal, 32); err == nil {
			return float32(v)
		}
		return float32(0)
	case "float64", "double":
		if v, err := strconv.ParseFloat(strVal, 64); err == nil {
			return v
		}
		return float64(0)
	case "string":
		return strVal
	default:
		return val
	}
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// WriteViaOPCUA 通过 OPC-UA 服务端写入值（外部调用接口）
// 返回写入是否成功，以及更新后的值（用于验证）
func (s *Server) WriteViaOPCUA(channelID, deviceID, pointID string, value any) error {
	key := fmt.Sprintf("%s/%s/%s", channelID, deviceID, pointID)

	s.mu.RLock()
	node, ok := s.nodeMap[key]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("node not found: %s", key)
	}

	// 调用 WritePoint
	req := WriteRequest{
		ChannelID: channelID,
		DeviceID:  deviceID,
		PointID:   pointID,
		Value:     value,
	}
	err := s.sb.WritePoint(channelID, deviceID, pointID, value)
	if err != nil {
		s.recordWrite(req, false, err.Error())
		return fmt.Errorf("write failed: %v", err)
	}

	// 更新节点值
	status := uint32(0) // Good
	node.SetValue(ua.DataValue{
		Value:           value,
		StatusCode:      ua.StatusCode(status),
		SourceTimestamp: time.Now(),
		ServerTimestamp: time.Now(),
	})

	// 更新统计
	s.mu.Lock()
	s.stats.WriteCount++
	s.mu.Unlock()

	s.recordWrite(req, true, "")

	return nil
}

// BatchWrite 批量写入多个点位
type WriteRequest struct {
	ChannelID string `json:"channel_id"`
	DeviceID  string `json:"device_id"`
	PointID   string `json:"point_id"`
	Value     any    `json:"value"`
}

type BatchWriteResult struct {
	ChannelID string `json:"channel_id"`
	DeviceID  string `json:"device_id"`
	PointID   string `json:"point_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) BatchWrite(requests []WriteRequest) []BatchWriteResult {
	results := make([]BatchWriteResult, 0, len(requests))

	for _, req := range requests {
		result := BatchWriteResult{
			ChannelID: req.ChannelID,
			DeviceID:  req.DeviceID,
			PointID:   req.PointID,
			Success:   false,
		}

		err := s.WriteViaOPCUA(req.ChannelID, req.DeviceID, req.PointID, req.Value)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Success = true
		}

		results = append(results, result)
	}

	return results
}

// WriteHistoryItem 写入历史记录
type WriteHistoryItem struct {
	ChannelID string    `json:"channel_id"`
	DeviceID  string    `json:"device_id"`
	PointID   string    `json:"point_id"`
	Value     any       `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

const maxWriteHistorySize = 1000

// recordWrite 记录写入历史
func (s *Server) recordWrite(req WriteRequest, success bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := WriteHistoryItem{
		ChannelID: req.ChannelID,
		DeviceID:  req.DeviceID,
		PointID:   req.PointID,
		Value:     req.Value,
		Timestamp: time.Now(),
		Success:   success,
		Error:     errMsg,
	}

	s.writeHistory = append(s.writeHistory, item)

	if len(s.writeHistory) > maxWriteHistorySize {
		s.writeHistory = s.writeHistory[len(s.writeHistory)-maxWriteHistorySize:]
	}
}

// GetWriteHistory 获取写入历史
func (s *Server) GetWriteHistory(limit int) []WriteHistoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.writeHistory) {
		limit = len(s.writeHistory)
	}

	start := len(s.writeHistory) - limit
	result := make([]WriteHistoryItem, limit)
	copy(result, s.writeHistory[start:])
	return result
}

// NodeIDMappingInfo 返回节点 ID 映射信息，用于调试和诊断
type NodeIDMappingInfo struct {
	CompactID string `json:"compact_id"`
	FullPath  string `json:"full_path"`
	Type      string `json:"type"` // "channel", "device", "point"
	ChannelID string `json:"channel_id,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`
	PointID   string `json:"point_id,omitempty"`
}

// GetNodeIDMappings 返回所有节点 ID 映射
func (s *Server) GetNodeIDMappings() []NodeIDMappingInfo {
	if s.idMapper == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var mappings []NodeIDMappingInfo

	// All mappings are now in reverseShortPath with ns=2;i=XXXX format
	s.idMapper.mu.RLock()
	for shortPath, fullPath := range s.idMapper.reverseShortPath {
		mappings = append(mappings, NodeIDMappingInfo{
			CompactID: shortPath,
			FullPath:  fullPath,
			Type:      "full",
		})
	}
	s.idMapper.mu.RUnlock()

	return mappings
}

// ResolveNodeID 根据节点 ID 字符串解析为原始 ID
// 支持格式: ns=2;i=XXXX, 111 (legacy), Gateway/Channels/...
// 返回 (channelID, deviceID, pointID, isCompact, error)
func (s *Server) ResolveNodeID(nodeID string) (string, string, string, bool, error) {
	if s.idMapper == nil {
		return "", "", "", false, fmt.Errorf("id mapper not initialized")
	}

	// Try to parse as compact ID format: ns=X;i=Y
	if strings.HasPrefix(nodeID, "ns=") {
		chID, devID, ptID, ok := s.idMapper.GetOriginalIDs(nodeID)
		if ok {
			return chID, devID, ptID, true, nil
		}
		return "", "", "", false, fmt.Errorf("invalid compact node ID: %s", nodeID)
	}

	// Try to parse as legacy compact ID (all digits, e.g., 111)
	if isCompactNodeID(nodeID) {
		chID, devID, ptID, ok := s.idMapper.GetOriginalIDs(nodeID)
		if ok {
			return chID, devID, ptID, true, nil
		}
		return "", "", "", false, fmt.Errorf("invalid compact node ID: %s", nodeID)
	}

	// Try to parse full path format
	// Format: Gateway/Channels/{channelID}/Devices/{deviceID}/Points/{pointID}
	parts := strings.Split(nodeID, "/")
	if len(parts) >= 6 && parts[0] == "Gateway" && parts[1] == "Channels" && parts[3] == "Devices" && parts[5] == "Points" {
		return parts[2], parts[4], parts[6], false, nil
	}

	return "", "", "", false, fmt.Errorf("unrecognized node ID format: %s", nodeID)
}

// GetCompactNodeID 返回指定通道/设备/点位的紧凑节点 ID
// 如果不存在返回空字符串
func (s *Server) GetCompactNodeID(channelID, deviceID, pointID string) string {
	if s.idMapper == nil {
		return ""
	}

	// Check if this mapping exists by looking at the reverse short path
	key := fmt.Sprintf("%s/%s/%s", channelID, deviceID, pointID)
	s.idMapper.mu.RLock()
	defer s.idMapper.mu.RUnlock()

	for shortPath, fullPath := range s.idMapper.reverseShortPath {
		if fullPath == key {
			return shortPath
		}
	}

	// If not found, generate a new one (will assign new numbers)
	return s.idMapper.GenerateCompactNodeID(channelID, deviceID, pointID)
}

// CompactIDStats 返回紧凑 ID 的统计信息
type CompactIDStats struct {
	TotalMappings int    `json:"total_mappings"`
	Namespace     uint16 `json:"namespace"`
	NextFolderID  uint32 `json:"next_folder_id"`
	SampleMapping string `json:"sample_mapping,omitempty"` // e.g., "Device001.Temperature -> ns=2;s=Device001.Temperature"
}

// GetCompactIDStats 返回紧凑 ID 统计信息
func (s *Server) GetCompactIDStats() CompactIDStats {
	if s.idMapper == nil {
		return CompactIDStats{}
	}

	stats := s.idMapper.Size()

	s.idMapper.mu.RLock()
	ns := s.idMapper.namespace
	nextFolderID := s.idMapper.nextFolderID

	// Get a sample mapping
	var sample string
	for shortPath, fullPath := range s.idMapper.reverseShortPath {
		sample = fmt.Sprintf("%s -> %s", fullPath, shortPath)
		break
	}
	s.idMapper.mu.RUnlock()

	return CompactIDStats{
		TotalMappings: stats,
		Namespace:     ns,
		NextFolderID:  nextFolderID,
		SampleMapping: sample,
	}
}
