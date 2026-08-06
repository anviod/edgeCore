package sparkplugb

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/northbound/reconnect"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	StatusDisconnected = 0
	StatusConnected    = 1
	StatusReconnecting = 2
	StatusError        = 3
)

type Client struct {
	config   model.SparkplugBConfig
	configMu sync.RWMutex
	client   mqtt.Client
	status   int
	statusMu sync.RWMutex
	stopChan chan struct{}

	lastValues sync.Map

	periodicMu sync.Mutex
	periodic   map[string]*periodicItem

	reconnectSched reconnect.Scheduler
}

type periodicItem struct {
	values map[string]model.Value
	ticker *time.Ticker
	stop   chan struct{}
}

func NewClient(cfg model.SparkplugBConfig) *Client {
	return &Client{
		config:   cfg,
		stopChan: make(chan struct{}),
		periodic: make(map[string]*periodicItem),
	}
}

func (c *Client) GetStatus() int {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

func (c *Client) setStatus(s int) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.status = s
}

func (c *Client) Start() error {
	if !c.config.Enable {
		return nil
	}

	c.setStatus(StatusReconnecting)
	if err := c.setupClient(); err != nil {
		c.setStatus(StatusError)
		return err
	}

	var connectErr error
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		connectErr = token.Error()
		log.Printf("Sparkplug B initial connection failed: %v", connectErr)
		c.setStatus(StatusDisconnected)
	}

	go c.retryLoop()

	if connectErr != nil {
		c.scheduleReconnect()
		return connectErr
	}
	return nil
}

func (c *Client) setupClient() error {
	opts := mqtt.NewClientOptions()
	broker := fmt.Sprintf("tcp://%s:%d", c.config.Broker, c.config.Port)
	if c.config.SSL {
		broker = fmt.Sprintf("ssl://%s:%d", c.config.Broker, c.config.Port)
		tlsConfig, err := c.createTLSConfig()
		if err != nil {
			return fmt.Errorf("TLS config: %w", err)
		}
		opts.SetTLSConfig(tlsConfig)
	}
	opts.AddBroker(broker)
	opts.SetClientID(c.config.ClientID)
	opts.SetUsername(c.config.Username)
	opts.SetPassword(c.config.Password)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)

	deathTopic := fmt.Sprintf("spBv1.0/%s/NDEATH/%s", c.config.GroupID, c.config.NodeID)
	deathPayload := c.createDeathPayload()
	opts.SetWill(deathTopic, string(deathPayload), 0, false)

	opts.SetOnConnectHandler(c.onConnect)
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		c.onConnectionLost(client, err)
		c.scheduleReconnect()
	})

	c.client = mqtt.NewClient(opts)
	return nil
}

func (c *Client) scheduleReconnect() {
	if !c.reconnectSched.TryStart() {
		return
	}
	go c.reconnectLogic()
}

func (c *Client) reconnectLogic() {
	defer c.reconnectSched.Done()

	var logThrottle reconnect.LogThrottle
	retryCount := 0

	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		if c.client != nil && c.client.IsConnected() {
			return
		}

		if c.client == nil {
			if err := c.setupClient(); err != nil {
				c.setStatus(StatusError)
				return
			}
		}

		c.setStatus(StatusReconnecting)
		attempt := retryCount + 1
		broker := fmt.Sprintf("%s:%d", c.config.Broker, c.config.Port)

		if logThrottle.ShouldLog(attempt, 30*time.Second, 10) {
			log.Printf("Sparkplug B reconnect attempt %d to %s", attempt, broker)
		}

		token := c.client.Connect()
		if token.Wait() && token.Error() == nil {
			log.Printf("Sparkplug B reconnected to %s", broker)
			return
		}

		retryCount++
		delay := reconnect.Backoff(retryCount)

		if retryCount <= 10 {
			if logThrottle.ShouldLog(retryCount, 30*time.Second, 10) {
				log.Printf("Sparkplug B reconnect failed (attempt %d), retrying in %v", retryCount, delay)
			}
		} else {
			c.setStatus(StatusError)
			if logThrottle.ShouldLog(retryCount, 60*time.Second, 10) {
				log.Printf("Sparkplug B reconnect failed repeatedly (attempt %d), backing off %v", retryCount, delay)
			}
			retryCount = 0
		}

		select {
		case <-c.stopChan:
			return
		case <-time.After(delay):
		}
	}
}

func (c *Client) retryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if c.GetStatus() == StatusDisconnected || c.GetStatus() == StatusError {
				c.scheduleReconnect()
			}
		}
	}
}

func (c *Client) Stop() {
	select {
	case <-c.stopChan:
	default:
		close(c.stopChan)
	}

	if c.client != nil && c.client.IsConnected() {
		deathTopic := fmt.Sprintf("spBv1.0/%s/NDEATH/%s", c.config.GroupID, c.config.NodeID)
		deathPayload := c.createDeathPayload()
		c.client.Publish(deathTopic, 0, false, deathPayload).Wait()
		c.client.Disconnect(250)
	}
	c.stopPeriodicTasks()
	c.setStatus(StatusDisconnected)
}

func (c *Client) UpdateConfig(cfg model.SparkplugBConfig) error {
	c.Stop()
	c.config = cfg
	c.stopChan = make(chan struct{})
	c.periodic = make(map[string]*periodicItem)
	return c.Start()
}

func (c *Client) Publish(v model.Value) {
	c.configMu.RLock()
	devCfg, ok := model.LookupNorthboundPublishConfig(v.DeviceID, c.config.Devices, c.config.VirtualDevices)
	c.configMu.RUnlock()
	if !ok {
		return
	}

	if devCfg.Strategy == "cov" || devCfg.Strategy == "change" {
		key := v.DeviceID + ":" + v.PointID
		lastVal, loaded := c.lastValues.Load(key)
		if loaded && lastVal == v.Value {
			return
		}
		c.lastValues.Store(key, v.Value)
	} else if devCfg.Strategy == "periodic" && time.Duration(devCfg.Interval) > 0 {
		c.periodicMu.Lock()
		if item, exists := c.periodic[v.DeviceID]; exists {
			item.values[v.PointID] = v
		}
		c.periodicMu.Unlock()
		return
	}

	if c.client == nil || !c.client.IsConnected() {
		return
	}

	c.publishValue(v)
}

func (c *Client) publishValue(v model.Value) {
	topic := fmt.Sprintf("spBv1.0/%s/DDATA/%s/%s", c.config.GroupID, c.config.NodeID, v.DeviceID)
	payload, err := c.createDataPayload(v)
	if err != nil {
		log.Printf("Error creating Sparkplug B payload: %v", err)
		return
	}
	token := c.client.Publish(topic, 0, false, payload)
	if token.Wait() && token.Error() != nil {
		log.Printf("Error publishing to Sparkplug B: %v", token.Error())
	}
}

func (c *Client) updatePeriodicTasks() {
	c.periodicMu.Lock()
	defer c.periodicMu.Unlock()

	c.configMu.RLock()
	devices := c.config.Devices
	virtualDevices := c.config.VirtualDevices
	c.configMu.RUnlock()

	isPeriodicEnabled := func(devID string) bool {
		cfg, ok := model.LookupNorthboundPublishConfig(devID, devices, virtualDevices)
		return ok && cfg.Enable && cfg.Strategy == "periodic" && time.Duration(cfg.Interval) > 0
	}

	for devID, item := range c.periodic {
		if !isPeriodicEnabled(devID) {
			close(item.stop)
			item.ticker.Stop()
			delete(c.periodic, devID)
		}
	}

	startPeriodic := func(devID string, devCfg model.DevicePublishConfig) {
		if !devCfg.Enable || devCfg.Strategy != "periodic" || time.Duration(devCfg.Interval) <= 0 {
			return
		}
		if _, exists := c.periodic[devID]; exists {
			return
		}
		item := &periodicItem{
			values: make(map[string]model.Value),
			ticker: time.NewTicker(time.Duration(devCfg.Interval)),
			stop:   make(chan struct{}),
		}
		c.periodic[devID] = item
		go c.runPeriodicTask(devID, item)
	}

	for devID, devCfg := range devices {
		startPeriodic(devID, devCfg)
	}
	for devID, devCfg := range virtualDevices {
		startPeriodic(devID, devCfg)
	}
}

func (c *Client) stopPeriodicTasks() {
	c.periodicMu.Lock()
	defer c.periodicMu.Unlock()
	for devID, item := range c.periodic {
		close(item.stop)
		item.ticker.Stop()
		delete(c.periodic, devID)
	}
}

func (c *Client) runPeriodicTask(deviceID string, item *periodicItem) {
	for {
		select {
		case <-item.stop:
			return
		case <-c.stopChan:
			return
		case <-item.ticker.C:
			c.flushPeriodic(deviceID, item)
		}
	}
}

func (c *Client) flushPeriodic(deviceID string, item *periodicItem) {
	c.periodicMu.Lock()
	if len(item.values) == 0 {
		c.periodicMu.Unlock()
		return
	}
	values := make([]model.Value, 0, len(item.values))
	for _, v := range item.values {
		values = append(values, v)
	}
	c.periodicMu.Unlock()

	for _, v := range values {
		c.publishValue(v)
	}
}

func (c *Client) onConnect(client mqtt.Client) {
	c.setStatus(StatusConnected)
	log.Println("Sparkplug B Connected")

	birthTopic := fmt.Sprintf("spBv1.0/%s/NBIRTH/%s", c.config.GroupID, c.config.NodeID)
	birthPayload := c.createBirthPayload()
	client.Publish(birthTopic, 0, false, birthPayload)

	c.updatePeriodicTasks()
}

func (c *Client) onConnectionLost(client mqtt.Client, err error) {
	c.setStatus(StatusDisconnected)
	log.Printf("Sparkplug B Connection Lost: %v", err)
}

func (c *Client) createDeathPayload() []byte {
	m := map[string]interface{}{
		"timestamp": time.Now().UnixMilli(),
		"metrics": []map[string]interface{}{
			{
				"name":  "bdSeq",
				"type":  "UInt64",
				"value": 0,
			},
		},
	}
	b, _ := json.Marshal(m)
	return b
}

func (c *Client) createBirthPayload() []byte {
	m := map[string]interface{}{
		"timestamp": time.Now().UnixMilli(),
		"metrics": []map[string]interface{}{
			{
				"name":  "bdSeq",
				"type":  "UInt64",
				"value": 0,
			},
			{
				"name":  "Node Control/Rebirth",
				"type":  "Boolean",
				"value": false,
			},
		},
	}
	b, _ := json.Marshal(m)
	return b
}

func (c *Client) createDataPayload(v model.Value) ([]byte, error) {
	m := map[string]interface{}{
		"timestamp": v.TS.UnixMilli(),
		"metrics": []map[string]interface{}{
			{
				"name":  v.PointID,
				"type":  "String",
				"value": v.Value,
			},
			{
				"name":  "location",
				"type":  "String",
				"value": "sh",
			},
			{
				"name":  "number",
				"type":  "String",
				"value": "12345613",
			},
		},
	}
	return json.Marshal(m)
}

func (c *Client) createTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
	}

	if c.config.CACert != "" {
		caCert, err := os.ReadFile(c.config.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %v", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	if c.config.ClientCert != "" && c.config.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(c.config.ClientCert, c.config.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client keypair: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}
