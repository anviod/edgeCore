// Package sync provides multi-node config file synchronization (deprecated).
// Runtime configuration now lives in data/config.db; SyncManager is disabled in main.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/model"
)

// NodeSnapshot represents one node's configuration tree and file view.
type NodeSnapshot struct {
	NodeID     string        `json:"node_id"`
	NodeName   string        `json:"node_name,omitempty"`
	CapturedAt time.Time     `json:"captured_at"`
	Files      []ConfigFile  `json:"files"`
	Channels   []TreeChannel `json:"channels"`
	Northbound []TreeSection `json:"northbound"`
	System     []TreeSection `json:"system"`
	Summary    TreeSummary   `json:"summary"`
}

// ConfigFile captures one source config file for file-level diffing.
type ConfigFile struct {
	Path    string         `json:"path"`
	Section string         `json:"section"`
	Hash    string         `json:"hash"`
	Content any            `json:"content,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

// TreeSummary contains counters used by the UI.
type TreeSummary struct {
	ChannelCount    int `json:"channel_count"`
	DeviceCount     int `json:"device_count"`
	PointCount      int `json:"point_count"`
	NorthboundCount int `json:"northbound_count"`
	SystemCount     int `json:"system_count"`
	FileCount       int `json:"file_count"`
}

// TreeChannel is the top-level channel node.
type TreeChannel struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	Name       string         `json:"name"`
	Protocol   string         `json:"protocol"`
	Status     string         `json:"status"`
	Enabled    bool           `json:"enabled"`
	HasDiff    bool           `json:"has_diff"`
	SourceFile string         `json:"source_file,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Devices    []TreeDevice   `json:"devices,omitempty"`
}

// TreeDevice is a device node.
type TreeDevice struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Enabled    bool           `json:"enabled"`
	HasDiff    bool           `json:"has_diff"`
	PointCount int            `json:"point_count"`
	SourceFile string         `json:"source_file,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Points     []TreePoint    `json:"points,omitempty"`
}

// TreePoint is a point node.
type TreePoint struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	HasDiff    bool           `json:"has_diff"`
	SourceFile string         `json:"source_file,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
}

// TreeSection represents a node in northbound/system trees.
type TreeSection struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	Name       string         `json:"name"`
	Section    string         `json:"section"`
	Status     string         `json:"status"`
	Enabled    bool           `json:"enabled"`
	HasDiff    bool           `json:"has_diff"`
	SourceFile string         `json:"source_file,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Children   []TreeSection  `json:"children,omitempty"`
}

// DiffSummary aggregates diff counters.
type DiffSummary struct {
	Total              int `json:"total"`
	Same               int `json:"same"`
	Different          int `json:"different"`
	OnlySource         int `json:"only_source"`
	OnlyTarget         int `json:"only_target"`
	FileDifferent      int `json:"file_different"`
	StructureDifferent int `json:"structure_different"`
}

// DiffItem represents one diff entry.
type DiffItem struct {
	Key         string         `json:"key"`
	Kind        string         `json:"kind"` // file or structure
	Type        string         `json:"type"` // same, different, onlySource, onlyTarget
	Path        string         `json:"path,omitempty"`
	File        string         `json:"file,omitempty"`
	SourceValue any            `json:"source_value,omitempty"`
	TargetValue any            `json:"target_value,omitempty"`
	SourceHash  string         `json:"source_hash,omitempty"`
	TargetHash  string         `json:"target_hash,omitempty"`
	SourceMeta  map[string]any `json:"source_meta,omitempty"`
	TargetMeta  map[string]any `json:"target_meta,omitempty"`
}

// DiffResult contains file-level and structure-level diff details.
type DiffResult struct {
	SourceNodeID   string      `json:"source_node_id"`
	TargetNodeID   string      `json:"target_node_id"`
	Summary        DiffSummary `json:"summary"`
	FileLevel      []DiffItem  `json:"file_level"`
	StructureLevel []DiffItem  `json:"structure_level"`
}

// BuildNodeSnapshot converts a loaded config into a tree snapshot.
func BuildNodeSnapshot(nodeID string, cfg *config.Config) *NodeSnapshot {
	if cfg == nil {
		return &NodeSnapshot{
			NodeID:     nodeID,
			CapturedAt: time.Now(),
		}
	}

	snapshot := &NodeSnapshot{
		NodeID:     nodeID,
		NodeName:   cfg.System.Hostname.Name,
		CapturedAt: time.Now(),
	}

	snapshot.Files = buildConfigFiles(cfg)
	snapshot.Channels = buildChannelNodes(cfg)
	snapshot.Northbound = buildNorthboundNodes(cfg)
	snapshot.System = buildSystemNodes(cfg)
	snapshot.Summary = TreeSummary{
		ChannelCount:    len(snapshot.Channels),
		NorthboundCount: len(snapshot.Northbound),
		SystemCount:     len(snapshot.System),
		FileCount:       len(snapshot.Files),
	}
	for _, ch := range snapshot.Channels {
		snapshot.Summary.DeviceCount += len(ch.Devices)
		for _, dev := range ch.Devices {
			snapshot.Summary.PointCount += len(dev.Points)
		}
	}

	return snapshot
}

// CompareSnapshots produces file-level and structure-level diffs.
func CompareSnapshots(source, target *NodeSnapshot) *DiffResult {
	result := &DiffResult{}
	if source != nil {
		result.SourceNodeID = source.NodeID
	}
	if target != nil {
		result.TargetNodeID = target.NodeID
	}

	fileDiffs := compareFiles(source, target)
	structureDiffs := compareStructure(source, target)
	result.FileLevel = fileDiffs
	result.StructureLevel = structureDiffs

	for _, item := range append(fileDiffs, structureDiffs...) {
		switch item.Type {
		case "same":
			result.Summary.Same++
		case "different":
			result.Summary.Different++
		case "onlySource":
			result.Summary.OnlySource++
		case "onlyTarget":
			result.Summary.OnlyTarget++
		}
	}
	for _, item := range fileDiffs {
		if item.Type != "same" {
			result.Summary.FileDifferent++
		}
	}
	for _, item := range structureDiffs {
		if item.Type != "same" {
			result.Summary.StructureDifferent++
		}
	}
	result.Summary.Total = result.Summary.Same + result.Summary.Different + result.Summary.OnlySource + result.Summary.OnlyTarget
	return result
}

func buildConfigFiles(cfg *config.Config) []ConfigFile {
	files := []ConfigFile{
		newConfigFile("server.yaml", "server", cfg.Server),
		newConfigFile("storage.yaml", "storage", cfg.Storage),
		newConfigFile("northbound.yaml", "northbound", cfg.Northbound),
		newConfigFile("channels.yaml", "channels", cfg.Channels),
		newConfigFile("edge_rules.yaml", "edge_rules", cfg.EdgeRules),
		newConfigFile("system.yaml", "system", cfg.System),
		newConfigFile("users.yaml", "users", cfg.Users),
	}

	for _, ch := range cfg.Channels {
		for _, dev := range ch.Devices {
			path := dev.DeviceFile
			if path == "" {
				// 使用相对于 data 目录的路径，实际路径由运行时确定
				path = filepath.Join("devices", ch.Protocol, dev.ID+".yaml")
			}
			files = append(files, newConfigFile(path, "device", dev))
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func buildChannelNodes(cfg *config.Config) []TreeChannel {
	nodes := make([]TreeChannel, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		channel := TreeChannel{
			Type:       "channel",
			ID:         ch.ID,
			Label:      ch.Name,
			Name:       ch.Name,
			Protocol:   ch.Protocol,
			Status:     deriveStatus(ch.Enable, ch.NodeRuntime),
			Enabled:    ch.Enable,
			SourceFile: "channels.yaml",
			Config:     map[string]any{"id": ch.ID, "name": ch.Name, "protocol": ch.Protocol, "enable": ch.Enable, "config": ch.Config},
		}
		channel.Devices = make([]TreeDevice, 0, len(ch.Devices))
		for _, dev := range ch.Devices {
			device := TreeDevice{
				Type:       "device",
				ID:         dev.ID,
				Label:      dev.Name,
				Name:       dev.Name,
				Status:     deriveDeviceStatus(dev),
				Enabled:    dev.Enable,
				SourceFile: deviceSourceFile(ch, dev),
				Config:     deviceConfigMap(ch, dev),
			}
			device.Points = make([]TreePoint, 0, len(dev.Points))
			device.PointCount = len(dev.Points)
			for _, pt := range dev.Points {
				device.Points = append(device.Points, TreePoint{
					Type:       "point",
					ID:         pt.ID,
					Label:      pt.Name,
					Name:       pt.Name,
					Status:     pointStatus(pt),
					SourceFile: device.SourceFile,
					Config:     pointConfigMap(pt),
				})
			}
			channel.Devices = append(channel.Devices, device)
		}
		nodes = append(nodes, channel)
	}
	return nodes
}

func buildNorthboundNodes(cfg *config.Config) []TreeSection {
	items := []TreeSection{}
	if len(cfg.Northbound.MQTT) > 0 {
		for _, item := range cfg.Northbound.MQTT {
			items = append(items, sectionFromStruct("mqtt", item.ID, item.Name, item.Enable, "northbound.yaml", item))
		}
	}
	if len(cfg.Northbound.HTTP) > 0 {
		for _, item := range cfg.Northbound.HTTP {
			items = append(items, sectionFromStruct("http", item.ID, item.Name, item.Enable, "northbound.yaml", item))
		}
	}
	if len(cfg.Northbound.OPCUA) > 0 {
		for _, item := range cfg.Northbound.OPCUA {
			items = append(items, sectionFromStruct("opcua", item.ID, item.Name, item.Enable, "northbound.yaml", item))
		}
	}
	if len(cfg.Northbound.SparkplugB) > 0 {
		for _, item := range cfg.Northbound.SparkplugB {
			items = append(items, sectionFromStruct("sparkplug_b", item.ID, item.Name, item.Enable, "northbound.yaml", item))
		}
	}
	if len(cfg.Northbound.EdgeOSMQTT) > 0 {
		for _, item := range cfg.Northbound.EdgeOSMQTT {
			items = append(items, sectionFromStruct("edgeos_mqtt", item.ID, item.Name, item.Enable, "northbound.yaml", item))
		}
	}
	if len(cfg.Northbound.EdgeOSNATS) > 0 {
		for _, item := range cfg.Northbound.EdgeOSNATS {
			items = append(items, sectionFromStruct("edgeos_nats", item.ID, item.Name, item.Enable, "northbound.yaml", item))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Section == items[j].Section {
			return items[i].ID < items[j].ID
		}
		return items[i].Section < items[j].Section
	})
	return items
}

func buildSystemNodes(cfg *config.Config) []TreeSection {
	items := []TreeSection{
		sectionFromStruct("server", "server", "server.yaml", true, "server.yaml", cfg.Server),
		sectionFromStruct("storage", "storage", "storage.yaml", true, "storage.yaml", cfg.Storage),
		sectionFromStruct("system", "system", "system.yaml", true, "system.yaml", cfg.System),
	}
	for _, rule := range cfg.EdgeRules {
		name := rule.Name
		if name == "" {
			name = rule.ID
		}
		items = append(items, sectionFromStruct("edge_rule", rule.ID, name, true, "edge_rules.yaml", rule))
	}
	for _, user := range cfg.Users {
		items = append(items, sectionFromStruct("user", user.Username, user.Username, true, "users.yaml", user))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Section == items[j].Section {
			return items[i].ID < items[j].ID
		}
		return items[i].Section < items[j].Section
	})
	return items
}

func sectionFromStruct(section, id, name string, enabled bool, sourceFile string, content any) TreeSection {
	return TreeSection{
		Type:       section,
		ID:         id,
		Label:      name,
		Name:       name,
		Section:    section,
		Status:     boolStatus(enabled),
		Enabled:    enabled,
		SourceFile: sourceFile,
		Config:     normalizeMap(content),
	}
}

func deviceConfigMap(ch model.Channel, dev model.Device) map[string]any {
	data := map[string]any{
		"id":         dev.ID,
		"name":       dev.Name,
		"enable":     dev.Enable,
		"interval":   dev.Interval,
		"config":     dev.Config,
		"storage":    dev.Storage,
		"points":     dev.Points,
		"channel_id": ch.ID,
		"protocol":   ch.Protocol,
	}
	if dev.DegradeOnFailure != nil {
		data["degrade_on_failure"] = *dev.DegradeOnFailure
	}
	return data
}

func pointConfigMap(pt model.Point) map[string]any {
	return map[string]any{
		"id":            pt.ID,
		"name":          pt.Name,
		"address":       pt.Address,
		"register_type": pt.RegisterType.String(),
		"function_code": pt.FunctionCode,
		"datatype":      pt.DataType,
		"scale":         pt.Scale,
		"offset":        pt.Offset,
		"format":        pt.Format,
		"word_order":    pt.WordOrder,
		"read_formula":  pt.ReadFormula,
		"write_formula": pt.WriteFormula,
		"unit":          pt.Unit,
		"readwrite":     pt.ReadWrite,
		"group":         pt.Group,
		"scan_class":    pt.ScanClass,
		"report_mode":   pt.ReportMode,
	}
}

func deviceSourceFile(ch model.Channel, dev model.Device) string {
	if dev.DeviceFile != "" && !strings.HasPrefix(dev.DeviceFile, "data") {
		return dev.DeviceFile
	}
	return "db:devices/" + ch.Protocol + "/" + dev.ID
}

func deriveStatus(enabled bool, runtime *model.NodeRuntime) string {
	if !enabled {
		return "offline"
	}
	if runtime == nil {
		return "online"
	}
	switch runtime.State {
	case 0:
		return "online"
	case 1:
		return "warning"
	case 2, 3:
		return "offline"
	default:
		return "online"
	}
}

func deriveDeviceStatus(dev model.Device) string {
	if !dev.Enable {
		return "offline"
	}
	switch dev.State {
	case 0:
		return "online"
	case 1:
		return "warning"
	case 2, 3:
		return "offline"
	default:
		return "online"
	}
}

func pointStatus(pt model.Point) string {
	if strings.EqualFold(pt.ReadWrite, "rw") {
		return "online"
	}
	if strings.EqualFold(pt.ReadWrite, "r") {
		return "online"
	}
	return "online"
}

func boolStatus(enabled bool) string {
	if enabled {
		return "online"
	}
	return "offline"
}

func newConfigFile(path, section string, content any) ConfigFile {
	normalized := normalizeValue(content)
	hash := hashValue(normalized)
	return ConfigFile{
		Path:    path,
		Section: section,
		Hash:    hash,
		Content: normalized,
		Meta: map[string]any{
			"section": section,
		},
	}
}

func normalizeMap(content any) map[string]any {
	normalized := normalizeValue(content)
	if m, ok := normalized.(map[string]any); ok {
		return m
	}
	return map[string]any{"value": normalized}
}

func normalizeValue(content any) any {
	switch v := content.(type) {
	case nil:
		return nil
	case map[string]any:
		out := make(map[string]any, len(v))
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = normalizeValue(v[k])
		}
		return out
	case []any:
		arr := make([]any, len(v))
		for i := range v {
			arr[i] = normalizeValue(v[i])
		}
		return arr
	case string:
		return v
	case float64:
		return v
	case bool:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		var out any
		if err := json.Unmarshal(data, &out); err != nil {
			return fmt.Sprintf("%v", v)
		}
		// Check if we're going to recurse with the same type
		switch out.(type) {
		case map[string]any, []any:
			return normalizeValue(out)
		default:
			return out
		}
	}
}

func hashValue(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte(fmt.Sprintf("%v", v))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type flatEntry struct {
	Key   string
	Value any
	Hash  string
	File  string
	Meta  map[string]any
}

func compareFiles(source, target *NodeSnapshot) []DiffItem {
	sourceMap := map[string]ConfigFile{}
	targetMap := map[string]ConfigFile{}
	if source != nil {
		for _, file := range source.Files {
			sourceMap[file.Path] = file
		}
	}
	if target != nil {
		for _, file := range target.Files {
			targetMap[file.Path] = file
		}
	}

	keys := unionKeys(sourceMap, targetMap)
	items := make([]DiffItem, 0, len(keys))
	for _, key := range keys {
		s, sOK := sourceMap[key]
		t, tOK := targetMap[key]
		item := DiffItem{Key: key, Kind: "file", File: key}
		switch {
		case sOK && tOK:
			item.SourceHash = s.Hash
			item.TargetHash = t.Hash
			item.SourceValue = s.Content
			item.TargetValue = t.Content
			if s.Hash == t.Hash {
				item.Type = "same"
			} else {
				item.Type = "different"
			}
		case sOK:
			item.Type = "onlySource"
			item.SourceHash = s.Hash
			item.SourceValue = s.Content
		case tOK:
			item.Type = "onlyTarget"
			item.TargetHash = t.Hash
			item.TargetValue = t.Content
		}
		items = append(items, item)
	}
	return items
}

func compareStructure(source, target *NodeSnapshot) []DiffItem {
	sourceMap := flattenSnapshot(source)
	targetMap := flattenSnapshot(target)
	keys := unionKeys(sourceMap, targetMap)
	items := make([]DiffItem, 0, len(keys))
	for _, key := range keys {
		s, sOK := sourceMap[key]
		t, tOK := targetMap[key]
		item := DiffItem{Key: key, Kind: "structure", Path: key}
		switch {
		case sOK && tOK:
			item.SourceHash = s.Hash
			item.TargetHash = t.Hash
			item.SourceValue = s.Value
			item.TargetValue = t.Value
			item.SourceMeta = s.Meta
			item.TargetMeta = t.Meta
			if s.Hash == t.Hash {
				item.Type = "same"
			} else {
				item.Type = "different"
			}
		case sOK:
			item.Type = "onlySource"
			item.SourceHash = s.Hash
			item.SourceValue = s.Value
			item.SourceMeta = s.Meta
		case tOK:
			item.Type = "onlyTarget"
			item.TargetHash = t.Hash
			item.TargetValue = t.Value
			item.TargetMeta = t.Meta
		}
		items = append(items, item)
	}
	return items
}

func flattenSnapshot(snapshot *NodeSnapshot) map[string]flatEntry {
	result := map[string]flatEntry{}
	if snapshot == nil {
		return result
	}

	for _, ch := range snapshot.Channels {
		channelPath := "channels/" + ch.ID
		result[channelPath] = flatEntry{
			Key:   channelPath,
			Value: ch.Config,
			Hash:  hashValue(ch.Config),
			File:  ch.SourceFile,
			Meta: map[string]any{
				"type":     ch.Type,
				"label":    ch.Label,
				"protocol": ch.Protocol,
				"enabled":  ch.Enabled,
				"status":   ch.Status,
			},
		}
		for _, dev := range ch.Devices {
			devicePath := channelPath + "/devices/" + dev.ID
			result[devicePath] = flatEntry{
				Key:   devicePath,
				Value: dev.Config,
				Hash:  hashValue(dev.Config),
				File:  dev.SourceFile,
				Meta: map[string]any{
					"type":       dev.Type,
					"label":      dev.Label,
					"enabled":    dev.Enabled,
					"status":     dev.Status,
					"pointCount": dev.PointCount,
				},
			}
			for _, pt := range dev.Points {
				pointPath := devicePath + "/points/" + pt.ID
				result[pointPath] = flatEntry{
					Key:   pointPath,
					Value: pt.Config,
					Hash:  hashValue(pt.Config),
					File:  pt.SourceFile,
					Meta: map[string]any{
						"type":   pt.Type,
						"label":  pt.Label,
						"status": pt.Status,
					},
				}
			}
		}
	}

	for _, section := range snapshot.Northbound {
		key := "northbound/" + section.Section + "/" + section.ID
		result[key] = flatEntry{
			Key:   key,
			Value: section.Config,
			Hash:  hashValue(section.Config),
			File:  section.SourceFile,
			Meta: map[string]any{
				"type":    section.Type,
				"label":   section.Label,
				"enabled": section.Enabled,
				"status":  section.Status,
			},
		}
	}

	for _, section := range snapshot.System {
		key := "system/" + section.Section + "/" + section.ID
		result[key] = flatEntry{
			Key:   key,
			Value: section.Config,
			Hash:  hashValue(section.Config),
			File:  section.SourceFile,
			Meta: map[string]any{
				"type":    section.Type,
				"label":   section.Label,
				"enabled": section.Enabled,
				"status":  section.Status,
			},
		}
	}

	return result
}

func unionKeys[T any](left map[string]T, right map[string]T) []string {
	keys := make([]string, 0, len(left)+len(right))
	seen := make(map[string]struct{}, len(left)+len(right))
	for k := range left {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
			seen[k] = struct{}{}
		}
	}
	for k := range right {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
			seen[k] = struct{}{}
		}
	}
	sort.Strings(keys)
	return keys
}
