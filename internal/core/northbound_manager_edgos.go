package core

import (
	"fmt"
	"log"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/northbound/edgos_mqtt"
	"github.com/anviod/edgeCore/internal/northbound/edgos_nats"
)

// updateEdgeOSMQTTClients 更新 edgeOS(MQTT) 客户端
func (nm *NorthboundManager) updateEdgeOSMQTTClients(oldConfigs, newConfigs []model.EdgeOSMQTTConfig) {
	// 停止已删除或禁用的客户端
	for _, oldCfg := range oldConfigs {
		if client, exists := nm.edgeOSMQTTClients[oldCfg.ID]; exists {
			// 检查是否在新配置中
			found := false
			for _, newCfg := range newConfigs {
				if newCfg.ID == oldCfg.ID {
					found = true
					if !newCfg.Enable {
						client.Stop()
						delete(nm.edgeOSMQTTClients, oldCfg.ID)
					}
					break
				}
			}
			if !found {
				client.Stop()
				delete(nm.edgeOSMQTTClients, oldCfg.ID)
			}
		}
	}

	// 启动或更新新的客户端
	for _, newCfg := range newConfigs {
		if newCfg.Enable {
			if client, exists := nm.edgeOSMQTTClients[newCfg.ID]; exists {
				// 更新现有客户端
				client.UpdateConfig(newCfg)
			} else {
				// 创建新客户端
				client := edgos_mqtt.NewClient(newCfg, nm.sb, nm.storage)
				nm.wireEdgeOSMQTTClient(client)
				nm.edgeOSMQTTClients[newCfg.ID] = client
				if err := client.Start(); err != nil {
					log.Printf("Failed to start edgeOS(MQTT) client [%s]: %v", newCfg.Name, err)
				} else {
					log.Printf("Northbound edgeOS(MQTT) client [%s] started", newCfg.Name)
				}
			}
		}
	}
}

// updateEdgeOSNATSClients 更新 edgeOS(NATS) 客户端
func (nm *NorthboundManager) updateEdgeOSNATSClients(oldConfigs, newConfigs []model.EdgeOSNATSConfig) {
	// 停止已删除或禁用的客户端
	for _, oldCfg := range oldConfigs {
		if client, exists := nm.edgeOSNATSClients[oldCfg.ID]; exists {
			// 检查是否在新配置中
			found := false
			for _, newCfg := range newConfigs {
				if newCfg.ID == oldCfg.ID {
					found = true
					if !newCfg.Enable {
						client.Stop()
						delete(nm.edgeOSNATSClients, oldCfg.ID)
					}
					break
				}
			}
			if !found {
				client.Stop()
				delete(nm.edgeOSNATSClients, oldCfg.ID)
			}
		}
	}

	// 启动或更新新的客户端
	for _, newCfg := range newConfigs {
		if newCfg.Enable {
			if client, exists := nm.edgeOSNATSClients[newCfg.ID]; exists {
				// 更新现有客户端
				client.UpdateConfig(newCfg)
			} else {
				// 创建新客户端
				client := edgos_nats.NewClient(newCfg, nm.sb, nm.storage)
				nm.wireEdgeOSNATSClient(client)
				nm.edgeOSNATSClients[newCfg.ID] = client
				if err := client.Start(); err != nil {
					log.Printf("Failed to start edgeOS(NATS) client [%s]: %v", newCfg.Name, err)
				} else {
					log.Printf("Northbound edgeOS(NATS) client [%s] started", newCfg.Name)
				}
			}
		}
	}
}

// UpsertEdgeOSMQTTConfig 更新或插入 edgeOS(MQTT) 配置
func (nm *NorthboundManager) UpsertEdgeOSMQTTConfig(cfg model.EdgeOSMQTTConfig) (string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if err := nm.validateNorthboundChannelName(cfg.ID, cfg.Name); err != nil {
		return "", err
	}

	found := false
	for i, c := range nm.config.EdgeOSMQTT {
		if c.ID == cfg.ID {
			nm.config.EdgeOSMQTT[i] = cfg
			found = true
			break
		}
	}
	if !found {
		nm.config.EdgeOSMQTT = append(nm.config.EdgeOSMQTT, cfg)
	}

	if err := nm.saveConfig(); err != nil {
		return "", err
	}

	client, exists := nm.edgeOSMQTTClients[cfg.ID]

	if !cfg.Enable {
		if exists {
			client.Stop()
			delete(nm.edgeOSMQTTClients, cfg.ID)
		}
		return "", nil
	}

	var startErr error
	if !exists {
		newClient := edgos_mqtt.NewClient(cfg, nm.sb, nm.storage)
		nm.wireEdgeOSMQTTClient(newClient)
		nm.edgeOSMQTTClients[cfg.ID] = newClient
		startErr = newClient.Start()
	} else {
		startErr = client.UpdateConfig(cfg)
	}

	return connectorStartWarning("edgeOS MQTT Broker", cfg.Name, startErr), nil
}

// DeleteEdgeOSMQTTConfig 删除 edgeOS(MQTT) 配置
func (nm *NorthboundManager) DeleteEdgeOSMQTTConfig(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if client, exists := nm.edgeOSMQTTClients[id]; exists {
		client.Stop()
		delete(nm.edgeOSMQTTClients, id)
	}

	newConfigs := []model.EdgeOSMQTTConfig{}
	for _, c := range nm.config.EdgeOSMQTT {
		if c.ID != id {
			newConfigs = append(newConfigs, c)
		}
	}
	nm.config.EdgeOSMQTT = newConfigs

	return nm.saveConfig()
}

// GetEdgeOSMQTTStats 获取 edgeOS(MQTT) 统计信息
func (nm *NorthboundManager) GetEdgeOSMQTTStats(id string) (edgos_mqtt.EdgeOSMQTTStats, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if client, ok := nm.edgeOSMQTTClients[id]; ok {
		return client.GetStats(), nil
	}
	return edgos_mqtt.EdgeOSMQTTStats{}, fmt.Errorf("edgeOS(MQTT) client not found")
}

// UpsertEdgeOSNATSConfig 更新或插入 edgeOS(NATS) 配置
func (nm *NorthboundManager) UpsertEdgeOSNATSConfig(cfg model.EdgeOSNATSConfig) (string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if err := nm.validateNorthboundChannelName(cfg.ID, cfg.Name); err != nil {
		return "", err
	}

	found := false
	for i, c := range nm.config.EdgeOSNATS {
		if c.ID == cfg.ID {
			nm.config.EdgeOSNATS[i] = cfg
			found = true
			break
		}
	}
	if !found {
		nm.config.EdgeOSNATS = append(nm.config.EdgeOSNATS, cfg)
	}

	if err := nm.saveConfig(); err != nil {
		return "", err
	}

	client, exists := nm.edgeOSNATSClients[cfg.ID]

	if !cfg.Enable {
		if exists {
			client.Stop()
			delete(nm.edgeOSNATSClients, cfg.ID)
		}
		return "", nil
	}

	var startErr error
	if !exists {
		newClient := edgos_nats.NewClient(cfg, nm.sb, nm.storage)
		nm.wireEdgeOSNATSClient(newClient)
		nm.edgeOSNATSClients[cfg.ID] = newClient
		startErr = newClient.Start()
	} else {
		startErr = client.UpdateConfig(cfg)
	}

	return connectorStartWarning("NATS 服务", cfg.Name, startErr), nil
}

// DeleteEdgeOSNATSConfig 删除 edgeOS(NATS) 配置
func (nm *NorthboundManager) DeleteEdgeOSNATSConfig(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if client, exists := nm.edgeOSNATSClients[id]; exists {
		client.Stop()
		delete(nm.edgeOSNATSClients, id)
	}

	newConfigs := []model.EdgeOSNATSConfig{}
	for _, c := range nm.config.EdgeOSNATS {
		if c.ID != id {
			newConfigs = append(newConfigs, c)
		}
	}
	nm.config.EdgeOSNATS = newConfigs

	return nm.saveConfig()
}

// GetEdgeOSNATSStats 获取 edgeOS(NATS) 统计信息
func (nm *NorthboundManager) GetEdgeOSNATSStats(id string) (edgos_nats.EdgeOSNATSStats, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if client, ok := nm.edgeOSNATSClients[id]; ok {
		return client.GetStats(), nil
	}
	return edgos_nats.EdgeOSNATSStats{}, fmt.Errorf("edgeOS(NATS) client not found")
}

// PublishEdgeOSMQTT 发布消息到指定的 edgeOS(MQTT) 客户端
func (nm *NorthboundManager) PublishEdgeOSMQTT(clientID, topic string, payload []byte) error {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if client, ok := nm.edgeOSMQTTClients[clientID]; ok {
		return client.PublishRaw(topic, payload)
	}
	return fmt.Errorf("edgeOS(MQTT) client not found")
}

// PublishEdgeOSNATS 发布消息到指定的 edgeOS(NATS) 客户端
func (nm *NorthboundManager) PublishEdgeOSNATS(clientID, subject string, payload []byte) error {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if client, ok := nm.edgeOSNATSClients[clientID]; ok {
		return client.PublishRaw(subject, payload)
	}
	return fmt.Errorf("edgeOS(NATS) client not found")
}
