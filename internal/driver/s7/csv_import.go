package s7

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

// CSVPointConfig CSV导入的点位配置
type CSVPointConfig struct {
	TagName     string // 标签名
	Type        string // 数据类型 (BOOL, LREAL, REAL, DWORD)
	Description string // 描述
	IOAddress   string // I/O地址 (Device1.DB1.BOOL.7006.7)
	Unit        string // 单位
	DataGroup   string // 数据分组
	ReadOnly    bool   // 是否只读
}

// ParseCSVFile 解析CSV文件，返回点位配置列表
func ParseCSVFile(filePath string) ([]CSVPointConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	return ParseCSVReader(file)
}

// ParseCSVReader 从reader解析CSV数据
func ParseCSVReader(reader io.Reader) ([]CSVPointConfig, error) {
	csvReader := csv.NewReader(reader)
	csvReader.LazyQuotes = true
	csvReader.TrimLeadingSpace = true

	// 跳过文件头（VERSION, Data Group, ID, Description等）
	// 查找 "Common Variant" 行，之后是实际数据
	var headerRow []string
	var points []CSVPointConfig
	foundHeader := false

	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row: %w", err)
		}

		// 查找 "Common Variant" 标记行
		if len(row) > 0 && row[0] == "Common Variant" {
			// 下一行是表头
			headerRow, err = csvReader.Read()
			if err != nil {
				return nil, fmt.Errorf("failed to read CSV header: %w", err)
			}
			foundHeader = true
			continue
		}

		if !foundHeader {
			continue
		}

		// 解析数据行
		if len(row) < 9 {
			continue // 跳过不完整的行
		}

		config := parseCSVRow(row, headerRow)
		if config != nil {
			points = append(points, *config)
		}
	}

	return points, nil
}

// parseCSVRow 解析单行CSV数据
func parseCSVRow(row []string, header []string) *CSVPointConfig {
	// 根据CSV格式，关键列的位置是固定的：
	// 0: ID, 1: Tag Name, 2: Type, 5: Description, 8: I/O Address, 12: Read Only, 14: Unit, 15: Data Group

	getField := func(idx int) string {
		if idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	tagName := getField(1)
	if tagName == "" {
		return nil
	}

	ioAddress := getField(8)
	if ioAddress == "" {
		return nil
	}

	return &CSVPointConfig{
		TagName:     tagName,
		Type:        getField(2),
		Description: getField(5),
		IOAddress:   ioAddress,
		Unit:        getField(14),
		DataGroup:   getField(15),
		ReadOnly:    strings.ToUpper(getField(12)) == "YES",
	}
}

// ConvertCSVToS7Address 将CSV I/O地址转换为S7地址格式
// 输入: Device1.DB1.BOOL.7006.7 -> 输出: DB1.DBX7006.7
// 输入: Device1.DB1.REAL.7500 -> 输出: DB1.DBD7500
// 输入: Device1.DB1.LREAL.7500 -> 输出: DB1.DBD7500
// 输入: Device1.Q.BOOL.1.3 -> 输出: Q1.3
// 输入: Device1.I.BOOL.0.0 -> 输出: I0.0
func ConvertCSVToS7Address(ioAddress string) (string, error) {
	// 格式: Device{N}.{Area}.{DataType}.{ByteOffset}.{BitOffset?}
	parts := strings.Split(ioAddress, ".")
	if len(parts) < 4 {
		return "", fmt.Errorf("invalid I/O address format: %s", ioAddress)
	}

	// 跳过 Device{N} 部分
	areaStr := parts[1]    // DB1, Q, I, M 等
	dataType := parts[2]   // BOOL, REAL, LREAL, DWORD
	byteOffset := parts[3] // 字节偏移

	bitOffset := ""
	if len(parts) > 4 {
		bitOffset = parts[4]
	}

	// 根据区域和数据类型构建S7地址
	areaUpper := strings.ToUpper(areaStr)

	if strings.HasPrefix(areaUpper, "DB") {
		// DB区域
		dbNum := areaStr[2:] // 提取DB号
		return convertDBAddress(dbNum, dataType, byteOffset, bitOffset)
	}

	// 非DB区域 (Q, I, M, T, C)
	return convertNonDBAddress(areaStr, dataType, byteOffset, bitOffset)
}

// convertDBAddress 转换DB区域地址
func convertDBAddress(dbNum, dataType, byteOffset, bitOffset string) (string, error) {
	switch strings.ToUpper(dataType) {
	case "BOOL":
		if bitOffset == "" {
			return "", fmt.Errorf("BOOL type requires bit offset in address")
		}
		return fmt.Sprintf("DB%s.DBX%s.%s", dbNum, byteOffset, bitOffset), nil

	case "REAL", "LREAL":
		// REAL和LREAL都使用DBD (double word)
		return fmt.Sprintf("DB%s.DBD%s", dbNum, byteOffset), nil

	case "DWORD":
		return fmt.Sprintf("DB%s.DBD%s", dbNum, byteOffset), nil

	case "WORD", "INT", "UINT":
		return fmt.Sprintf("DB%s.DBW%s", dbNum, byteOffset), nil

	case "BYTE", "SINT", "USINT":
		return fmt.Sprintf("DB%s.DBB%s", dbNum, byteOffset), nil

	default:
		// 默认使用DBD
		return fmt.Sprintf("DB%s.DBD%s", dbNum, byteOffset), nil
	}
}

// convertNonDBAddress 转换非DB区域地址 (Q, I, M等)
func convertNonDBAddress(area, dataType, byteOffset, bitOffset string) (string, error) {
	areaUpper := strings.ToUpper(area)

	switch strings.ToUpper(dataType) {
	case "BOOL":
		if bitOffset == "" {
			return "", fmt.Errorf("BOOL type requires bit offset in address")
		}
		return fmt.Sprintf("%s%s.%s", areaUpper, byteOffset, bitOffset), nil

	case "REAL", "LREAL", "DWORD":
		return fmt.Sprintf("%sD%s", areaUpper, byteOffset), nil

	case "WORD", "INT", "UINT":
		return fmt.Sprintf("%sW%s", areaUpper, byteOffset), nil

	case "BYTE", "SINT", "USINT":
		return fmt.Sprintf("%sB%s", areaUpper, byteOffset), nil

	default:
		return fmt.Sprintf("%s%s", areaUpper, byteOffset), nil
	}
}

// ConvertCSVTypeToS7DataType 将CSV数据类型转换为S7数据类型
func ConvertCSVTypeToS7DataType(csvType string) string {
	switch strings.ToUpper(csvType) {
	case "BOOL":
		return "bool"
	case "REAL":
		return "float32"
	case "LREAL":
		return "float64"
	case "DWORD":
		return "uint32"
	case "WORD":
		return "uint16"
	case "INT":
		return "int16"
	case "UINT":
		return "uint16"
	case "BYTE":
		return "uint8"
	case "SINT":
		return "int8"
	case "USINT":
		return "uint8"
	case "DINT":
		return "int32"
	case "UDINT":
		return "uint32"
	case "STRING":
		return "string"
	default:
		return "float32" // 默认
	}
}

// CSVToPoints 将CSV配置转换为Point列表
func CSVToPoints(csvPoints []CSVPointConfig) ([]model.Point, error) {
	var points []model.Point

	for i, cp := range csvPoints {
		s7Addr, err := ConvertCSVToS7Address(cp.IOAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to convert address for point %s: %w", cp.TagName, err)
		}

		dataType := ConvertCSVTypeToS7DataType(cp.Type)

		rw := "R"
		if !cp.ReadOnly {
			rw = "RW"
		}

		point := model.Point{
			ID:           fmt.Sprintf("csv_%d", i+1),
			Name:         cp.TagName,
			Address:      s7Addr,
			DataType:     dataType,
			Unit:         cp.Unit,
			ReadWrite:    rw,
			Group:        cp.DataGroup,
			RegisterType: model.RegHolding, // S7不使用Modbus寄存器类型，设置默认值
		}

		points = append(points, point)
	}

	return points, nil
}

// DataGroupStats 数据分组统计
type DataGroupStats struct {
	Group  string
	Count  int
	Points []CSVPointConfig
}

// GroupByDataGroup 按数据分组统计
func GroupByDataGroup(csvPoints []CSVPointConfig) []DataGroupStats {
	groupMap := make(map[string]*DataGroupStats)

	for _, cp := range csvPoints {
		group := cp.DataGroup
		if group == "" {
			group = "未分组"
		}

		if stats, ok := groupMap[group]; ok {
			stats.Count++
			stats.Points = append(stats.Points, cp)
		} else {
			groupMap[group] = &DataGroupStats{
				Group:  group,
				Count:  1,
				Points: []CSVPointConfig{cp},
			}
		}
	}

	result := make([]DataGroupStats, 0, len(groupMap))
	for _, stats := range groupMap {
		result = append(result, *stats)
	}

	return result
}

// parseDataGroupID 解析数据分组ID
func parseDataGroupID(groupStr string) int {
	id, err := strconv.Atoi(groupStr)
	if err != nil {
		return 0
	}
	return id
}
