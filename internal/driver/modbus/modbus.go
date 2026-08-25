package modbus

import (
	"context"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("modbus-tcp", NewModbusDriver)
	driver.RegisterDriver("modbus-rtu", NewModbusDriver)
	driver.RegisterDriver("modbus-rtu-over-tcp", NewModbusDriver)
	// Backward compatibility aliases for legacy *-simple protocol names.
	driver.RegisterDriver("modbus-tcp-simple", NewModbusDriver)
	driver.RegisterDriver("modbus-rtu-simple", NewModbusDriver)
	driver.RegisterDriver("modbus-rtu-over-tcp-simple", NewModbusDriver)
}

type ModbusDriver struct {
	config              model.DriverConfig
	transport           *ModbusTransport
	scheduler           *PointScheduler
	stateMachine        *DeviceStateMachine
	connController      *core.ConnectionController
	slaveID             uint8
	connectionStartTime time.Time
	reconnectCount      int64
	lastDisconnectTime  time.Time
	mu                  sync.RWMutex
}

func NewModbusDriver() driver.Driver {
	return &ModbusDriver{
		stateMachine: NewDeviceStateMachine(),
	}
}

func (d *ModbusDriver) Init(config model.DriverConfig) error {
	d.config = config

	d.slaveID = 1
	if v, ok := config.Config["slave_id"]; ok {
		switch val := v.(type) {
		case int:
			d.slaveID = uint8(val)
		case float64:
			d.slaveID = uint8(val)
		}
	}

	byteOrder4 := "ABCD"
	if v, ok := config.Config["byteOrder"]; ok {
		byteOrder4 = v.(string)
	}

	batchSize := uint16(120)
	if v, ok := config.Config["batchSize"]; ok {
		if f, ok := v.(float64); ok {
			batchSize = uint16(f)
		} else if i, ok := v.(int); ok {
			batchSize = uint16(i)
		}
	}

	startAddress := 0
	if v, ok := config.Config["startAddress"]; ok {
		if f, ok := v.(float64); ok {
			startAddress = int(f)
		} else if i, ok := v.(int); ok {
			startAddress = i
		}
	}

	addressBase := uint16(0)
	if v, ok := config.Config["start_address"]; ok {
		switch val := v.(type) {
		case int:
			addressBase = uint16(val)
		case float64:
			addressBase = uint16(val)
		}
	} else if v, ok := config.Config["address_base"]; ok {
		switch val := v.(type) {
		case int:
			addressBase = uint16(val)
		case float64:
			addressBase = uint16(val)
		}
	}

	instructionInterval := 10 * time.Millisecond
	if v, ok := config.Config["instructionInterval"]; ok {
		if f, ok := v.(float64); ok {
			instructionInterval = time.Duration(f) * time.Millisecond
		} else if i, ok := v.(int); ok {
			instructionInterval = time.Duration(i) * time.Millisecond
		}
	}

	d.transport = NewModbusTransport(config)

	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		d.transport.SetMetricsRecorder(mc, config.ChannelID)
	}

	d.transport.SetUnitID(d.slaveID)

	decoder := NewPointDecoder(byteOrder4, startAddress, addressBase)

	if v, ok := config.Config["use_dataformat_decoder"]; ok {
		switch val := v.(type) {
		case bool:
			decoder.EnableDataformatDecoder(val)
		case string:
			if val == "true" || val == "1" {
				decoder.EnableDataformatDecoder(true)
			}
		}
	}

	d.scheduler = NewPointScheduler(d.transport, decoder, 125, batchSize, instructionInterval)
	d.scheduler.SetSlaveID(d.slaveID)

	d.connController = core.NewConnectionController("modbus", config.ChannelID, config.Protocol)

	return nil
}

func (d *ModbusDriver) performMTUProbe() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if mtu, err := d.transport.DetectMTU(ctx); err == nil {
		d.scheduler.SetMaxPacketSize(mtu)
		zap.L().Info("[Modbus] MTU探测成功",
			zap.String("channelID", d.config.ChannelID),
			zap.Uint16("maxRegisters", mtu),
		)
	} else {
		zap.L().Warn("[Modbus] MTU探测失败，使用默认值",
			zap.String("channelID", d.config.ChannelID),
			zap.Error(err),
		)
	}
}

func (d *ModbusDriver) Connect(ctx context.Context) error {
	d.connectionStartTime = time.Now()
	d.reconnectCount++

	if err := d.transport.Connect(ctx); err != nil {
		return err
	}
	// Probe only after the transport is connected. Starting this from Init
	// races with configuration/scheduler setup and can probe test hooks before
	// the caller has finished wiring the driver.
	go d.performMTUProbe()
	return nil
}

func (d *ModbusDriver) Disconnect() error {
	d.lastDisconnectTime = time.Now()
	return d.transport.Disconnect()
}

func (d *ModbusDriver) Health() driver.HealthStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.transport.IsConnected() {
		return driver.HealthStatusGood
	}

	if d.transport.IsReconnectExhausted() {
		return driver.HealthStatusBad
	}

	return driver.HealthStatusUnknown
}

func (d *ModbusDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if len(points) == 0 {
		return make(map[string]model.Value), nil
	}

	if d.stateMachine.GetState() == StateProbing {
		results, err := d.scheduler.Read(ctx, points)
		return results, err
	}

	results, err := d.scheduler.Read(ctx, points)

	if err != nil {
		if d.connController.IsConnectionFailure(err) {
			zap.L().Warn("[Modbus] 连接失败，记录连接错误",
				zap.String("channelID", d.config.ChannelID),
				zap.Error(err),
			)
			return results, err
		}

		if d.connController.IsReadFailure(err) {
			zap.L().Debug("[Modbus] 读取失败，记录读取错误",
				zap.String("channelID", d.config.ChannelID),
				zap.Error(err),
			)
			d.connController.RecordReadFailure()
			return results, err
		}

		d.connController.RecordReadFailure()
		return results, err
	}

	d.connController.RecordReadSuccess()
	return results, nil
}

func (d *ModbusDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	err := d.scheduler.Write(ctx, point, value)
	if err != nil {
		if d.connController.IsConnectionFailure(err) {
			return err
		}
		d.connController.RecordReadFailure()
		return err
	}

	d.connController.RecordReadSuccess()
	return nil
}

func (d *ModbusDriver) SetSlaveID(slaveID uint8) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.slaveID = slaveID
	d.transport.SetUnitID(slaveID)
	if d.scheduler != nil {
		d.scheduler.SetSlaveID(slaveID)
	}

	return nil
}

func (d *ModbusDriver) SetDeviceConfig(config map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if v, ok := config["slave_id"]; ok {
		switch val := v.(type) {
		case int:
			d.slaveID = uint8(val)
		case float64:
			d.slaveID = uint8(val)
		}
		d.transport.SetUnitID(d.slaveID)
	}

	addressBase := uint16(0)
	if v, ok := config["start_address"]; ok {
		switch val := v.(type) {
		case int:
			addressBase = uint16(val)
		case float64:
			addressBase = uint16(val)
		}
	} else if v, ok := config["address_base"]; ok {
		switch val := v.(type) {
		case int:
			addressBase = uint16(val)
		case float64:
			addressBase = uint16(val)
		}
	} else {
		if v, ok := d.config.Config["start_address"]; ok {
			switch val := v.(type) {
			case int:
				addressBase = uint16(val)
			case float64:
				addressBase = uint16(val)
			}
		} else if v, ok := d.config.Config["address_base"]; ok {
			switch val := v.(type) {
			case int:
				addressBase = uint16(val)
			case float64:
				addressBase = uint16(val)
			}
		}
	}

	byteOrder4 := "ABCD"
	if v, ok := d.config.Config["byteOrder"]; ok {
		byteOrder4 = v.(string)
	}

	startAddress := 0
	if v, ok := d.config.Config["startAddress"]; ok {
		if f, ok := v.(float64); ok {
			startAddress = int(f)
		} else if i, ok := v.(int); ok {
			startAddress = i
		}
	}

	decoder := NewPointDecoder(byteOrder4, startAddress, addressBase)

	batchSize := uint16(120)
	if v, ok := config["batchSize"]; ok {
		switch val := v.(type) {
		case int:
			batchSize = uint16(val)
		case float64:
			batchSize = uint16(val)
		}
	}

	d.scheduler = NewPointScheduler(d.transport, decoder, 125, batchSize, 10*time.Millisecond)
	d.scheduler.SetSlaveID(d.slaveID)
	applySchedulerIOConfig(d.scheduler, config)

	return nil
}

func applySchedulerIOConfig(scheduler *PointScheduler, config map[string]any) {
	if scheduler == nil || config == nil {
		return
	}
	if v, ok := config["max_gap"]; ok {
		switch val := v.(type) {
		case int:
			scheduler.SetGroupThreshold(uint16(val))
		case float64:
			scheduler.SetGroupThreshold(uint16(val))
		}
	}
	if v, ok := config["group_threshold"]; ok {
		switch val := v.(type) {
		case int:
			scheduler.SetGroupThreshold(uint16(val))
		case float64:
			scheduler.SetGroupThreshold(uint16(val))
		}
	}
	if v, ok := config["batchSize"]; ok {
		switch val := v.(type) {
		case int:
			scheduler.SetMaxPacketSize(uint16(val))
		case float64:
			scheduler.SetMaxPacketSize(uint16(val))
		}
	}
}

func (d *ModbusDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport != nil {
		return d.transport.GetConnectionMetrics()
	}
	return 0, 0, "", "", time.Time{}
}

// ScheduleReconnect schedules a reconnection attempt for the Modbus driver.
func (d *ModbusDriver) ScheduleReconnect(ctx context.Context, timeout time.Duration) {
	if d.transport != nil {
		d.transport.ScheduleReconnect()
	}
}

func (d *ModbusDriver) GetMetrics() model.ChannelMetrics {
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()

	totalRequests := int64(0)
	successCount := int64(0)
	failureCount := int64(0)

	if d.scheduler != nil {
		d.scheduler.mu.Lock()
		totalRequests = d.scheduler.txTotal
		successCount = d.scheduler.rxTotal
		failureCount = d.scheduler.errorsTotal
		d.scheduler.mu.Unlock()
	}

	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(successCount) / float64(totalRequests)
	}

	metrics := model.ChannelMetrics{
		QualityScore:       d.calculateQualityScore(),
		Protocol:           "Modbus",
		SuccessRate:        successRate,
		TimeoutCount:       failureCount,
		CrcError:           0,
		CrcErrorRate:       0.0,
		RetryRate:          0.0,
		ExceptionCode:      0,
		AvgRtt:             0,
		MaxRtt:             0,
		MinRtt:             0,
		TotalRequests:      totalRequests,
		SuccessCount:       successCount,
		FailureCount:       failureCount,
		PacketLoss:         1.0 - successRate,
		ReconnectCount:     reconCount,
		ConnectionSeconds:  connSec,
		LocalAddr:          localAddr,
		RemoteAddr:         remoteAddr,
		LastDisconnectTime: lastDisc,
		Timestamp:          time.Now(),
	}

	return metrics
}

func (d *ModbusDriver) calculateQualityScore() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.transport == nil || !d.transport.IsConnected() {
		return 0
	}

	score := 70

	if d.reconnectCount > 10 {
		score -= 20
	} else if d.reconnectCount > 5 {
		score -= 10
	} else if d.reconnectCount > 0 {
		score -= 5
	}

	if d.scheduler != nil {
		d.scheduler.mu.Lock()
		total := d.scheduler.txTotal
		success := d.scheduler.rxTotal
		d.scheduler.mu.Unlock()

		if total > 0 {
			rate := float64(success) / float64(total)
			if rate > 0.95 {
				score += 20
			} else if rate > 0.90 {
				score += 10
			} else if rate < 0.80 {
				score -= 10
			}
		}
	}

	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return score
}

func (d *ModbusDriver) GetConnectionController() *core.ConnectionController {
	return d.connController
}

// BindLinkMutex injects channelMu into ConnectionManager for shared-link reconnect.
func (d *ModbusDriver) BindLinkMutex(mu *sync.Mutex) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.transport != nil && d.transport.connMgr != nil {
		d.transport.connMgr.SetLinkMutex(mu)
	}
}
