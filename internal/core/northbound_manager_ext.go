package core

import (
	"fmt"
	"strings"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/model"
	"github.com/anviod/edgex/internal/northbound/edgos_mqtt"
	"github.com/anviod/edgex/internal/northbound/edgos_nats"
	"github.com/anviod/edgex/internal/northbound/http"
	"github.com/anviod/edgex/internal/northbound/mqtt"
)

func (nm *NorthboundManager) UpsertHTTPConfig(cfg model.HTTPConfig) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if err := nm.validateNorthboundChannelName(cfg.ID, cfg.Name); err != nil {
		return err
	}

	var oldCfg model.HTTPConfig
	found := false
	for i, c := range nm.config.HTTP {
		if c.ID == cfg.ID {
			oldCfg = c
			nm.config.HTTP[i] = cfg
			found = true
			break
		}
	}
	if !found {
		nm.config.HTTP = append(nm.config.HTTP, cfg)
	}

	// Diff Logic
	addedDevices := []string{}
	removedDevices := []string{}

	if found {
		for dID, devCfg := range cfg.Devices {
			if devCfg.Enable {
				if oldDevCfg, ok := oldCfg.Devices[dID]; !ok || !oldDevCfg.Enable {
					addedDevices = append(addedDevices, dID)
				}
			}
		}
		for dID, devCfg := range oldCfg.Devices {
			if devCfg.Enable {
				if newDevCfg, ok := cfg.Devices[dID]; !ok || !newDevCfg.Enable {
					removedDevices = append(removedDevices, dID)
				}
			}
		}
	} else {
		for dID, devCfg := range cfg.Devices {
			if devCfg.Enable {
				addedDevices = append(addedDevices, dID)
			}
		}
	}

	if err := nm.saveConfig(); err != nil {
		return err
	}

	client, exists := nm.httpClients[cfg.ID]
	if !cfg.Enable {
		if exists {
			client.Stop()
			delete(nm.httpClients, cfg.ID)
		}
		return nil
	}

	var targetClient *http.Client
	if !exists {
		newClient := http.NewClient(cfg, nm.storage)
		newClient.Start()
		nm.httpClients[cfg.ID] = newClient
		targetClient = newClient
	} else {
		client.UpdateConfig(cfg)
		targetClient = client
	}

	// Fire Events
	if targetClient != nil {
		for _, dID := range addedDevices {
			if dev := nm.findDevice(dID); dev != nil {
				targetClient.PublishDeviceLifecycle("add", *dev.(*model.Device))
			}
		}
		for _, dID := range removedDevices {
			if dev := nm.findDevice(dID); dev != nil {
				targetClient.PublishDeviceLifecycle("remove", *dev.(*model.Device))
			} else {
				targetClient.PublishDeviceLifecycle("remove", model.Device{ID: dID})
			}
		}
	}
	return nil
}

// SetChannelManager sets the ChannelManager reference for device lookups
func (nm *NorthboundManager) SetChannelManager(cm *ChannelManager) {
	nm.cm = cm
}

// BindShadowCore attaches ShadowCore → EAN Event publishing on the hot path.
// Safe to call multiple times; subscription is registered once.
func (nm *NorthboundManager) BindShadowCore(sc *ShadowCore) {
	if nm == nil || sc == nil {
		return
	}
	nm.mu.Lock()
	nm.shadowCore = sc
	if nm.eanShadowBridge == nil {
		nm.eanShadowBridge = capability.NewShadowEventBridge()
	}
	nm.mu.Unlock()

	nm.eanShadowOnce.Do(func() {
		sc.Subscribe(func(shadowDeviceID string, points map[string]model.ShadowPoint) {
			nm.onShadowDelta(shadowDeviceID, points)
		})
	})
	// Collect once at bind time; subsequent updates happen on EdgeOS client
	// connect/disconnect (not on every shadow delta).
	nm.refreshEANEventPublishers()
}

// RefreshEANEventPublishers re-collects EventPublishers from connected edgeOS clients.
func (nm *NorthboundManager) RefreshEANEventPublishers() {
	nm.refreshEANEventPublishers()
}

// wireEdgeOSMQTTClient hooks EAN runtime lifecycle so shadow event publishers
// refresh only when clients connect/disconnect — not on the shadow hot path.
func (nm *NorthboundManager) wireEdgeOSMQTTClient(client *edgos_mqtt.Client) {
	if client == nil {
		return
	}
	client.SetOnEANRuntimeChanged(func() {
		nm.refreshEANEventPublishers()
	})
}

// wireEdgeOSNATSClient hooks EAN runtime lifecycle so shadow event publishers
// refresh only when clients connect/disconnect — not on the shadow hot path.
func (nm *NorthboundManager) wireEdgeOSNATSClient(client *edgos_nats.Client) {
	if client == nil {
		return
	}
	client.SetOnEANRuntimeChanged(func() {
		nm.refreshEANEventPublishers()
	})
}

func (nm *NorthboundManager) refreshEANEventPublishers() {
	// Prefer sync refresh so connect/start paths close the publisher race window.
	// If a writer already holds nm.mu (Stop/UpdateConfig), defer a blocking
	// refresh instead of deadlocking.
	if !nm.mu.TryRLock() {
		go nm.refreshEANEventPublishersWait()
		return
	}
	nm.applyEANEventPublishersLocked()
}

func (nm *NorthboundManager) refreshEANEventPublishersWait() {
	nm.mu.RLock()
	nm.applyEANEventPublishersLocked()
}

// applyEANEventPublishersLocked requires nm.mu held for read and releases it
// before SetPublishers (bridge has its own lock).
func (nm *NorthboundManager) applyEANEventPublishersLocked() {
	bridge := nm.eanShadowBridge
	mqttClients := make([]*edgos_mqtt.Client, 0, len(nm.edgeOSMQTTClients))
	for _, c := range nm.edgeOSMQTTClients {
		mqttClients = append(mqttClients, c)
	}
	natsClients := make([]*edgos_nats.Client, 0, len(nm.edgeOSNATSClients))
	for _, c := range nm.edgeOSNATSClients {
		natsClients = append(natsClients, c)
	}
	nm.mu.RUnlock()
	if bridge == nil {
		return
	}
	pubs := make([]*capability.EventPublisher, 0, len(mqttClients)+len(natsClients))
	for _, c := range mqttClients {
		if rt := c.CapabilityRuntime(); rt != nil {
			pubs = append(pubs, rt.Events())
		}
	}
	for _, c := range natsClients {
		if rt := c.CapabilityRuntime(); rt != nil {
			pubs = append(pubs, rt.Events())
		}
	}
	bridge.SetPublishers(pubs)
}

func (nm *NorthboundManager) onShadowDelta(shadowDeviceID string, points map[string]model.ShadowPoint) {
	if len(points) == 0 {
		return
	}

	nm.mu.RLock()
	bridge := nm.eanShadowBridge
	sc := nm.shadowCore
	nm.mu.RUnlock()
	if bridge == nil {
		return
	}

	channelID, deviceID := "", shadowDeviceID
	if sc != nil {
		if cid, did, err := sc.ResolvePublishTarget(shadowDeviceID); err == nil {
			channelID, deviceID = cid, did
		}
	}
	if strings.HasPrefix(deviceID, "shadow-") {
		deviceID = strings.TrimPrefix(deviceID, "shadow-")
	}

	views := make(map[string]capability.ShadowPointView, len(points))
	for pointID, pt := range points {
		ts := pt.CollectedAt
		if ts.IsZero() {
			ts = pt.Timestamp
		}
		views[pointID] = capability.ShadowPointView{
			Value:         pt.Value,
			PreviousValue: pt.PreviousValue,
			Quality:       pt.Quality,
			TimestampMs:   ts.UnixMilli(),
		}
	}
	bridge.HandleDelta(deviceID, channelID, views)
}


// findDevice retrieves a device by ID from all channels via ChannelManager
func (nm *NorthboundManager) findDevice(dID string) any {
	if nm.cm == nil {
		return nil
	}

	nm.cm.mu.RLock()
	defer nm.cm.mu.RUnlock()

	// Search through all channels to find the device
	for _, ch := range nm.cm.channels {
		for i, dev := range ch.Devices {
			if dev.ID == dID {
				return &ch.Devices[i]
			}
		}
	}
	return nil
}

func (nm *NorthboundManager) DeleteHTTPConfig(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if client, exists := nm.httpClients[id]; exists {
		client.Stop()
		delete(nm.httpClients, id)
	}

	newConfigs := []model.HTTPConfig{}
	for _, c := range nm.config.HTTP {
		if c.ID != id {
			newConfigs = append(newConfigs, c)
		}
	}
	nm.config.HTTP = newConfigs

	return nm.saveConfig()
}

// PublishHTTP sends a raw payload via a specific HTTP config
func (nm *NorthboundManager) PublishHTTP(configID string, payload []byte) error {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if client, ok := nm.httpClients[configID]; ok {
		return client.Send(payload)
	}
	return fmt.Errorf("HTTP config %s not found or not running", configID)
}

// PublishMQTTClient publishes to a specific client
func (nm *NorthboundManager) PublishMQTTClient(clientID string, topic string, payload []byte) error {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if client, ok := nm.mqttClients[clientID]; ok {
		return client.PublishRaw(topic, payload)
	}
	return fmt.Errorf("MQTT client %s not found", clientID)
}

func (nm *NorthboundManager) UpsertMQTTConfig(cfg model.MQTTConfig) (string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if err := nm.validateNorthboundChannelName(cfg.ID, cfg.Name); err != nil {
		return "", err
	}

	var oldCfg model.MQTTConfig
	found := false
	for i, c := range nm.config.MQTT {
		if c.ID == cfg.ID {
			oldCfg = c
			nm.config.MQTT[i] = cfg
			found = true
			break
		}
	}
	if !found {
		nm.config.MQTT = append(nm.config.MQTT, cfg)
	}

	// Diff Logic - detect added and removed devices
	addedDevices := []string{}
	removedDevices := []string{}

	if found {
		for dID, devCfg := range cfg.Devices {
			if devCfg.Enable {
				if oldDevCfg, ok := oldCfg.Devices[dID]; !ok || !oldDevCfg.Enable {
					addedDevices = append(addedDevices, dID)
				}
			}
		}
		for dID, devCfg := range oldCfg.Devices {
			if devCfg.Enable {
				if newDevCfg, ok := cfg.Devices[dID]; !ok || !newDevCfg.Enable {
					removedDevices = append(removedDevices, dID)
				}
			}
		}
	} else {
		// New config - all enabled devices are "added"
		for dID, devCfg := range cfg.Devices {
			if devCfg.Enable {
				addedDevices = append(addedDevices, dID)
			}
		}
	}

	if err := nm.saveConfig(); err != nil {
		return "", err
	}

	client, exists := nm.mqttClients[cfg.ID]
	if !cfg.Enable {
		if exists {
			client.Stop()
			delete(nm.mqttClients, cfg.ID)
		}
		return "", nil
	}

	var startErr error
	var targetClient *mqtt.Client
	if !exists {
		newClient := mqtt.NewClient(cfg, nm.sb, nm.storage)
		startErr = newClient.Start()
		nm.mqttClients[cfg.ID] = newClient
		targetClient = newClient
	} else {
		startErr = client.UpdateConfig(cfg)
		targetClient = client
	}

	// Fire device lifecycle events
	if targetClient != nil {
		for _, dID := range addedDevices {
			if dev := nm.findDevice(dID); dev != nil {
				targetClient.PublishDeviceLifecycle("add", *dev.(*model.Device))
			}
		}
		for _, dID := range removedDevices {
			if dev := nm.findDevice(dID); dev != nil {
				targetClient.PublishDeviceLifecycle("remove", *dev.(*model.Device))
			} else {
				targetClient.PublishDeviceLifecycle("remove", model.Device{ID: dID})
			}
		}
	}
	return connectorStartWarning("MQTT Broker", cfg.Name, startErr), nil
}
