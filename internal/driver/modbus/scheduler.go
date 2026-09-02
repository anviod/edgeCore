package modbus

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// Scheduler 接口定义
type Scheduler interface {
	Read(ctx context.Context, points []model.Point) (map[string]model.Value, error)
	Write(ctx context.Context, point model.Point, value any) error
	GetDecoder() Decoder
}

// PointRuntime 点位运行态状态
type PointRuntime struct {
	Point         model.Point
	FailCount     int
	LastSuccess   time.Time
	State         string // OK, SKIPPED
	CooldownUntil time.Time
}

// permanentSkipUntil 协议显式非法地址(Illegal Data Address / 异常码2)的永久跳过
// 哨兵时间。点位被判非法地址后不再自动重试，仅当点位配置被人工修改（见
// prepareRuntimes 的配置变更检测）或进程重启重建 pointStates 时才解除。
var permanentSkipUntil = time.Unix(1<<62, 0)

// PointGroup 表示一组连续的点位及其地址信息
type PointGroup struct {
	RegType        model.RegisterType // 寄存器类型
	StartOffset    uint16             // 起始地址
	Count          uint16             // 数量
	Points         []model.Point      // 该组中的所有点位
	CustomFuncCode byte               // 自定义功能码（当RegType为RegCustom时使用）
}

// AddressInfo 用于存储点位的地址信息
type AddressInfo struct {
	Point         model.Point
	RegType       model.RegisterType
	Offset        uint16
	RegisterCount uint16 // 该点位占用的寄存器数
}

// PointScheduler 实现 Scheduler 接口
type PointScheduler struct {
	transport           Transport
	decoder             Decoder
	maxPacketSize       uint16
	groupThreshold      uint16
	instructionInterval time.Duration

	// adaptive batch parameters
	currentBatchSize uint16
	successStreak    int
	failureStreak    int

	// lightweight counters
	txTotal     int64
	rxTotal     int64
	errorsTotal int64

	pointStates map[string]*PointRuntime

	slaveID  uint8
	rttModel *RTTModel
	mu       sync.Mutex
}

func NewPointScheduler(transport Transport, decoder Decoder, maxPacketSize uint16, groupThreshold uint16, instructionInterval time.Duration) *PointScheduler {
	if maxPacketSize == 0 {
		maxPacketSize = 125
	}
	if groupThreshold == 0 {
		groupThreshold = 50
	}
	return &PointScheduler{
		transport:           transport,
		decoder:             decoder,
		maxPacketSize:       maxPacketSize,
		groupThreshold:      groupThreshold,
		instructionInterval: instructionInterval,
		currentBatchSize:    maxPacketSize,
		pointStates:         make(map[string]*PointRuntime),
		rttModel:            NewRTTModel(),
	}
}

func (s *PointScheduler) SetSlaveID(slaveID uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slaveID = slaveID
}

func (s *PointScheduler) GetSlaveID() uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.slaveID
}

func (s *PointScheduler) GetDecoder() Decoder {
	return s.decoder
}

func (s *PointScheduler) Read(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	now := time.Now()
	result := make(map[string]model.Value)
	allSuccess := true

	// 1. Prepare runtimes and filter points
	activePoints := s.prepareRuntimes(points)

	// If no points to read (all skipped), return empty result
	if len(activePoints) == 0 {
		return result, nil
	}

	// 2. Group points
	groups, err := s.groupPoints(activePoints)
	if err != nil {
		return nil, err
	}

	//log.Printf("Optimized reading %d points into %d groups", len(activePoints), len(groups))

	// 3. Read groups
	for i, group := range groups {
		if i > 0 && s.instructionInterval > 0 {
			time.Sleep(s.instructionInterval)
		}

		start := time.Now()
		values, err := s.readGroup(ctx, group)
		duration := time.Since(start)
		// update adaptive batch size based on outcome
		s.adaptBatchSize(err == nil, duration)

		// update lightweight counters
		atomic.AddInt64(&s.txTotal, 1)
		if err == nil {
			atomic.AddInt64(&s.rxTotal, 1)
		} else {
			atomic.AddInt64(&s.errorsTotal, 1)
			allSuccess = false
		}
		if err != nil {
			log.Printf("Error reading group starting at offset %d: %v", group.StartOffset, err)
			// Mark group failed. pointLevel 仅在单点组时为 true：多点点位的批量
			// 失败属于整组/设备级问题，不得据此把点位判为永久非法地址。
			groupLevel := len(group.Points) > 1
			for _, p := range group.Points {
				s.markPointFailed(p.ID, err, !groupLevel)
				result[p.ID] = model.Value{
					PointID: p.ID,
					Value:   nil,
					Quality: "Bad",
					TS:      now,
				}
			}
			continue
		}

		// Process success (including partial success from fallback)
		for id, val := range values {
			quality := "Good"
			var pointErr error
			if e, ok := val.(error); ok {
				pointErr = e
				val = nil
			}

			if val == nil {
				quality = "Bad"
				// 单点隔离回退读错误：可精确判定是否协议显式非法地址
				s.markPointFailed(id, pointErr, true)
				allSuccess = false
			} else {
				s.markPointSuccess(id, now)
			}

			result[id] = model.Value{
				PointID: id,
				Value:   val,
				Quality: quality,
				TS:      now,
			}
		}
	}

	// Record cycle metrics
	if mt, ok := s.transport.(*ModbusTransport); ok && mt.metricsRecorder != nil {
		mt.metricsRecorder.RecordCycle(mt.channelID, allSuccess)
	}

	return result, nil
}

func (s *PointScheduler) Write(ctx context.Context, point model.Point, value any) error {
	// Encode value
	regs, err := s.decoder.Encode(point, value)
	if err != nil {
		return err
	}

	// Determine write method based on type
	regType := point.RegisterType
	offset := uint16(0)
	_, offset, err = s.decoder.ParseAddress(point.Address)
	if err != nil {
		offset = 0
	}

	switch regType {
	case model.RegCoil:
		var boolVal bool
		switch v := value.(type) {
		case bool:
			boolVal = v
		case int:
			boolVal = v != 0
		case float64:
			boolVal = v != 0
		case string:
			boolVal = v == "true" || v == "1"
		default:
			return fmt.Errorf("unsupported value type for coil: %T", value)
		}
		return s.transport.WriteCoil(ctx, offset, boolVal)

	case model.RegHolding, model.RegCustom:
		if len(regs) == 1 {
			return s.transport.WriteRegister(ctx, offset, regs[0])
		}
		return s.transport.WriteRegisters(ctx, offset, regs)

	default:
		return fmt.Errorf("write not supported for register type: %s", regType.String())
	}
}

func (s *PointScheduler) prepareRuntimes(points []model.Point) []model.Point {
	s.mu.Lock()
	defer s.mu.Unlock()

	var active []model.Point
	now := time.Now()

	for _, p := range points {
		rt, exists := s.pointStates[p.ID]
		if !exists {
			rt = &PointRuntime{
				Point: p,
				State: "OK",
			}
			s.pointStates[p.ID] = rt
		} else if isPointConfigChanged(rt.Point, p) {
			// 人工重置信号：点位的关键配置（地址/寄存器类型/数据类型）被修改，
			// 清除永久跳过与冷却，立即重新采集（参照 Kepware：改配置即恢复）。
			rt.Point = p
			rt.State = "OK"
			rt.FailCount = 0
			rt.CooldownUntil = time.Time{}
		}

		// Check if skipped
		if rt.State == "SKIPPED" {
			if now.After(rt.CooldownUntil) {
				// Cooldown over, try again
				rt.State = "OK"
				rt.FailCount = 0 // Reset fail count to give it a chance
				active = append(active, p)
			}
			// else skip
		} else {
			active = append(active, p)
		}
	}
	return active
}

func (s *PointScheduler) markPointFailed(pointID string, err error, pointLevel bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rt, ok := s.pointStates[pointID]; ok {
		// Connection/transport-level failures (timeout, not connected, link reset)
		// are device-level faults, not point faults. Per Kepware's model, a device
		// outage must not cool its points down — otherwise on recovery every point
		// would remain SKIPPED and the device starves (防饿死). Thaw the point so
		// the whole tag set is immediately readable once the link is back.
		if err != nil && isConnectionError(err) {
			rt.FailCount = 0
			rt.State = "OK"
			rt.CooldownUntil = time.Time{}
			return
		}

		rt.FailCount++

		// 非法地址仅在"单点隔离"且协议显式返回 Illegal Data Address（异常码2）时判定。
		// pointLevel=false 表示错误来自整组批量读失败（设备离线/整组死掉），绝不能
		// 据此把全部点位判为非法地址；否则设备故障会被错误降级为永久点位跳过。
		if pointLevel && err != nil && isIllegalDataAddress(err) {
			rt.State = "SKIPPED"
			rt.CooldownUntil = permanentSkipUntil
			log.Printf("Point %s permanently SKIPPED: Illegal Data Address (Exception 2). Edit the point config to reset.", pointID)
			return
		}

		// 如果连续失败次数较多，则进入较长时间的冷却期
		// 3次失败：冷却 60秒
		// 10次失败：冷却 5分钟
		if rt.FailCount >= 10 {
			rt.State = "SKIPPED"
			rt.CooldownUntil = time.Now().Add(5 * time.Minute)
			log.Printf("Point %s failed 10 times, skipping for 5 minutes", pointID)
		} else if rt.FailCount >= 3 {
			rt.State = "SKIPPED"
			rt.CooldownUntil = time.Now().Add(60 * time.Second)
			log.Printf("Point %s skipped due to repeated failures (%d times) for 60s", pointID, rt.FailCount)
		}
	}
}

// isIllegalDataAddress 判定是否为 Modbus 协议显式返回的"非法数据地址"（异常码2）。
// 仅在单点读返回该确定性异常时命中；用文本上的非法地址异常精确区分，避免把
// illegal data value(异常3)、设备离线等其它失败误判为永久非法地址。
func isIllegalDataAddress(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "illegal data address") ||
		strings.Contains(msg, "exception 2") ||
		strings.Contains(msg, "exception '2'")
}

// isPointConfigChanged 判断点位关键配置（地址/寄存器类型）是否被人工修改。
// 在 prepareRuntimes 中作为"人工重置"信号：点位被编辑后清除永久跳过/冷却。
func isPointConfigChanged(a, b model.Point) bool {
	return a.Address != b.Address || a.RegisterType != b.RegisterType || a.DataType != b.DataType
}

// connectionErrorHints 判定传输/链路级失败（设备级故障）。命中时点位不做冷却，
// 视为设备离线而非点位异常，避免连接恢复后点位饿死。
var connectionErrorHints = []string{
	"not connected", "connection refused", "connection reset", "connection closed",
	"network unreachable", "no route to host", "broken pipe", "dial ",
	"tls handshake", "cannot assign requested address", "i/o timeout",
	"request timed out", "timeout", "modbus: not connected",
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, h := range connectionErrorHints {
		if strings.Contains(msg, h) {
			return true
		}
	}
	return false
}

func (s *PointScheduler) markPointSuccess(pointID string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rt, ok := s.pointStates[pointID]; ok {
		rt.FailCount = 0
		rt.LastSuccess = now
		rt.State = "OK"
	}
}

func (s *PointScheduler) groupPoints(points []model.Point) ([]PointGroup, error) {
	if len(points) == 0 {
		return []PointGroup{}, nil
	}

	// 1. Parse address info
	addressInfos := make([]AddressInfo, len(points))
	for i, p := range points {
		// 优先使用Point中指定的RegisterType，如果没有指定则从地址解析
		regType := p.RegisterType
		offset := uint16(0)
		var err error

		// 无论RegisterType是什么，都应该解析地址以获取正确的偏移量
		// 因为地址可能是人类可读格式（如 "40000"），需要转换为协议地址（如 0）
		_, offset, err = s.decoder.ParseAddress(p.Address)
		if err != nil {
			offset = 0
		}

		// 如果用户明确指定了RegisterType，则使用用户指定的类型
		// 否则，使用从地址解析出的类型
		if regType == model.RegHolding {
			// RegisterType为默认值，使用从地址解析出的类型
			parsedType, _, err := s.decoder.ParseAddress(p.Address)
			if err == nil && parsedType != model.RegHolding {
				regType = parsedType
			}
		}

		addressInfos[i] = AddressInfo{
			Point:         p,
			RegType:       regType,
			Offset:        offset,
			RegisterCount: s.decoder.GetPointRegisterCount(p),
		}
	}

	// 2. Group by RegType
	typeGroups := make(map[model.RegisterType][]AddressInfo)
	for _, info := range addressInfos {
		typeGroups[info.RegType] = append(typeGroups[info.RegType], info)
	}

	// 3. Group by contiguous address ranges
	var groups []PointGroup

	for regType, infos := range typeGroups {
		// 对coil和discrete input单独处理
		if regType == model.RegCoil || regType == model.RegDiscreteInput {
			for _, info := range infos {
				groups = append(groups, PointGroup{
					RegType:     regType,
					StartOffset: info.Offset,
					Count:       1,
					Points:      []model.Point{info.Point},
				})
			}
			continue
		}

		sort.Slice(infos, func(i, j int) bool {
			return infos[i].Offset < infos[j].Offset
		})

		s.mu.Lock()
		effectiveMax := s.maxPacketSize
		s.mu.Unlock()

		i := 0
		for i < len(infos) {
			currentGroup := PointGroup{
				RegType:     regType,
				StartOffset: infos[i].Offset,
				Points:      []model.Point{infos[i].Point},
				Count:       infos[i].RegisterCount,
			}

			currentEndOffset := currentGroup.StartOffset + currentGroup.Count

			lastProcessedIndex := i

			for j := i + 1; j < len(infos); j++ {
				info := infos[j]

				gap := int(info.Offset) - int(currentEndOffset)
				if gap < 0 {
					gap = 0
				}

				wouldExceedMax := (currentGroup.Count + uint16(gap) + info.RegisterCount) > effectiveMax

				if gap > int(s.groupThreshold) {
					break
				}

				if wouldExceedMax {
					break
				}

				newCount := info.Offset - currentGroup.StartOffset + info.RegisterCount
				currentGroup.Count = newCount
				currentGroup.Points = append(currentGroup.Points, info.Point)
				currentEndOffset = info.Offset + info.RegisterCount
				lastProcessedIndex = j
			}

			if len(currentGroup.Points) > 0 {
				groups = append(groups, currentGroup)
			}

			i = lastProcessedIndex + 1
		}
	}

	return groups, nil
}

func (s *PointScheduler) readGroup(ctx context.Context, group PointGroup) (map[string]any, error) {
	result := make(map[string]any)

	// Single point read for bools
	if group.RegType == model.RegCoil {
		startTime := time.Now()
		val, err := s.transport.ReadCoil(ctx, group.StartOffset)
		rtt := time.Since(startTime)
		s.rttModel.Record(1, rtt)

		if err != nil {
			return nil, err
		}
		result[group.Points[0].ID] = val
		return result, nil
	}
	if group.RegType == model.RegDiscreteInput {
		startTime := time.Now()
		val, err := s.transport.ReadDiscreteInput(ctx, group.StartOffset)
		rtt := time.Since(startTime)
		s.rttModel.Record(1, rtt)

		if err != nil {
			return nil, err
		}
		result[group.Points[0].ID] = val
		return result, nil
	}

	// 处理自定义功能码 - 当前版本暂不支持
	// 如需使用非标功能码，请使用标准的Holding/Input寄存器类型
	if group.RegType == model.RegCustom && group.CustomFuncCode > 0 {
		log.Printf("Warning: Custom function code not fully supported yet, using Holding Register")
		// 回退到标准Holding寄存器读取
		group.RegType = model.RegHolding
	}

	// 处理自定义功能码 - 当前版本暂不支持
	// 如需使用非标功能码，请使用标准的Holding/Input寄存器类型
	if group.RegType == model.RegCustom && group.CustomFuncCode > 0 {
		log.Printf("Warning: Custom function code not fully supported yet, using Holding Register")
		// 回退到标准Holding寄存器读取
		group.RegType = model.RegHolding
	}

	// Batch read for registers
	startTime := time.Now()
	bytes, err := s.transport.ReadRegisters(ctx, group.RegType.ShortString(), group.StartOffset, group.Count)
	rtt := time.Since(startTime)
	s.rttModel.Record(int(group.Count), rtt)

	if err != nil {
		// Performance optimization: If it's a timeout, skip per-point fallback to avoid long blocking.
		// In industrial collection, a group timeout usually means the device or bus is busy/offline.
		isTimeout := strings.Contains(strings.ToLower(err.Error()), "timeout")
		if isTimeout {
			log.Printf("Group read timed out for %s, skipping fallback", group.RegType)
			for _, point := range group.Points {
				result[point.ID] = err
			}
			return result, nil // Return result with errors so they are marked Bad
		}

		// Fallback: try per-point reads to avoid whole-group failure due to illegal addresses
		// But limit to avoid excessive network requests
		fallbackTimeout := time.NewTimer(5 * time.Second)
		defer fallbackTimeout.Stop()

		for _, point := range group.Points {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-fallbackTimeout.C:
				log.Printf("Fallback read timeout, stopping further reads")
				for _, p := range group.Points {
					if _, exists := result[p.ID]; !exists {
						result[p.ID] = fmt.Errorf("fallback read timeout")
					}
				}
				return result, nil
			default:
				_, offset, _ := s.decoder.ParseAddress(point.Address)
				regCount := s.decoder.GetPointRegisterCount(point)

				b, perr := s.transport.ReadRegisters(ctx, group.RegType.ShortString(), offset, regCount)
				if perr != nil || len(b) < int(regCount*2) {
					// Mark as failed with error value; caller will convert to Bad quality
					if perr != nil {
						result[point.ID] = perr
					} else {
						result[point.ID] = fmt.Errorf("read length mismatch")
					}
					// record debug info if transport supports metrics recorder
					if mt, ok := s.transport.(*ModbusTransport); ok && mt.metricsRecorder != nil {
						mt.metricsRecorder.RecordPointDebug(mt.channelID, point.ID, nil, nil, "Bad")
					}
					continue
				}

				val, quality, derr := s.decoder.Decode(point, b)
				if derr != nil {
					log.Printf("Error decoding point %s in fallback: %v", point.ID, derr)
					result[point.ID] = derr
					if mt, ok := s.transport.(*ModbusTransport); ok && mt.metricsRecorder != nil {
						mt.metricsRecorder.RecordPointDebug(mt.channelID, point.ID, b, nil, "Bad")
					}
					continue
				}

				if mt, ok := s.transport.(*ModbusTransport); ok && mt.metricsRecorder != nil {
					mt.metricsRecorder.RecordPointDebug(mt.channelID, point.ID, b, val, quality)
				}

				result[point.ID] = val
			}
		}

		// If we managed to read at least one point, treat as success at group level.
		// The caller will mark individual points Good/Bad based on value.
		if len(result) > 0 {
			return result, nil
		}

		// If everything failed, propagate the original error.
		return nil, err
	}

	// Distribute data to points
	for _, point := range group.Points {
		_, offset, _ := s.decoder.ParseAddress(point.Address)
		regCount := s.decoder.GetPointRegisterCount(point)

		byteOffset := (offset - group.StartOffset) * 2
		byteLength := regCount * 2

		if int(byteOffset+byteLength) > len(bytes) {
			continue
		}

		pointBytes := bytes[byteOffset : byteOffset+byteLength]
		val, quality, err := s.decoder.Decode(point, pointBytes)
		if err != nil {
			log.Printf("Error decoding point %s: %v", point.ID, err)
			// record debug info if transport supports metrics recorder
			if mt, ok := s.transport.(*ModbusTransport); ok && mt.metricsRecorder != nil {
				mt.metricsRecorder.RecordPointDebug(mt.channelID, point.ID, pointBytes, nil, "Bad")
			}
			continue
		}

		// record successful decode
		if mt, ok := s.transport.(*ModbusTransport); ok && mt.metricsRecorder != nil {
			mt.metricsRecorder.RecordPointDebug(mt.channelID, point.ID, pointBytes, val, quality)
		}

		result[point.ID] = val
	}

	return result, nil
}

// SetMaxPacketSize allows updating the scheduler's maximum packet size (e.g. after MTU probe)
func (s *PointScheduler) SetMaxPacketSize(m uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m == 0 {
		return
	}
	s.maxPacketSize = m
	if s.currentBatchSize == 0 || s.currentBatchSize > m {
		s.currentBatchSize = m
	}
}

// SetGroupThreshold 设置块读合并的最大地址间隙（来自 GapOptimizer）。
func (s *PointScheduler) SetGroupThreshold(threshold uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if threshold == 0 {
		return
	}
	s.groupThreshold = threshold
}

func (s *PointScheduler) getEffectiveMaxPacketSize() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentBatchSize == 0 {
		return s.maxPacketSize
	}
	if s.currentBatchSize < s.maxPacketSize {
		return s.currentBatchSize
	}
	return s.maxPacketSize
}

// adaptBatchSize uses RTTModel to dynamically adjust batch size based on actual performance
func (s *PointScheduler) adaptBatchSize(success bool, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentBatchSize == 0 {
		s.currentBatchSize = s.maxPacketSize
	}

	// Use RTTModel to determine optimal batch size
	optimalSize := s.rttModel.BestBatchSize()
	if optimalSize > 0 {
		// Clamp to valid range
		if optimalSize > int(s.maxPacketSize) {
			optimalSize = int(s.maxPacketSize)
		}
		if optimalSize < 8 {
			optimalSize = 8
		}
		s.currentBatchSize = uint16(optimalSize)
	}
}
