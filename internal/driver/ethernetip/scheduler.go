package ethernetip

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	go_ethernet_ip "github.com/anviod/ethernet-ip"
	"github.com/anviod/ethernet-ip/messages/packet"
	"go.uber.org/zap"
)

type ENIPScheduler struct {
	transport *ENIPTransport
	decoder   *ENIPDecoder

	batchReadMax int
	minInterval  time.Duration

	totalRequests int64
	successCount  int64
	failureCount  int64
	mu            sync.Mutex
}

var logixClass2AttrIDs = map[string]int{
	"BoolTag":   1,
	"SintTag":   2,
	"IntTag":    3,
	"DintTag":   4,
	"LintTag":   5,
	"UsintTag":  6,
	"UintTag":   7,
	"UdintTag":  8,
	"UlintTag":  9,
	"RealTag":   10,
	"LrealTag":  11,
	"StringTag": 12,
}

func (s *ENIPScheduler) resolveLogixTagName(tagName string) string {
	if lastDot := strings.LastIndex(tagName, "."); lastDot >= 0 {
		tagName = tagName[lastDot+1:]
	}
	if lastColon := strings.LastIndex(tagName, ":"); lastColon >= 0 {
		tagName = tagName[lastColon+1:]
	}
	return tagName
}

func (s *ENIPScheduler) getLogixClass2AttrID(tagName string) (int, bool) {
	attrID, ok := logixClass2AttrIDs[tagName]
	return attrID, ok
}

func NewENIPScheduler(transport *ENIPTransport, decoder *ENIPDecoder, cfg map[string]any) *ENIPScheduler {
	s := &ENIPScheduler{
		transport:    transport,
		decoder:      decoder,
		batchReadMax: 50,
		minInterval:  5 * time.Millisecond,
	}

	if v, ok := cfg["batch_read_max"]; ok {
		switch val := v.(type) {
		case float64:
			s.batchReadMax = int(val)
		case int:
			s.batchReadMax = val
		}
	}

	if s.batchReadMax > 50 {
		s.batchReadMax = 50
	}

	return s
}

type pointWithTag struct {
	Point model.Point
	Tag   *ENIPTag
}

// tagWithPoint 用于批量读取时存储标签与点位的映射关系
type tagWithPoint struct {
	tag  *go_ethernet_ip.Tag
	pwt  pointWithTag
	name string
}

func (s *ENIPScheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value)

	if s.transport.NeedProbeCheck() {
		zap.L().Debug("[ENIP] Performing probe check for low-frequency collection")
		s.transport.ProbeConnection()
	}

	tcp := s.transport.GetClient()
	if tcp == nil {
		// 尝试自动重连
		zap.L().Warn("[ENIP] Client not connected, attempting reconnect")
		if err := s.transport.Connect(ctx); err != nil {
			zap.L().Error("[ENIP] Reconnect failed", zap.Error(err))
			s.transport.RecordFailure(err)
			return nil, fmt.Errorf("ENIP client not connected: %w", err)
		}
		tcp = s.transport.GetClient()
		if tcp == nil {
			s.transport.RecordFailure(fmt.Errorf("failed to get client after reconnect"))
			return nil, fmt.Errorf("ENIP client not connected after reconnect attempt")
		}
		zap.L().Info("[ENIP] Reconnect successful")
		s.transport.RecordSuccess()
	}

	var parsed []pointWithTag
	for _, p := range points {
		tag, err := s.decoder.ParseAddress(p.Address)
		if err != nil {
			zap.L().Warn("[ENIP] Failed to parse address",
				zap.String("point", p.Name),
				zap.String("address", p.Address),
				zap.Error(err),
			)
			results[p.ID] = model.Value{
				PointID: p.ID,
				Quality: "Bad",
				TS:      time.Now(),
			}
			s.incFailure()
			continue
		}
		parsed = append(parsed, pointWithTag{Point: p, Tag: tag})
	}

	if len(parsed) == 0 {
		return results, nil
	}

	// 在 Logix 模式下，分离 Class 2 属性标签和普通标签
	if strings.EqualFold(s.transport.connectionType, "logix") {
		var class2Points, regularPoints []pointWithTag
		for _, pwt := range parsed {
			address := pwt.Point.Address
			if address == "" {
				address = pwt.Point.Name
			}
			resolvedName := s.resolveLogixTagName(address)
			if _, ok := s.getLogixClass2AttrID(resolvedName); ok {
				class2Points = append(class2Points, pwt)
			} else {
				regularPoints = append(regularPoints, pwt)
			}
		}

		// 读取 Class 2 属性标签
		for _, pwt := range class2Points {
			address := pwt.Point.Address
			if address == "" {
				address = pwt.Point.Name
			}
			resolvedName := s.resolveLogixTagName(address)
			attrID, _ := s.getLogixClass2AttrID(resolvedName)

			data, err := s.readClass2Attribute(tcp, attrID)
			if err != nil {
				zap.L().Warn("[ENIP] Failed to read Class 2 attribute",
					zap.String("point", pwt.Point.Name),
					zap.String("address", address),
					zap.Error(err),
				)
				results[pwt.Point.ID] = model.Value{
					PointID: pwt.Point.ID,
					Quality: "Bad",
					TS:      time.Now(),
				}
				s.incFailure()
				continue
			}

			value, err := s.decoder.DecodeValue(data, pwt.Point.DataType)
			if err != nil {
				zap.L().Warn("[ENIP] Failed to decode Class 2 attribute value",
					zap.String("point", pwt.Point.Name),
					zap.Error(err),
				)
				results[pwt.Point.ID] = model.Value{
					PointID: pwt.Point.ID,
					Quality: "Bad",
					TS:      time.Now(),
				}
				s.incFailure()
				continue
			}

			results[pwt.Point.ID] = model.Value{
				PointID: pwt.Point.ID,
				Value:   value,
				Quality: "Good",
				TS:      time.Now(),
			}
			s.incSuccess()
		}

		// 读取普通标签
		if len(regularPoints) > 0 {
			groups := s.groupTags(regularPoints)
			for _, group := range groups {
				newTcp, err := s.readGroup(ctx, tcp, group, results)
				if err != nil {
					zap.L().Warn("[ENIP] Failed to read group",
						zap.Error(err),
					)
				} else if newTcp != nil {
					// 更新连接对象，以便后续调用使用新连接
					tcp = newTcp
				}
			}
		}
	} else {
		// 非 Logix 模式，使用标准 Tag 读取
		groups := s.groupTags(parsed)
		for _, group := range groups {
			newTcp, err := s.readGroup(ctx, tcp, group, results)
			if err != nil {
				zap.L().Warn("[ENIP] Failed to read group",
					zap.Error(err),
				)
			} else if newTcp != nil {
				// 更新连接对象，以便后续调用使用新连接
				tcp = newTcp
			}
		}
	}

	return results, nil
}

func (s *ENIPScheduler) groupTags(points []pointWithTag) [][]pointWithTag {
	if len(points) <= s.batchReadMax {
		return [][]pointWithTag{points}
	}

	var groups [][]pointWithTag
	for i := 0; i < len(points); i += s.batchReadMax {
		end := i + s.batchReadMax
		if end > len(points) {
			end = len(points)
		}
		groups = append(groups, points[i:end])
	}

	return groups
}

func (s *ENIPScheduler) readGroup(ctx context.Context, tcp *go_ethernet_ip.EIPTCP, points []pointWithTag, results map[string]model.Value) (*go_ethernet_ip.EIPTCP, error) {
	if len(points) == 0 {
		return tcp, nil
	}

	s.incTotal()

	// 检查连接状态
	if tcp == nil || !tcp.IsConnected() {
		zap.L().Warn("[ENIP] Connection lost during readGroup, attempting to reconnect")
		if err := s.transport.Connect(ctx); err != nil {
			zap.L().Error("[ENIP] Reconnect failed in readGroup", zap.Error(err))
			s.transport.RecordFailure(err)
			for _, pwt := range points {
				results[pwt.Point.ID] = model.Value{
					PointID: pwt.Point.ID,
					Quality: "Bad",
					TS:      time.Now(),
				}
				s.incFailure()
			}
			return nil, fmt.Errorf("connection lost: %w", err)
		}
		tcp = s.transport.GetClient()
		if tcp == nil {
			zap.L().Error("[ENIP] Failed to get client after reconnect")
			s.transport.RecordFailure(fmt.Errorf("failed to get client after reconnect"))
			for _, pwt := range points {
				results[pwt.Point.ID] = model.Value{
					PointID: pwt.Point.ID,
					Quality: "Bad",
					TS:      time.Now(),
				}
				s.incFailure()
			}
			return nil, fmt.Errorf("failed to get client after reconnect")
		}
		zap.L().Info("[ENIP] Reconnect successful in readGroup")
		s.transport.RecordSuccess()
	}

	// 使用 TagGroup 进行批量读取优化
	tg := go_ethernet_ip.NewTagGroup(new(sync.Mutex))

	// 存储标签与点位的映射关系
	tagsWithPoints := make([]tagWithPoint, 0, len(points))

	// 阶段1: 初始化所有标签并添加到 TagGroup
	for _, pwt := range points {
		// 使用 Path 构建完整的标签名称，如果 Path 长度大于1
		fullName := pwt.Tag.Name
		if len(pwt.Tag.Path) > 1 {
			fullName = strings.Join(pwt.Tag.Path, ".")
		}
		// 如果是简单数组标签（非程序标签），添加数组索引
		if pwt.Tag.ArrayIndex >= 0 && len(pwt.Tag.Path) == 1 {
			fullName = fmt.Sprintf("%s[%d]", pwt.Tag.Name, pwt.Tag.ArrayIndex)
		}

		tag := new(go_ethernet_ip.Tag)

		err := tcp.InitializeTag(fullName, tag)
		if err != nil {
			zap.L().Warn("[ENIP] Failed to initialize tag",
				zap.String("name", fullName),
				zap.Error(err),
			)
			results[pwt.Point.ID] = model.Value{
				PointID: pwt.Point.ID,
				Quality: "Bad",
				TS:      time.Now(),
			}
			s.incFailure()
			continue
		}

		tg.Add(tag)
		tagsWithPoints = append(tagsWithPoints, tagWithPoint{tag: tag, pwt: pwt, name: fullName})
	}

	if len(tagsWithPoints) == 0 {
		return tcp, nil
	}

	// 阶段2: 批量读取所有标签
	startTime := time.Now()
	err := tg.Read()
	readDuration := time.Since(startTime)

	if err != nil {
		zap.L().Warn("[ENIP] Batch read failed",
			zap.Error(err),
			zap.Duration("duration", readDuration),
		)
		// 如果批量读取失败，尝试逐个读取
		for _, twp := range tagsWithPoints {
			readErr := twp.tag.Read()
			if readErr != nil {
				zap.L().Warn("[ENIP] Failed to read tag",
					zap.String("name", twp.name),
					zap.Error(readErr),
				)
				results[twp.pwt.Point.ID] = model.Value{
					PointID: twp.pwt.Point.ID,
					Quality: "Bad",
					TS:      time.Now(),
				}
				s.incFailure()
			} else {
				s.processTagValue(twp, results)
			}
		}
		return tcp, nil
	}

	// 阶段3: 处理批量读取结果
	for _, twp := range tagsWithPoints {
		s.processTagValue(twp, results)
	}

	zap.L().Info("[ENIP] Batch read completed",
		zap.Int("count", len(tagsWithPoints)),
		zap.Duration("duration", readDuration),
	)

	return tcp, nil
}

// processTagValue 处理单个标签的值并存储到结果中
func (s *ENIPScheduler) processTagValue(twp tagWithPoint, results map[string]model.Value) {
	val := twp.tag.GetValue()
	if val == nil {
		zap.L().Warn("[ENIP] Tag value is nil",
			zap.String("name", twp.name),
		)
		results[twp.pwt.Point.ID] = model.Value{
			PointID: twp.pwt.Point.ID,
			Quality: "Bad",
			TS:      time.Now(),
		}
		s.incFailure()
		return
	}

	results[twp.pwt.Point.ID] = model.Value{
		PointID: twp.pwt.Point.ID,
		Value:   val,
		Quality: "Good",
		TS:      time.Now(),
	}
	s.incSuccess()
}

func (s *ENIPScheduler) WritePoint(ctx context.Context, p model.Point, value interface{}) error {
	// 获取连接配置
	var cfg *go_ethernet_ip.Config
	if s.transport.port != 0 {
		cfg = &go_ethernet_ip.Config{TCPPort: uint16(s.transport.port)}
	}
	conn, err := go_ethernet_ip.NewTCP(s.transport.ip, cfg)
	if err != nil {
		return fmt.Errorf("failed to create ENIP client: %w", err)
	}
	defer conn.Close()

	// 连接到设备
	if err := conn.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	address := p.Address
	if address == "" {
		address = p.Name
	}

	// 在 Logix 模式下，对预定义 Class 2 属性标签使用 Class 2 写入
	if strings.EqualFold(s.transport.connectionType, "logix") {
		resolvedName := s.resolveLogixTagName(address)
		if attrID, ok := s.getLogixClass2AttrID(resolvedName); ok {
			encoded, err := s.decoder.EncodeValue(value, p.DataType)
			if err != nil {
				return fmt.Errorf("failed to encode Class 2 value for %s: %w", address, err)
			}
			if err := s.writeClass2Attribute(conn, attrID, encoded); err == nil {
				return nil
			} else {
				zap.L().Warn("Logix Class 2 write failed, falling back to tag write",
					zap.String("address", address),
					zap.Error(err),
				)
			}
		}
	}

	// 使用 Tag 方式写入
	tag := new(go_ethernet_ip.Tag)
	err = conn.InitializeTag(address, tag)
	if err != nil {
		return fmt.Errorf("failed to initialize tag %s: %w", address, err)
	}

	// 根据数据类型设置值（处理 JSON 解析的 float64 类型）
	switch strings.ToUpper(p.DataType) {
	case "BOOL":
		switch v := value.(type) {
		case bool:
			tag.SetBool(v)
		default:
			return fmt.Errorf("invalid type for BOOL: %T", value)
		}
	case "SINT":
		tag.SetInt8(int8(toInt64(value)))
	case "INT":
		tag.SetInt16(int16(toInt64(value)))
	case "DINT":
		tag.SetInt32(int32(toInt64(value)))
	case "LINT":
		tag.SetInt64(toInt64(value))
	case "USINT":
		tag.SetUInt8(uint8(toUint64(value)))
	case "UINT":
		tag.SetUInt16(uint16(toUint64(value)))
	case "UDINT":
		tag.SetUInt32(uint32(toUint64(value)))
	case "ULINT":
		tag.SetUInt64(toUint64(value))
	case "REAL":
		switch v := value.(type) {
		case float32:
			tag.SetFloat32(v)
		case float64:
			tag.SetFloat32(float32(v))
		default:
			return fmt.Errorf("invalid type for REAL: %T", value)
		}
	case "LREAL":
		switch v := value.(type) {
		case float64:
			tag.SetFloat64(v)
		case float32:
			tag.SetFloat64(float64(v))
		default:
			return fmt.Errorf("invalid type for LREAL: %T", value)
		}
	case "STRING":
		switch v := value.(type) {
		case string:
			tag.SetString(v)
		default:
			return fmt.Errorf("invalid type for STRING: %T", value)
		}
	default:
		return fmt.Errorf("unsupported data type: %s", p.DataType)
	}

	// 执行写入
	if err := tag.Write(); err != nil {
		return fmt.Errorf("failed to write tag %s: %w", address, err)
	}

	return nil
}

// toInt64 将 interface{} 转换为 int64（处理 JSON 解析的 float64）
func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int32:
		return int64(val)
	case int16:
		return int64(val)
	case int8:
		return int64(val)
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case float32:
		return int64(val)
	default:
		return 0
	}
}

// toUint64 将 interface{} 转换为 uint64（处理 JSON 解析的 float64）
func toUint64(v interface{}) uint64 {
	switch val := v.(type) {
	case uint64:
		return val
	case uint32:
		return uint64(val)
	case uint16:
		return uint64(val)
	case uint8:
		return uint64(val)
	case uint:
		return uint64(val)
	case float64:
		return uint64(val)
	case float32:
		return uint64(val)
	default:
		return 0
	}
}

// writeClass2Attribute 使用现有的连接进行 Class 2 属性写入
func (s *ENIPScheduler) writeClass2Attribute(tcp *ENIPClient, attrID int, data []byte) error {
	pathData := []byte{
		0x20, 0x02, // Class ID: Class 2
		0x24, 0x01, // Instance ID: Instance 1
		0x30, byte(attrID), // Attribute ID
	}

	mr := packet.NewMessageRouter(0x10, pathData, data) // 0x10 = Set Attribute Single
	response, err := tcp.Send(mr)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if response == nil || response.Packet == nil {
		return fmt.Errorf("empty response")
	}

	itemIdx := -1
	for i, item := range response.Packet.Items {
		if item.TypeID == packet.ItemIDUnconnectedMessage {
			itemIdx = i
			break
		}
	}

	if itemIdx < 0 {
		return fmt.Errorf("no CIP response found")
	}

	item := response.Packet.Items[itemIdx]
	if len(item.Data) < 2 {
		return fmt.Errorf("response data too short")
	}

	rmr := &packet.MessageRouterResponse{}
	rmr.Decode(item.Data)

	if rmr.GeneralStatus != 0 {
		return fmt.Errorf("CIP error: 0x%02X", rmr.GeneralStatus)
	}

	return nil
}

// readClass2Attribute 使用现有的连接进行 Class 2 属性读取
func (s *ENIPScheduler) readClass2Attribute(tcp *ENIPClient, attrID int) ([]byte, error) {
	pathData := []byte{
		0x20, 0x02, // Class ID: Class 2
		0x24, 0x01, // Instance ID: Instance 1
		0x30, byte(attrID), // Attribute ID
	}

	mr := packet.NewMessageRouter(0x0E, pathData, nil) // 0x0E = Get Attribute Single
	response, err := tcp.Send(mr)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if response == nil || response.Packet == nil {
		return nil, fmt.Errorf("empty response")
	}

	itemIdx := -1
	for i, item := range response.Packet.Items {
		if item.TypeID == packet.ItemIDUnconnectedMessage {
			itemIdx = i
			break
		}
	}

	if itemIdx < 0 {
		return nil, fmt.Errorf("no CIP response found")
	}

	item := response.Packet.Items[itemIdx]
	if len(item.Data) < 4 {
		return nil, fmt.Errorf("response data too short")
	}

	rmr := &packet.MessageRouterResponse{}
	rmr.Decode(item.Data)

	if rmr.GeneralStatus != 0 {
		return nil, fmt.Errorf("CIP error: 0x%02X", rmr.GeneralStatus)
	}

	// 返回属性数据
	return rmr.ResponseData, nil
}

func (s *ENIPScheduler) GetStats() (totalRequests, successCount, failureCount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalRequests, s.successCount, s.failureCount
}

func (s *ENIPScheduler) incTotal() {
	atomic.AddInt64(&s.totalRequests, 1)
}

func (s *ENIPScheduler) incSuccess() {
	atomic.AddInt64(&s.successCount, 1)
}

func (s *ENIPScheduler) incFailure() {
	atomic.AddInt64(&s.failureCount, 1)
}
