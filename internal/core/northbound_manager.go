package core

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/northbound/bacnet"
	"github.com/anviod/edgeCore/internal/northbound/edgos_mqtt"
	"github.com/anviod/edgeCore/internal/northbound/edgos_nats"
	"github.com/anviod/edgeCore/internal/northbound/http"
	"github.com/anviod/edgeCore/internal/northbound/mqtt"
	"github.com/anviod/edgeCore/internal/northbound/opcua"
	"github.com/anviod/edgeCore/internal/northbound/sparkplugb"
	"github.com/anviod/edgeCore/internal/storage"
)

type NorthboundStatus struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// connectorStartWarning logs a connector start failure after config was persisted
// and returns a user-facing warning message (empty when start succeeded).
func connectorStartWarning(protocol, name string, err error) string {
	if err == nil {
		return ""
	}
	log.Printf("Northbound %s [%s]: config saved but connector start failed: %v", protocol, name, err)
	return fmt.Sprintf("配置已保存，但 %s 暂不可达：%v", protocol, err)
}

type NorthboundManager struct {
	config            model.NorthboundConfig
	mqttClients       map[string]*mqtt.Client
	httpClients       map[string]*http.Client
	opcuaServers      map[string]*opcua.Server
	sparkplugClients  map[string]*sparkplugb.Client
	edgeOSMQTTClients map[string]*edgos_mqtt.Client
	edgeOSNATSClients map[string]*edgos_nats.Client
	bacnetServers     map[string]*bacnet.Server
	pipeline          *DataPipeline
	sb                model.SouthboundManager
	cm                *ChannelManager // Reference to ChannelManager for device lookups
	vsm               *VirtualShadowManager
	storage           *storage.Storage
	ctx               context.Context
	cancel            context.CancelFunc
	saveFunc          func(model.NorthboundConfig) error
	mu                sync.RWMutex

	shadowCore      *ShadowCore
	eanShadowBridge *capability.ShadowEventBridge
	eanShadowOnce   sync.Once
}

func NewNorthboundManager(cfg model.NorthboundConfig, pipeline *DataPipeline, sb model.SouthboundManager, s *storage.Storage, saveFunc func(model.NorthboundConfig) error) *NorthboundManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &NorthboundManager{
		config:            cfg,
		mqttClients:       make(map[string]*mqtt.Client),
		httpClients:       make(map[string]*http.Client),
		opcuaServers:      make(map[string]*opcua.Server),
		sparkplugClients:  make(map[string]*sparkplugb.Client),
		edgeOSMQTTClients: make(map[string]*edgos_mqtt.Client),
		edgeOSNATSClients: make(map[string]*edgos_nats.Client),
		bacnetServers:     make(map[string]*bacnet.Server),
		pipeline:          pipeline,
		sb:                sb,
		cm:                nil, // Set via SetChannelManager
		storage:           s,
		ctx:               ctx,
		cancel:            cancel,
		saveFunc:          saveFunc,
	}
}

func (nm *NorthboundManager) GetNorthboundStats() []NorthboundStatus {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var stats []NorthboundStatus

	// MQTT
	for _, cfg := range nm.config.MQTT {
		status := "Stopped"
		if !cfg.Enable {
			status = "Disabled"
		} else if _, ok := nm.mqttClients[cfg.ID]; ok {
			status = "Running"
		}
		stats = append(stats, NorthboundStatus{
			ID:     cfg.ID,
			Name:   cfg.Name,
			Type:   "MQTT",
			Status: status,
		})
	}

	// OPC UA
	for _, cfg := range nm.config.OPCUA {
		status := "Stopped"
		if !cfg.Enable {
			status = "Disabled"
		} else if _, ok := nm.opcuaServers[cfg.ID]; ok {
			status = "Running"
		}
		stats = append(stats, NorthboundStatus{
			ID:     cfg.ID,
			Name:   cfg.Name,
			Type:   "OPC UA",
			Status: status,
		})
	}

	// SparkplugB
	for _, cfg := range nm.config.SparkplugB {
		status := "Stopped"
		if !cfg.Enable {
			status = "Disabled"
		} else if _, ok := nm.sparkplugClients[cfg.ID]; ok {
			status = "Running"
		}
		stats = append(stats, NorthboundStatus{
			ID:     cfg.ID,
			Name:   cfg.Name,
			Type:   "SparkplugB",
			Status: status,
		})
	}

	// edgeOS(MQTT)
	for _, cfg := range nm.config.EdgeOSMQTT {
		status := "Stopped"
		if !cfg.Enable {
			status = "Disabled"
		} else if _, ok := nm.edgeOSMQTTClients[cfg.ID]; ok {
			status = "Running"
		}
		stats = append(stats, NorthboundStatus{
			ID:     cfg.ID,
			Name:   cfg.Name,
			Type:   "edgeOS(MQTT)",
			Status: status,
		})
	}

	// edgeOS(NATS)
	for _, cfg := range nm.config.EdgeOSNATS {
		status := "Stopped"
		if !cfg.Enable {
			status = "Disabled"
		} else if _, ok := nm.edgeOSNATSClients[cfg.ID]; ok {
			status = "Running"
		}
		stats = append(stats, NorthboundStatus{
			ID:     cfg.ID,
			Name:   cfg.Name,
			Type:   "edgeOS(NATS)",
			Status: status,
		})
	}

	// BACnet Server
	for _, cfg := range nm.config.BACnetServer {
		status := "Stopped"
		if !cfg.Enable {
			status = "Disabled"
		} else if _, ok := nm.bacnetServers[cfg.ID]; ok {
			status = "Running"
		}
		stats = append(stats, NorthboundStatus{
			ID:     cfg.ID,
			Name:   cfg.Name,
			Type:   "BACnet Server",
			Status: status,
		})
	}

	return stats
}

func (nm *NorthboundManager) SetVirtualShadowManager(vsm *VirtualShadowManager) {
	nm.vsm = vsm
}

func (nm *NorthboundManager) newOPCUAServer(cfg model.OPCUAConfig) *opcua.Server {
	var lister opcua.VirtualShadowLister
	if nm.vsm != nil {
		lister = nm.vsm
	}
	return opcua.NewServer(cfg, nm.sb, lister)
}

func (nm *NorthboundManager) Start() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Start MQTT Clients
	for _, cfg := range nm.config.MQTT {
		if cfg.Enable {
			client := mqtt.NewClient(cfg, nm.sb, nm.storage)
			if err := client.Start(); err != nil {
				log.Printf("Failed to start MQTT client [%s]: %v", cfg.Name, err)
			} else {
				log.Printf("Northbound MQTT client [%s] started", cfg.Name)
				nm.mqttClients[cfg.ID] = client
			}
		}
	}

	// Start HTTP Clients
	for _, cfg := range nm.config.HTTP {
		if cfg.Enable {
			client := http.NewClient(cfg, nm.storage)
			client.Start()
			nm.httpClients[cfg.ID] = client
			log.Printf("Northbound HTTP client [%s] started", cfg.Name)
		}
	}

	// Start OPC UA Servers
	for _, cfg := range nm.config.OPCUA {
		if cfg.Enable {
			server := nm.newOPCUAServer(cfg)
			if err := server.Start(); err != nil {
				log.Printf("Failed to start OPC UA server [%s]: %v", cfg.Name, err)
			} else {
				//log.Printf("Northbound OPC UA server [%s] started", cfg.Name)
				nm.opcuaServers[cfg.ID] = server
			}
		}
	}

	// Start SparkplugB Clients
	for _, cfg := range nm.config.SparkplugB {
		if cfg.Enable {
			client := sparkplugb.NewClient(cfg)
			if err := client.Start(); err != nil {
				log.Printf("Failed to start Sparkplug B client [%s]: %v", cfg.Name, err)
			} else {
				log.Printf("Northbound Sparkplug B client [%s] started", cfg.Name)
				nm.sparkplugClients[cfg.ID] = client
			}
		}
	}

	// Start edgeOS(MQTT) Clients
	for _, cfg := range nm.config.EdgeOSMQTT {
		if cfg.Enable {
			client := edgos_mqtt.NewClient(cfg, nm.sb, nm.storage)
			nm.wireEdgeOSMQTTClient(client)
			nm.edgeOSMQTTClients[cfg.ID] = client
			if err := client.Start(); err != nil {
				log.Printf("Failed to start edgeOS(MQTT) client [%s]: %v", cfg.Name, err)
			} else {
				log.Printf("Northbound edgeOS(MQTT) client [%s] started", cfg.Name)
			}
		}
	}

	// Start edgeOS(NATS) Clients
	for _, cfg := range nm.config.EdgeOSNATS {
		if cfg.Enable {
			client := edgos_nats.NewClient(cfg, nm.sb, nm.storage)
			nm.wireEdgeOSNATSClient(client)
			nm.edgeOSNATSClients[cfg.ID] = client
			if err := client.Start(); err != nil {
				log.Printf("Failed to start edgeOS(NATS) client [%s]: %v", cfg.Name, err)
			} else {
				log.Printf("Northbound edgeOS(NATS) client [%s] started", cfg.Name)
			}
		}
	}

	// Start BACnet Servers
	for _, cfg := range nm.config.BACnetServer {
		if cfg.Enable {
			server := bacnet.NewServer(cfg, nm.sb)
			if err := server.Start(); err != nil {
				log.Printf("Failed to start BACnet Server [%s]: %v", cfg.Name, err)
			} else {
				log.Printf("Northbound BACnet Server [%s] started", cfg.Name)
				nm.bacnetServers[cfg.ID] = server
			}
		}
	}

	// Subscribe to pipeline
	nm.pipeline.AddHandler(nm.handleValue)
}

func (nm *NorthboundManager) handleValue(v model.Value) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	for _, client := range nm.mqttClients {
		client.Publish(v)
	}
	for _, client := range nm.httpClients {
		client.Publish(v)
	}
	for _, server := range nm.opcuaServers {
		server.Update(v)
	}
	for _, client := range nm.sparkplugClients {
		client.Publish(v)
	}
	for _, client := range nm.edgeOSMQTTClients {
		client.Publish(v)
	}
	for _, client := range nm.edgeOSNATSClients {
		client.Publish(v)
	}
	for _, server := range nm.bacnetServers {
		server.Update(v)
	}
}

// OnDeviceStatusChange handles device status changes and notifies northbound clients.
// Virtual shadow devices do not emit southbound status events; only physical devices are handled here.
func (nm *NorthboundManager) OnDeviceStatusChange(deviceID string, status int) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	for _, cfg := range nm.config.MQTT {
		if client, ok := nm.mqttClients[cfg.ID]; ok {
			if devCfg, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(cfg.Devices), cfg.VirtualDevices); ok && devCfg.Enable {
				client.PublishDeviceStatus(deviceID, status)
			}
		}
	}

	for _, cfg := range nm.config.HTTP {
		if client, ok := nm.httpClients[cfg.ID]; ok {
			if devCfg, ok := model.LookupNorthboundPublishConfig(deviceID, cfg.Devices, cfg.VirtualDevices); ok && devCfg.Enable {
				client.PublishDeviceStatus(deviceID, status)
			}
		}
	}

	for _, cfg := range nm.config.EdgeOSMQTT {
		if client, ok := nm.edgeOSMQTTClients[cfg.ID]; ok {
			if devCfg, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(cfg.Devices), cfg.VirtualDevices); ok && devCfg.Enable {
				client.PublishDeviceStatus(deviceID, status)
			}
		}
	}

	for _, cfg := range nm.config.EdgeOSNATS {
		if client, ok := nm.edgeOSNATSClients[cfg.ID]; ok {
			if devCfg, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(cfg.Devices), cfg.VirtualDevices); ok && devCfg.Enable {
				client.PublishDeviceStatus(deviceID, status)
			}
		}
	}

	nm.publishDeviceLifecycleNotification(deviceID, status)
}

// publishDeviceLifecycleNotification publishes device online/offline notifications to edgeOS
func (nm *NorthboundManager) publishDeviceLifecycleNotification(deviceID string, status int) {
	// Find the device to get its details
	var device *model.Device
	var channel *model.Channel

	// Iterate through all channels to find the device
	for _, ch := range nm.cm.GetChannels() {
		for _, dev := range ch.Devices {
			if dev.ID == deviceID {
				device = &dev
				channel = &ch
				break
			}
		}
		if device != nil {
			break
		}
	}

	if device == nil {
		return
	}

	// Prepare details for notification
	details := make(map[string]any)
	details["protocol"] = channel.Protocol
	if device.Config != nil {
		for k, v := range device.Config {
			details[k] = v
		}
	}

	// edgeOS(MQTT)
	for _, cfg := range nm.config.EdgeOSMQTT {
		if client, ok := nm.edgeOSMQTTClients[cfg.ID]; ok {
			if devCfg, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(cfg.Devices), cfg.VirtualDevices); ok && devCfg.Enable {
				if status == 0 {
					client.PublishDeviceOnline(deviceID, device.Name, details)
				} else {
					reason := "Unknown"
					if status == 2 {
						reason = "Connection timeout"
					} else if status == 1 {
						reason = "Unstable connection"
					} else if status == 3 {
						reason = "Quarantined"
					}
					client.PublishDeviceOffline(deviceID, device.Name, reason, details)
				}
			}
		}
	}

	// edgeOS(NATS)
	for _, cfg := range nm.config.EdgeOSNATS {
		if client, ok := nm.edgeOSNATSClients[cfg.ID]; ok {
			if devCfg, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(cfg.Devices), cfg.VirtualDevices); ok && devCfg.Enable {
				if status == 0 {
					client.PublishDeviceOnline(deviceID, device.Name, details)
				} else {
					reason := "Unknown"
					if status == 2 {
						reason = "Connection timeout"
					} else if status == 1 {
						reason = "Unstable connection"
					} else if status == 3 {
						reason = "Quarantined"
					}
					client.PublishDeviceOffline(deviceID, device.Name, reason, details)
				}
			}
		}
	}
}

// PublishPointsMetadata publishes all point definitions to all edgeOS clients
func (nm *NorthboundManager) PublishPointsMetadata() {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	for _, client := range nm.edgeOSMQTTClients {
		go func(c *edgos_mqtt.Client) {
			if err := c.PublishPointsMetadata(); err != nil {
				log.Printf("Failed to publish points metadata via edgeOS(MQTT): %v", err)
			}
		}(client)
	}
	for _, client := range nm.edgeOSNATSClients {
		go func(c *edgos_nats.Client) {
			if err := c.PublishPointsMetadata(); err != nil {
				log.Printf("Failed to publish points metadata via edgeOS(NATS): %v", err)
			}
		}(client)
	}
}

// PublishDeviceReport republishes the device inventory (edgeCore/devices/report) to all
// edgeOS clients. Called when channels/devices are added/updated or the northbound
// Devices mapping changes, so EdgeOS's device list stays in sync without re-connecting.
func (nm *NorthboundManager) PublishDeviceReport() {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	for _, client := range nm.edgeOSMQTTClients {
		go func(c *edgos_mqtt.Client) {
			c.PublishDeviceReport()
		}(client)
	}
	for _, client := range nm.edgeOSNATSClients {
		go func(c *edgos_nats.Client) {
			c.PublishDeviceReport()
		}(client)
	}
}

// PublishPointsSync publishes all point current values for a device to all edgeOS clients
func (nm *NorthboundManager) PublishPointsSync(channelID, deviceID string) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	for _, client := range nm.edgeOSMQTTClients {
		go func(c *edgos_mqtt.Client) {
			if err := c.PublishPointsSync(channelID, deviceID); err != nil {
				log.Printf("Failed to publish points sync via edgeOS(MQTT): %v", err)
			}
		}(client)
	}
	for _, client := range nm.edgeOSNATSClients {
		go func(c *edgos_nats.Client) {
			if err := c.PublishPointsSync(channelID, deviceID); err != nil {
				log.Printf("Failed to publish points sync via edgeOS(NATS): %v", err)
			}
		}(client)
	}
}

// PublishMQTT publishes a message to a specific MQTT client or all if clientID is empty
func (nm *NorthboundManager) PublishMQTT(clientID string, topic string, payload []byte) error {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if clientID != "" {
		if client, ok := nm.mqttClients[clientID]; ok {
			return client.PublishRaw(topic, payload)
		}
		return fmt.Errorf("MQTT client %s not found", clientID)
	}

	// If no client ID specified, try to publish to first available or all?
	// For now, let's say if no client ID, we pick the first one.
	for _, client := range nm.mqttClients {
		return client.PublishRaw(topic, payload)
	}
	return fmt.Errorf("no active MQTT clients")
}

func (nm *NorthboundManager) GetOPCUAStats(id string) (opcua.Stats, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if server, ok := nm.opcuaServers[id]; ok {
		return server.GetStats(), nil
	}
	return opcua.Stats{}, fmt.Errorf("OPC UA server %s not found or not running", id)
}

// WriteOPCUA 通过 OPC-UA 服务端写入单个点位值
func (nm *NorthboundManager) WriteOPCUA(serverID, channelID, deviceID, pointID string, value any) error {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	server, ok := nm.opcuaServers[serverID]
	if !ok {
		return fmt.Errorf("OPC UA server %s not found or not running", serverID)
	}

	return server.WriteViaOPCUA(channelID, deviceID, pointID, value)
}

// BatchWriteOPCUA 批量写入多个点位
func (nm *NorthboundManager) BatchWriteOPCUA(serverID string, requests []opcua.WriteRequest) []opcua.BatchWriteResult {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	server, ok := nm.opcuaServers[serverID]
	if !ok {
		results := make([]opcua.BatchWriteResult, len(requests))
		for i := range requests {
			results[i] = opcua.BatchWriteResult{
				ChannelID: requests[i].ChannelID,
				DeviceID:  requests[i].DeviceID,
				PointID:   requests[i].PointID,
				Success:   false,
				Error:     fmt.Sprintf("OPC UA server %s not found", serverID),
			}
		}
		return results
	}

	return server.BatchWrite(requests)
}

// GetOPCUAWriteHistory 获取 OPC-UA 写入历史
func (nm *NorthboundManager) GetOPCUAWriteHistory(serverID string, limit int) ([]opcua.WriteHistoryItem, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	server, ok := nm.opcuaServers[serverID]
	if !ok {
		return nil, fmt.Errorf("OPC UA server %s not found or not running", serverID)
	}

	return server.GetWriteHistory(limit), nil
}

func (nm *NorthboundManager) GetMQTTStats(id string) (mqtt.MQTTStats, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if client, ok := nm.mqttClients[id]; ok {
		return client.GetStats(), nil
	}
	return mqtt.MQTTStats{}, fmt.Errorf("MQTT client %s not found or not running", id)
}

func (nm *NorthboundManager) Stop() {
	nm.cancel()
	nm.mu.Lock()
	defer nm.mu.Unlock()

	for _, client := range nm.mqttClients {
		client.Stop()
	}
	for _, client := range nm.httpClients {
		client.Stop()
	}
	for _, server := range nm.opcuaServers {
		server.Stop()
	}
	for _, client := range nm.sparkplugClients {
		client.Stop()
	}
	for _, client := range nm.edgeOSMQTTClients {
		client.Stop()
	}
	for _, client := range nm.edgeOSNATSClients {
		client.Stop()
	}
	for _, server := range nm.bacnetServers {
		server.Stop()
	}
}

func (nm *NorthboundManager) GetConfig() model.NorthboundConfig {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	// Copy status to config before returning
	status := make(map[string]int)
	for id, client := range nm.mqttClients {
		status[id] = client.GetStatus()
	}
	for id, client := range nm.sparkplugClients {
		status[id] = client.GetStatus()
	}
	for id, client := range nm.edgeOSMQTTClients {
		status[id] = client.GetStatus()
	}
	for id, client := range nm.edgeOSNATSClients {
		status[id] = client.GetStatus()
	}
	// OPC UA status usually implies running if in the map

	cfg := nm.config
	cfg.Status = status
	return model.SanitizeNorthboundForClient(cfg)
}

// MQTT Operations (Implemented in northbound_manager_ext.go)

// UpsertMQTTConfig updates or inserts MQTT configuration and handles device lifecycle events
// See northbound_manager_ext.go for implementation

func (nm *NorthboundManager) DeleteMQTTConfig(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Remove from runtime
	if client, exists := nm.mqttClients[id]; exists {
		client.Stop()
		delete(nm.mqttClients, id)
	}

	// Remove from config
	newConfigs := []model.MQTTConfig{}
	for _, c := range nm.config.MQTT {
		if c.ID != id {
			newConfigs = append(newConfigs, c)
		}
	}
	nm.config.MQTT = newConfigs

	return nm.saveConfig()
}

// SparkplugB Operations

func (nm *NorthboundManager) UpsertSparkplugBConfig(cfg model.SparkplugBConfig) (string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if err := nm.validateNorthboundChannelName(cfg.ID, cfg.Name); err != nil {
		return "", err
	}

	found := false
	for i, c := range nm.config.SparkplugB {
		if c.ID == cfg.ID {
			nm.config.SparkplugB[i] = cfg
			found = true
			break
		}
	}
	if !found {
		nm.config.SparkplugB = append(nm.config.SparkplugB, cfg)
	}

	if err := nm.saveConfig(); err != nil {
		return "", err
	}

	client, exists := nm.sparkplugClients[cfg.ID]

	if !cfg.Enable {
		if exists {
			client.Stop()
			delete(nm.sparkplugClients, cfg.ID)
		}
		return "", nil
	}

	var startErr error
	if !exists {
		newClient := sparkplugb.NewClient(cfg)
		startErr = newClient.Start()
		nm.sparkplugClients[cfg.ID] = newClient
	} else {
		startErr = client.UpdateConfig(cfg)
	}

	return connectorStartWarning("Sparkplug B Broker", cfg.Name, startErr), nil
}

func (nm *NorthboundManager) DeleteSparkplugBConfig(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if client, exists := nm.sparkplugClients[id]; exists {
		client.Stop()
		delete(nm.sparkplugClients, id)
	}

	newConfigs := []model.SparkplugBConfig{}
	for _, c := range nm.config.SparkplugB {
		if c.ID != id {
			newConfigs = append(newConfigs, c)
		}
	}
	nm.config.SparkplugB = newConfigs

	return nm.saveConfig()
}

// OPC UA Operations

func (nm *NorthboundManager) UpsertOPCUAConfig(cfg model.OPCUAConfig) (model.OPCUAConfig, string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if err := nm.validateNorthboundChannelName(cfg.ID, cfg.Name); err != nil {
		return model.OPCUAConfig{}, "", err
	}

	found := false
	for i, c := range nm.config.OPCUA {
		if c.ID == cfg.ID {
			cfg = model.MergeOPCUAConfig(c, cfg)
			nm.config.OPCUA[i] = cfg
			found = true
			break
		}
	}
	if !found {
		nm.config.OPCUA = append(nm.config.OPCUA, cfg)
	}

	if strings.TrimSpace(cfg.ServerCertPEM) != "" || strings.TrimSpace(cfg.ServerKeyPEM) != "" {
		if strings.TrimSpace(cfg.ServerCertPEM) == "" || strings.TrimSpace(cfg.ServerKeyPEM) == "" {
			return model.OPCUAConfig{}, "", fmt.Errorf("server certificate and private key must both be configured")
		}
		if err := opcua.ValidateServerPEMPair(cfg.ServerCertPEM, cfg.ServerKeyPEM); err != nil {
			return model.OPCUAConfig{}, "", err
		}
	}

	if err := nm.saveConfig(); err != nil {
		return model.OPCUAConfig{}, "", err
	}

	server, exists := nm.opcuaServers[cfg.ID]

	if !cfg.Enable {
		if exists {
			server.Stop()
			delete(nm.opcuaServers, cfg.ID)
		}
		return cfg, "", nil
	}

	var startErr error
	if !exists {
		newServer := nm.newOPCUAServer(cfg)
		startErr = newServer.Start()
		if startErr == nil {
			nm.opcuaServers[cfg.ID] = newServer
		}
	} else {
		startErr = server.UpdateConfig(cfg)
	}

	return cfg, connectorStartWarning("OPC UA 服务", cfg.Name, startErr), nil
}

// UpdateOPCUACertificates updates server/trusted PEM blobs and restarts the OPC UA server.
func (nm *NorthboundManager) UpdateOPCUACertificates(id string, certPEM, keyPEM string, trusted []string) (model.OPCUAConfig, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	idx := -1
	for i, c := range nm.config.OPCUA {
		if c.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return model.OPCUAConfig{}, fmt.Errorf("OPC UA server %s not found", id)
	}

	cfg := nm.config.OPCUA[idx]
	if strings.TrimSpace(certPEM) != "" {
		cfg.ServerCertPEM = certPEM
	}
	if strings.TrimSpace(keyPEM) != "" {
		cfg.ServerKeyPEM = keyPEM
	}
	if trusted != nil {
		cfg.TrustedCertsPEM = trusted
	}
	nm.config.OPCUA[idx] = cfg

	if strings.TrimSpace(cfg.ServerCertPEM) != "" || strings.TrimSpace(cfg.ServerKeyPEM) != "" {
		if strings.TrimSpace(cfg.ServerCertPEM) == "" || strings.TrimSpace(cfg.ServerKeyPEM) == "" {
			return model.OPCUAConfig{}, fmt.Errorf("server certificate and private key must both be configured")
		}
		if err := opcua.ValidateServerPEMPair(cfg.ServerCertPEM, cfg.ServerKeyPEM); err != nil {
			return model.OPCUAConfig{}, err
		}
	}

	if err := nm.saveConfig(); err != nil {
		return model.OPCUAConfig{}, err
	}

	if server, ok := nm.opcuaServers[id]; ok {
		if err := server.UpdateConfig(cfg); err != nil {
			return model.OPCUAConfig{}, err
		}
	} else if cfg.Enable {
		newServer := nm.newOPCUAServer(cfg)
		if err := newServer.Start(); err != nil {
			return model.OPCUAConfig{}, err
		}
		nm.opcuaServers[id] = newServer
	}

	return model.SanitizeOPCUAForClient(cfg), nil
}

func (nm *NorthboundManager) DeleteOPCUAConfig(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if server, exists := nm.opcuaServers[id]; exists {
		server.Stop()
		delete(nm.opcuaServers, id)
	}

	newConfigs := []model.OPCUAConfig{}
	for _, c := range nm.config.OPCUA {
		if c.ID != id {
			newConfigs = append(newConfigs, c)
		}
	}
	nm.config.OPCUA = newConfigs

	return nm.saveConfig()
}

// BACnet Server Operations

// UpsertBACnetServerConfig 更新或插入 BACnet Server 配置
func (nm *NorthboundManager) UpsertBACnetServerConfig(cfg model.BACnetServerConfig) (model.BACnetServerConfig, string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if err := nm.validateNorthboundChannelName(cfg.ID, cfg.Name); err != nil {
		return model.BACnetServerConfig{}, "", err
	}

	found := false
	for i, c := range nm.config.BACnetServer {
		if c.ID == cfg.ID {
			nm.config.BACnetServer[i] = cfg
			found = true
			break
		}
	}
	if !found {
		nm.config.BACnetServer = append(nm.config.BACnetServer, cfg)
	}

	if err := nm.saveConfig(); err != nil {
		return model.BACnetServerConfig{}, "", err
	}

	server, exists := nm.bacnetServers[cfg.ID]

	if !cfg.Enable {
		if exists {
			server.Stop()
			delete(nm.bacnetServers, cfg.ID)
		}
		return cfg, "", nil
	}

	var startErr error
	if !exists {
		newServer := bacnet.NewServer(cfg, nm.sb)
		startErr = newServer.Start()
		if startErr == nil {
			nm.bacnetServers[cfg.ID] = newServer
		}
	} else {
		startErr = server.UpdateConfig(cfg)
	}

	return cfg, connectorStartWarning("BACnet Server", cfg.Name, startErr), nil
}

// DeleteBACnetServerConfig 删除 BACnet Server 配置
func (nm *NorthboundManager) DeleteBACnetServerConfig(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if server, exists := nm.bacnetServers[id]; exists {
		server.Stop()
		delete(nm.bacnetServers, id)
	}

	newConfigs := []model.BACnetServerConfig{}
	for _, c := range nm.config.BACnetServer {
		if c.ID != id {
			newConfigs = append(newConfigs, c)
		}
	}
	nm.config.BACnetServer = newConfigs

	return nm.saveConfig()
}

// SyncBACnetServer 同步指定 BACnet Server 的地址空间
func (nm *NorthboundManager) SyncBACnetServer(id string) error {
	nm.mu.RLock()
	var cfg model.BACnetServerConfig
	found := false
	for _, c := range nm.config.BACnetServer {
		if c.ID == id {
			cfg = c
			found = true
			break
		}
	}
	srv, running := nm.bacnetServers[id]
	nm.mu.RUnlock()

	if !found {
		return fmt.Errorf("BACnet Server config %s not found", id)
	}
	if !cfg.Enable {
		return fmt.Errorf("BACnet Server %s is disabled", id)
	}

	if !running {
		newServer := bacnet.NewServer(cfg, nm.sb)
		if err := newServer.Start(); err != nil {
			return err
		}
		nm.mu.Lock()
		nm.bacnetServers[id] = newServer
		nm.mu.Unlock()
		log.Printf("BACnet Server [%s] started during sync", cfg.Name)
		return nil
	}

	if err := srv.SyncAddressSpace(); err != nil {
		return fmt.Errorf("sync BACnet Server [%s]: %w", cfg.Name, err)
	}
	log.Printf("BACnet Server [%s] address space synced", cfg.Name)
	return nil
}

// RebuildBACnetServers 同步所有已启用 BACnet Server 的地址空间
func (nm *NorthboundManager) RebuildBACnetServers() {
	nm.mu.RLock()
	type rebuildItem struct {
		srv *bacnet.Server
		cfg model.BACnetServerConfig
	}
	var items []rebuildItem
	for _, cfg := range nm.config.BACnetServer {
		if !cfg.Enable {
			continue
		}
		if srv, ok := nm.bacnetServers[cfg.ID]; ok {
			items = append(items, rebuildItem{srv: srv, cfg: cfg})
		}
	}
	nm.mu.RUnlock()

	for _, item := range items {
		if err := item.srv.SyncAddressSpace(); err != nil {
			log.Printf("Failed to sync BACnet Server [%s]: %v", item.cfg.Name, err)
		} else {
			log.Printf("BACnet Server [%s] address space synced", item.cfg.Name)
		}
	}
}

// GetBACnetServerStats 获取 BACnet Server 统计信息
func (nm *NorthboundManager) GetBACnetServerStats(id string) (bacnet.Stats, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if server, ok := nm.bacnetServers[id]; ok {
		return server.GetStats(), nil
	}
	return bacnet.Stats{}, fmt.Errorf("BACnet Server %s not found or not running", id)
}

// GetBACnetServerWriteHistory 获取 BACnet Server 写入历史
func (nm *NorthboundManager) GetBACnetServerWriteHistory(id string, limit int) ([]bacnet.WriteHistoryItem, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if server, ok := nm.bacnetServers[id]; ok {
		return server.GetWriteHistory(limit), nil
	}
	return nil, fmt.Errorf("BACnet Server %s not found or not running", id)
}

func (nm *NorthboundManager) saveConfig() error {
	if nm.saveFunc == nil {
		return nil
	}
	cfg, err := model.NormalizeNorthboundForSave(nm.config)
	if err != nil {
		return err
	}
	return nm.saveFunc(cfg)
}

// UpdateConfig 更新北向配置并重启相关客户端/服务器
func (nm *NorthboundManager) UpdateConfig(newConfig model.NorthboundConfig) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// 保存旧配置用于比较
	oldConfig := nm.config

	// 更新配置
	nm.config = newConfig

	// 处理 MQTT 配置变更
	nm.updateMQTTClients(oldConfig.MQTT, newConfig.MQTT)

	// 处理 HTTP 配置变更
	nm.updateHTTPClients(oldConfig.HTTP, newConfig.HTTP)

	// 处理 OPC UA 配置变更
	nm.updateOPCUAServers(oldConfig.OPCUA, newConfig.OPCUA)

	// 处理 SparkplugB 配置变更
	nm.updateSparkplugBClients(oldConfig.SparkplugB, newConfig.SparkplugB)

	// 处理 edgeOS(MQTT) 配置变更
	nm.updateEdgeOSMQTTClients(oldConfig.EdgeOSMQTT, newConfig.EdgeOSMQTT)

	// 处理 edgeOS(NATS) 配置变更
	nm.updateEdgeOSNATSClients(oldConfig.EdgeOSNATS, newConfig.EdgeOSNATS)

	// 处理 BACnet Server 配置变更
	nm.updateBACnetServers(oldConfig.BACnetServer, newConfig.BACnetServer)
}

// updateMQTTClients 更新 MQTT 客户端
func (nm *NorthboundManager) updateMQTTClients(oldConfigs, newConfigs []model.MQTTConfig) {
	// 停止已删除或禁用的客户端
	for _, oldCfg := range oldConfigs {
		if client, exists := nm.mqttClients[oldCfg.ID]; exists {
			// 检查是否在新配置中
			found := false
			for _, newCfg := range newConfigs {
				if newCfg.ID == oldCfg.ID {
					found = true
					if !newCfg.Enable {
						client.Stop()
						delete(nm.mqttClients, oldCfg.ID)
					}
					break
				}
			}
			if !found {
				client.Stop()
				delete(nm.mqttClients, oldCfg.ID)
			}
		}
	}

	// 启动或更新新的客户端
	for _, newCfg := range newConfigs {
		if newCfg.Enable {
			if client, exists := nm.mqttClients[newCfg.ID]; exists {
				// 更新现有客户端
				client.UpdateConfig(newCfg)
			} else {
				// 创建新客户端
				client := mqtt.NewClient(newCfg, nm.sb, nm.storage)
				if err := client.Start(); err != nil {
					log.Printf("Failed to start MQTT client [%s]: %v", newCfg.Name, err)
				} else {
					log.Printf("Northbound MQTT client [%s] started", newCfg.Name)
					nm.mqttClients[newCfg.ID] = client
				}
			}
		}
	}
}

// updateHTTPClients 更新 HTTP 客户端
func (nm *NorthboundManager) updateHTTPClients(oldConfigs, newConfigs []model.HTTPConfig) {
	// 停止已删除或禁用的客户端
	for _, oldCfg := range oldConfigs {
		if client, exists := nm.httpClients[oldCfg.ID]; exists {
			// 检查是否在新配置中
			found := false
			for _, newCfg := range newConfigs {
				if newCfg.ID == oldCfg.ID {
					found = true
					if !newCfg.Enable {
						client.Stop()
						delete(nm.httpClients, oldCfg.ID)
					}
					break
				}
			}
			if !found {
				client.Stop()
				delete(nm.httpClients, oldCfg.ID)
			}
		}
	}

	// 启动或更新新的客户端
	for _, newCfg := range newConfigs {
		if newCfg.Enable {
			if client, exists := nm.httpClients[newCfg.ID]; exists {
				// 更新现有客户端
				client.UpdateConfig(newCfg)
			} else {
				// 创建新客户端
				client := http.NewClient(newCfg, nm.storage)
				client.Start()
				nm.httpClients[newCfg.ID] = client
				log.Printf("Northbound HTTP client [%s] started", newCfg.Name)
			}
		}
	}
}

// RebuildOPCUAServers 同步所有已启用 OPC UA Server 的地址空间（默认不停服；
// 结构性配置变更才 Stop+Start）。不影响网关主进程与南向采集。
func (nm *NorthboundManager) RebuildOPCUAServers() {
	nm.mu.RLock()
	type rebuildItem struct {
		srv *opcua.Server
		cfg model.OPCUAConfig
	}
	var items []rebuildItem
	for _, cfg := range nm.config.OPCUA {
		if !cfg.Enable {
			continue
		}
		if srv, ok := nm.opcuaServers[cfg.ID]; ok {
			items = append(items, rebuildItem{srv: srv, cfg: cfg})
		}
	}
	nm.mu.RUnlock()

	for _, item := range items {
		if err := item.srv.SyncAddressSpace(); err != nil {
			log.Printf("Failed to sync OPC UA server [%s]: %v", item.cfg.Name, err)
		} else {
			log.Printf("OPC UA server [%s] address space synced", item.cfg.Name)
		}
	}
}

// SyncOPCUAServer 同步指定 OPC UA Server 的地址空间（南向点位映射等变更）。
func (nm *NorthboundManager) SyncOPCUAServer(id string) error {
	nm.mu.RLock()
	var cfg model.OPCUAConfig
	found := false
	for _, c := range nm.config.OPCUA {
		if c.ID == id {
			cfg = c
			found = true
			break
		}
	}
	srv, running := nm.opcuaServers[id]
	nm.mu.RUnlock()

	if !found {
		return fmt.Errorf("OPC UA server config %s not found", id)
	}
	if !cfg.Enable {
		return fmt.Errorf("OPC UA server %s is disabled", id)
	}

	if !running {
		newServer := nm.newOPCUAServer(cfg)
		if err := newServer.Start(); err != nil {
			return err
		}
		nm.mu.Lock()
		nm.opcuaServers[id] = newServer
		nm.mu.Unlock()
		log.Printf("OPC UA server [%s] started during sync", cfg.Name)
		return nil
	}

	if err := srv.SyncAddressSpace(); err != nil {
		return fmt.Errorf("sync OPC UA server [%s]: %w", cfg.Name, err)
	}
	log.Printf("OPC UA server [%s] address space synced", cfg.Name)
	return nil
}

// updateOPCUAServers 更新 OPC UA 服务器
func (nm *NorthboundManager) updateOPCUAServers(oldConfigs, newConfigs []model.OPCUAConfig) {
	// 停止已删除或禁用的服务器
	for _, oldCfg := range oldConfigs {
		if server, exists := nm.opcuaServers[oldCfg.ID]; exists {
			// 检查是否在新配置中
			found := false
			for _, newCfg := range newConfigs {
				if newCfg.ID == oldCfg.ID {
					found = true
					if !newCfg.Enable {
						server.Stop()
						delete(nm.opcuaServers, oldCfg.ID)
					}
					break
				}
			}
			if !found {
				server.Stop()
				delete(nm.opcuaServers, oldCfg.ID)
			}
		}
	}

	// 启动或更新新的服务器
	for _, newCfg := range newConfigs {
		if newCfg.Enable {
			if server, exists := nm.opcuaServers[newCfg.ID]; exists {
				// 更新现有服务器
				server.UpdateConfig(newCfg)
			} else {
				// 创建新服务器
				server := nm.newOPCUAServer(newCfg)
				if err := server.Start(); err != nil {
					log.Printf("Failed to start OPC UA server [%s]: %v", newCfg.Name, err)
				} else {
					log.Printf("Northbound OPC UA server [%s] started", newCfg.Name)
					nm.opcuaServers[newCfg.ID] = server
				}
			}
		}
	}
}

// updateSparkplugBClients 更新 SparkplugB 客户端
func (nm *NorthboundManager) updateSparkplugBClients(oldConfigs, newConfigs []model.SparkplugBConfig) {
	// 停止已删除或禁用的客户端
	for _, oldCfg := range oldConfigs {
		if client, exists := nm.sparkplugClients[oldCfg.ID]; exists {
			// 检查是否在新配置中
			found := false
			for _, newCfg := range newConfigs {
				if newCfg.ID == oldCfg.ID {
					found = true
					if !newCfg.Enable {
						client.Stop()
						delete(nm.sparkplugClients, oldCfg.ID)
					}
					break
				}
			}
			if !found {
				client.Stop()
				delete(nm.sparkplugClients, oldCfg.ID)
			}
		}
	}

	// 启动或更新新的客户端
	for _, newCfg := range newConfigs {
		if newCfg.Enable {
			if client, exists := nm.sparkplugClients[newCfg.ID]; exists {
				// 更新现有客户端
				client.UpdateConfig(newCfg)
			} else {
				// 创建新客户端
				client := sparkplugb.NewClient(newCfg)
				if err := client.Start(); err != nil {
					log.Printf("Failed to start Sparkplug B client [%s]: %v", newCfg.Name, err)
				} else {
					log.Printf("Northbound Sparkplug B client [%s] started", newCfg.Name)
				}
				nm.sparkplugClients[newCfg.ID] = client
			}
		}
	}
}

// updateBACnetServers 更新 BACnet Server
func (nm *NorthboundManager) updateBACnetServers(oldConfigs, newConfigs []model.BACnetServerConfig) {
	// 停止已删除或禁用的 Server
	for _, oldCfg := range oldConfigs {
		if server, exists := nm.bacnetServers[oldCfg.ID]; exists {
			found := false
			for _, newCfg := range newConfigs {
				if newCfg.ID == oldCfg.ID {
					found = true
					if !newCfg.Enable {
						server.Stop()
						delete(nm.bacnetServers, oldCfg.ID)
					}
					break
				}
			}
			if !found {
				server.Stop()
				delete(nm.bacnetServers, oldCfg.ID)
			}
		}
	}

	// 启动或更新新的 Server
	for _, newCfg := range newConfigs {
		if newCfg.Enable {
			if server, exists := nm.bacnetServers[newCfg.ID]; exists {
				server.UpdateConfig(newCfg)
			} else {
				server := bacnet.NewServer(newCfg, nm.sb)
				if err := server.Start(); err != nil {
					log.Printf("Failed to start BACnet Server [%s]: %v", newCfg.Name, err)
				} else {
					log.Printf("Northbound BACnet Server [%s] started", newCfg.Name)
					nm.bacnetServers[newCfg.ID] = server
				}
			}
		}
	}
}
