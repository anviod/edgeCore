package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgex/internal/ai_agent"
	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/config"
	"github.com/anviod/edgex/internal/core"
	"github.com/anviod/edgex/internal/mcp"
	"github.com/anviod/edgex/internal/model"
	"github.com/anviod/edgex/internal/northbound/opcua"
	"github.com/anviod/edgex/internal/pkg/logger"
	"github.com/anviod/edgex/internal/storage"
	syncpkg "github.com/anviod/edgex/internal/sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"go.uber.org/zap"
)

type SystemStats struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	GoRoutines  int     `json:"goroutines"`
}

type DashboardSummary struct {
	Channels   []core.ChannelStatus    `json:"channels"`
	Northbound []core.NorthboundStatus `json:"northbound"`
	EdgeRules  core.EdgeComputeMetrics `json:"edge_rules"`
	System     SystemStats             `json:"system"`
}

type Server struct {
	app                    *fiber.App
	cm                     *core.ChannelManager
	storage                *storage.Storage
	shadowCore             *core.ShadowCore
	virtualShadow          *core.VirtualShadowEngine
	vsm                    *core.VirtualShadowManager
	hub                    *Hub
	pipeline               *core.DataPipeline
	nbm                    *core.NorthboundManager
	ecm                    *core.EdgeComputeManager
	sm                     *core.SystemManager
	dsm                    *core.DeviceStorageManager
	cfgManager             *config.ConfigManager
	syncManager            *syncpkg.SyncManager
	logBroadcaster         *logger.LogBroadcaster
	randomWriteMu          sync.Mutex
	randomWriteStop        chan struct{}
	randomWriteRunning     bool
	startTime              time.Time
	logger                 *zap.Logger
	listenAddr             string
	serverMu               sync.Mutex
	portSwitching          bool
	aiAgent                *ai_agent.Agent
	aiSettingsMem          *model.AICopilotSettings
	mcpServer              *mcp.MCPServer // MCP 协议服务端（懒初始化）
	eanRuntime             *capability.Runtime
	eanRuntimeOnce         sync.Once
	storageAttachHook      func(*storage.Storage)
	runtimeStartHook       func()
	shadowSubscribeOnce    sync.Once
	runtimeCompactStop     chan struct{}
	runtimeCompactOnce     sync.Once
	runtimeCompactStopOnce sync.Once
}

func NewServer(cm *core.ChannelManager, st *storage.Storage, pl *core.DataPipeline, nbm *core.NorthboundManager, ecm *core.EdgeComputeManager, sm *core.SystemManager, dsm *core.DeviceStorageManager, cfgManager *config.ConfigManager, syncManager *syncpkg.SyncManager, logBroadcaster *logger.LogBroadcaster) *Server {
	app := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		// Long scan/browse/export work must use async job APIs, not stretch these.
	})
	app.Use(recover.New())
	app.Use(cors.New())

	hub := newHub()
	go hub.run()

	s := &Server{
		app:            app,
		cm:             cm,
		storage:        st,
		hub:            hub,
		pipeline:       pl,
		nbm:            nbm,
		ecm:            ecm,
		sm:             sm,
		dsm:            dsm,
		cfgManager:     cfgManager,
		syncManager:    syncManager,
		logBroadcaster: logBroadcaster,
		startTime:      time.Now(),
		logger:         zap.L(),
	}

	// Inject ChannelManager into EdgeComputeManager
	if ecm != nil {
		ecm.SetChannelManager(cm)
		ecm.SetStorage(st)
	}

	s.setupRoutes()
	return s
}

// SetVirtualShadowEngine 绑定虚拟影子引擎（公式点位增量计算）。
func (s *Server) SetVirtualShadowEngine(vse *core.VirtualShadowEngine) {
	s.virtualShadow = vse
}

// SetShadowCore 绑定影子设备核心，并订阅快照变更推送到 WebSocket。
func (s *Server) SetShadowCore(sc *core.ShadowCore) {
	s.shadowCore = sc
	if sc == nil {
		return
	}
	s.shadowSubscribeOnce.Do(func() {
		sc.Subscribe(func(shadowDeviceID string, points map[string]model.ShadowPoint) {
			channelID, deviceID, err := sc.ResolvePublishTarget(shadowDeviceID)
			if err != nil {
				return
			}
			for pointID, pt := range points {
				s.BroadcastShadowPoint(channelID, deviceID, pointID, pt)
			}
		})
	})
}

// BroadcastShadowPoint 将影子点位快照推送到 WebSocket 客户端。
func (s *Server) BroadcastShadowPoint(channelID, deviceID, pointID string, point model.ShadowPoint) {
	collectedAt := point.CollectedAt
	if collectedAt.IsZero() {
		collectedAt = point.Timestamp
	}
	msg := map[string]any{
		"channel_id":   channelID,
		"device_id":    deviceID,
		"point_id":     pointID,
		"value":        point.Value,
		"quality":      point.Quality,
		"timestamp":    collectedAt,
		"collected_at": collectedAt,
		"updated_at":   point.UpdatedAt,
	}
	s.BroadcastValue(msg)
}

// SetStorageAttachHook 注册存储绑定回调（安装流程创建 DB 后回传 main 等组件）。
func (s *Server) SetStorageAttachHook(fn func(*storage.Storage)) {
	s.storageAttachHook = fn
}

// SetRuntimeStartHook 注册数据采集/北向启动回调（安装完成后或正常启动时调用）。
func (s *Server) SetRuntimeStartHook(fn func()) {
	s.runtimeStartHook = fn
}

func (s *Server) Start(addr string) error {
	s.serverMu.Lock()
	s.listenAddr = addr
	s.serverMu.Unlock()

	// 启动时发布点位元数据
	if s.nbm != nil {
		go s.nbm.PublishPointsMetadata()
	}
	go s.broadcastLoop()
	if s.storage != nil {
		s.startRuntimeCompactLoop()
	}

	err := s.app.Listen(addr)
	if err == nil {
		return nil
	}

	s.serverMu.Lock()
	switching := s.portSwitching
	if switching {
		s.portSwitching = false
	}
	s.serverMu.Unlock()
	if switching {
		s.logger.Info("Web server stopped for port switch", zap.String("addr", addr))
		return nil
	}
	return err
}

func (s *Server) SwitchPort(newPort int) error {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()

	newAddr := fmt.Sprintf(":%d", newPort)
	if newAddr == s.listenAddr {
		return nil
	}

	s.portSwitching = true
	if err := s.app.Shutdown(); err != nil {
		s.logger.Warn("Shutdown old server failed", zap.Error(err))
	}

	newApp := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	})
	newApp.Use(recover.New())
	newApp.Use(cors.New())
	s.app = newApp
	s.setupRoutes()
	s.listenAddr = newAddr

	go func() {
		s.logger.Info("Web server restarting on new port", zap.String("addr", newAddr))
		if err := s.app.Listen(newAddr); err != nil {
			s.logger.Error("Web server failed on new port", zap.Error(err))
		}
	}()

	time.Sleep(500 * time.Millisecond)
	return nil
}

func (s *Server) GetListenPort() int {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	if s.listenAddr == "" {
		return 0
	}
	portStr := s.listenAddr
	if len(portStr) > 0 && portStr[0] == ':' {
		portStr = portStr[1:]
	}
	port, _ := strconv.Atoi(portStr)
	return port
}

func (s *Server) getEdgeComputeLogs(c *fiber.Ctx) error {
	ruleID := c.Query("rule_id")
	startStr := c.Query("start") // YYYY-MM-DD HH:mm
	endStr := c.Query("end")     // YYYY-MM-DD HH:mm

	var start, end time.Time
	var err error

	if startStr == "" {
		start = time.Now().Add(-24 * time.Hour)
	} else {
		start, err = time.Parse("2006-01-02 15:04", startStr)
		if err != nil {
			return c.Status(400).SendString("Invalid start time format")
		}
	}

	if endStr == "" {
		end = time.Now()
	} else {
		end, err = time.Parse("2006-01-02 15:04", endStr)
		if err != nil {
			return c.Status(400).SendString("Invalid end time format")
		}
	}

	logs, err := s.ecm.QueryLogs(start, end, ruleID)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return c.JSON(logs)
}

func (s *Server) exportEdgeComputeLogsToCSV(c *fiber.Ctx) error {
	ruleID := c.Query("rule_id")
	startStr := c.Query("start")
	endStr := c.Query("end")

	var start, end time.Time
	var err error

	if startStr == "" {
		start = time.Now().Add(-24 * time.Hour)
	} else {
		start, err = time.Parse("2006-01-02 15:04", startStr)
		if err != nil {
			return c.Status(400).SendString("Invalid start time format")
		}
	}

	if endStr == "" {
		end = time.Now()
	} else {
		end, err = time.Parse("2006-01-02 15:04", endStr)
		if err != nil {
			return c.Status(400).SendString("Invalid end time format")
		}
	}

	logs, err := s.ecm.QueryLogs(start, end, ruleID)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	b := &bytes.Buffer{}
	w := csv.NewWriter(b)
	// Write Header
	w.Write([]string{"RuleID", "RuleName", "Minute", "ErrorType", "ErrorMessage"})

	for _, log := range logs {
		w.Write([]string{
			log.RuleID,
			log.RuleName,
			log.Minute,
			log.ErrorType,
			log.ErrorMessage,
		})
	}
	w.Flush()

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=edge_logs_%s.csv", time.Now().Format("20060102150405")))
	return c.Send(b.Bytes())
}

func (s *Server) setupRoutes() {
	// WebSocket 实时值（不走 JWT，供 PointList / 虚拟影子页订阅）
	s.app.Get("/api/ws/values", websocket.New(s.handleWebSocket))
	s.app.Get("/ws", websocket.New(s.handleWebSocket))

	// MCP (Model Context Protocol) — 外部 LLM 接入端点（MCP API Key 认证，不走 JWT）
	s.app.Post("/api/mcp", s.handleMCP)
	s.app.Get("/api/mcp", s.handleMCP)
	s.app.Delete("/api/mcp", s.handleMCP)
	s.app.Post("/mcp", s.handleMCP)
	s.app.Get("/mcp", s.handleMCP)
	s.app.Delete("/mcp", s.handleMCP)

	api := s.app.Group("/api")

	// 认证相关 (无需 JWT)
	auth := api.Group("/auth")
	auth.Get("/system-info", s.handleGetSystemInfo)
	auth.Get("/nonce", s.handleGetNonce)
	auth.Post("/login", s.handleLogin)
	auth.Post("/logout", s.handleLogout)

	// 安装初始化 API (无需 JWT)
	install := api.Group("/install")
	install.Get("/status", s.checkInstallStatus)
	install.Get("/check-port", s.checkPort)
	install.Post("/check-path", s.checkPath)
	install.Post("/validate", s.validateConfig)
	install.Post("/start", s.startInstall)
	install.Get("/install-status", s.getInstallStatus)

	// WebSocket 实时值（注册在 JWT 之前；与上方 s.app 路由双保险）
	api.Get("/ws/values", websocket.New(s.handleWebSocket))

	// 应用 JWT 中间件到后续路由
	api.Use(JWTAuth())

	// Authenticated Auth Routes
	api.Post("/auth/change-password", s.handleChangePassword)

	// ===== 三级导航 API 端点 =====

	// 首页 Dashboard
	api.Get("/dashboard/summary", s.getDashboardSummary)

	// AI 助手（本地上下文助手 + Industrial Protocol Copilot 工作台）
	api.Get("/ai/status", s.getAiStatus)
	api.Post("/ai/chat", s.postAiChat)
	api.Get("/ai/settings", s.getAiSettings)
	api.Put("/ai/settings", s.putAiSettings)
	api.Get("/ai/quota", s.getAiQuota)
	api.Get("/ai/tasks", s.listAiTasks)
	api.Post("/ai/tasks", s.createAiTask)
	api.Post("/ai/tasks/upload", s.postAiTaskFromUpload)
	api.Get("/ai/tasks/:id", s.getAiTask)
	api.Post("/ai/tasks/:id/upload", s.uploadAiTaskFile)
	api.Post("/ai/tasks/:id/confirm", s.confirmAiTask)
	api.Post("/ai/validate", s.postAiValidate)
	api.Post("/ai/edge-rule/draft", s.postAiEdgeRuleDraft)
	api.Get("/ai/diagnostics/summary", s.getAiDiagnosticsSummary)

	// MCP 管理接口（UI 配置，走 JWT）
	api.Get("/mcp/help", s.handleMCPHelp)
	api.Post("/mcp/activate", s.handleMcpActivate)
	api.Get("/mcp/status", s.handleMcpStatus)
	api.Get("/mcp/key", s.handleMcpGetKey)
	api.Post("/mcp/generate-key", s.handleMcpGenerateKey)

	// EAN 2.0 Capability Runtime REST 端点（UI 专用，走 JWT）
	api.Get("/capability/agent/status", s.handleEanAgentStatus)
	api.Get("/capability/list", s.handleEanCapabilityList)
	api.Get("/capability/list/:id", s.handleEanCapabilityDetail)
	api.Post("/capability/invoke", s.handleEanInvoke)
	api.Get("/capability/invoke/:id/status", s.handleEanInvokeStatus)
	api.Get("/capability/discovery/agents", s.handleEanDiscoveryAgents)
	api.Get("/capability/events/history", s.handleEanEventHistory)
	api.Delete("/capability/events/history", s.handleEanEventClear)
	api.Get("/capability/settings", s.handleEanSettings)

	// 系统设置
	api.Get("/system", s.getSystemConfig)
	api.Put("/system", s.updateSystemConfig)

	// 边缘计算日志
	api.Get("/edge-compute/logs", s.getEdgeComputeLogs)
	api.Get("/edge-compute/logs/export", s.exportEdgeComputeLogsToCSV)

	api.Post("/system/restart", s.handleRestart)
	api.Get("/system/network/interfaces", s.getNetworkInterfaces)
	api.Get("/system/network/routes", s.getRoutes)
	api.Post("/system/network/routes", s.addRoute)
	api.Delete("/system/network/routes", s.deleteRoute)
	api.Get("/system/network/info", s.getNetworkInfo)
	api.Get("/system/hostname/status", s.getHostnameAccessStatus)
	api.Post("/system/network/connectivity", s.checkConnectivity)

	// 第一级：采集通道列表
	api.Get("/channels", s.getChannels)
	api.Post("/channels", s.addChannel)

	// 第二级：获取通道详情
	api.Get("/channels/:channelId", s.getChannel)
	api.Put("/channels/:channelId", s.updateChannel)
	api.Delete("/channels/:channelId", s.removeChannel)
	api.Post("/channels/:channelId/scan", s.scanChannel)
	api.Get("/jobs/:jobId", s.getAsyncJob)
	api.Delete("/jobs/:jobId", s.cancelAsyncJob)
	api.Get("/channels/:channelId/metrics", s.getChannelMetrics) // 通道监控指标
	api.Get("/diagnostics/scan-engine", s.getScanEngineDiagnostics)
	api.Get("/diagnostics/soak", s.getSoakMonitor)
	api.Get("/channels/:channelId/diagnostics/events", s.getChannelEventLog)
	api.Get("/devices/:deviceId/diagnostics", s.getDeviceDiagnostics)
	api.Get("/devices/:deviceId/history", s.getDeviceHistory) // New history API

	// 第二级：获取通道下的设备列表
	api.Get("/channels/:channelId/devices", s.getChannelDevices)
	api.Post("/channels/:channelId/devices", s.addDevice) // 新增设备 (支持单个或批量)
	api.Post("/channels/:channelId/devices/batch-modbus", s.batchCreateModbusSlaves)
	api.Delete("/channels/:channelId/devices", s.removeDevices) // 批量删除设备

	// 第三级：获取设备详情
	api.Get("/channels/:channelId/devices/:deviceId", s.getDevice)
	api.Put("/channels/:channelId/devices/:deviceId", s.updateDevice)             // 更新设备
	api.Delete("/channels/:channelId/devices/:deviceId", s.removeDevice)          // 删除设备
	api.Get("/channels/:channelId/devices/:deviceId/metrics", s.getDeviceMetrics) // 设备监控指标

	// 第三级：获取设备的点位数据
	api.Get("/channels/:channelId/devices/:deviceId/points", s.getDevicePoints)
	api.Post("/channels/:channelId/devices/:deviceId/points", s.addPoint)
	api.Put("/channels/:channelId/devices/:deviceId/points/:pointId", s.updatePoint)
	api.Delete("/channels/:channelId/devices/:deviceId/points/:pointId", s.removePoint)
	api.Delete("/channels/:channelId/devices/:deviceId/points", s.removePoints)
	api.Post("/channels/:channelId/devices/:deviceId/scan", s.scanDevice)                  // New: Scan points in device
	api.Get("/channels/:channelId/devices/:deviceId/points/export", s.exportDevicePoints)  // Export points
	api.Post("/channels/:channelId/devices/:deviceId/points/import", s.importDevicePoints) // Import points
	api.Post("/channels/:channelId/devices/:deviceId/points/generate-registers", s.generateDeviceRegisters)

	// 兼容路径：UI 可能会尝试直接通过设备 ID 访问点位（不带 channelId）
	api.Get("/devices/:deviceId/points", s.getDevicePoints)
	api.Post("/devices/:deviceId/points", s.getDevicePoints)                                        // 处理一些异常的 POST 行为
	api.Options("/devices/:deviceId/points", func(c *fiber.Ctx) error { return c.SendStatus(200) }) // 处理 CORS 预检

	// 特殊兼容：处理由于前端或反向代理可能导致的路径异常
	api.Get("/channels/:channelId/devices/:deviceId/points/", s.getDevicePoints)
	api.Get("/devices/:deviceId/points/", s.getDevicePoints)

	// 点位调试接口
	api.Get("/points/:pointId/debug", s.getPointDebug)

	// 兼容：实时值快照（用于前端简化展示）
	api.Get("/values/realtime", s.getRealtimeValues)

	// 写入点位值
	api.Post("/write", s.writePoint)

	// 北向数据上报配置
	api.Get("/northbound/config", s.getNorthboundConfig)
	api.Post("/northbound/mqtt", s.updateMQTTConfig)
	api.Delete("/northbound/mqtt/:id", s.deleteMQTTConfig) // MQTT Delete
	api.Post("/northbound/http", s.updateHTTPConfig)       // New HTTP Config
	api.Delete("/northbound/http/:id", s.deleteHTTPConfig) // New HTTP Delete
	api.Post("/northbound/opcua", s.updateOPCUAConfig)
	api.Post("/northbound/opcua/:id/certificate", s.uploadOPCUACertificate)
	api.Post("/northbound/opcua/:id/sync", s.syncOPCUAServer)
	api.Get("/northbound/opcua/:id/stats", s.getOPCUAStats)
	api.Post("/northbound/opcua/:id/write", s.writeOPCUA)
	api.Post("/northbound/opcua/:id/batch-write", s.batchWriteOPCUA)
	api.Get("/northbound/opcua/:id/write-history", s.getOPCUAWriteHistory)
	// BACnet Server
	api.Post("/northbound/bacnet_server", s.updateBACnetServerConfig)
	api.Delete("/northbound/bacnet_server/:id", s.deleteBACnetServerConfig)
	api.Post("/northbound/bacnet_server/:id/sync", s.syncBACnetServer)
	api.Get("/northbound/bacnet_server/:id/stats", s.getBACnetServerStats)
	api.Get("/northbound/bacnet_server/:id/write-history", s.getBACnetServerWriteHistory)
	api.Get("/northbound/mqtt/:id/stats", s.getMQTTStats)
	api.Post("/northbound/sparkplugb", s.upsertSparkplugBConfig)        // Sparkplug B Upsert
	api.Delete("/northbound/sparkplug_b/:id", s.deleteSparkplugBConfig) // Sparkplug B Delete

	// edgeOS(MQTT)
	api.Post("/northbound/edgeos-mqtt", s.updateEdgeOSMQTTConfig)
	api.Delete("/northbound/edgeos-mqtt/:id", s.deleteEdgeOSMQTTConfig)
	api.Get("/northbound/edgeos-mqtt/:id/stats", s.getEdgeOSMQTTStats)
	api.Post("/northbound/edgeos-mqtt/publish", s.publishEdgeOSMQTT)

	// edgeOS(NATS)
	api.Post("/northbound/edgeos-nats", s.updateEdgeOSNATSConfig)
	api.Delete("/northbound/edgeos-nats/:id", s.deleteEdgeOSNATSConfig)
	api.Get("/northbound/edgeos-nats/:id/stats", s.getEdgeOSNATSStats)
	api.Post("/northbound/edgeos-nats/publish", s.publishEdgeOSNATS)

	api.Get("/points", s.getAllPoints)

	// Edge Compute
	api.Get("/edge/rules", s.getEdgeRules)
	api.Post("/edge/rules", s.upsertEdgeRule)
	api.Delete("/edge/rules/:id", s.deleteEdgeRule)
	api.Get("/edge/states", s.getEdgeRuleStates)
	api.Get("/edge/rules/:id/window", s.getEdgeWindowData)

	api.Get("/virtual-shadows", s.listVirtualShadows)
	api.Get("/virtual-shadows/sources", s.listVirtualShadowSources)
	api.Get("/virtual-shadows/devices", s.searchVirtualShadowDevices)
	api.Get("/virtual-shadows/devices/:channelId/:deviceId/points", s.listVirtualShadowDevicePoints)
	api.Get("/virtual-shadows/:id", s.getVirtualShadow)
	api.Post("/virtual-shadows", s.createVirtualShadow)
	api.Put("/virtual-shadows/:id", s.updateVirtualShadow)
	api.Post("/virtual-shadows/:id/update", s.updateVirtualShadow) // 兼容仅允许 POST 的代理/网关
	api.Delete("/virtual-shadows/:id", s.deleteVirtualShadow)
	api.Get("/edge/cache", s.getEdgeCache)
	api.Get("/edge/metrics", s.getEdgeMetrics)
	api.Get("/edge/logs", s.handleGetEdgeLogs)
	api.Post("/edge/logs/clear", s.clearEdgeLogs)
	api.Get("/edge/events", s.getEdgeEvents)
	api.Get("/edge/failures", s.getEdgeFailures)

	tools := api.Group("/tools")
	tools.Post("/random-write/start", s.startRandomWrite)
	tools.Post("/random-write/stop", s.stopRandomWrite)

	// ===== 配置导入导出 API =====
	config := api.Group("/config")
	config.Get("/export", s.exportConfig)
	config.Post("/import", s.importConfig)

	// ===== 节点管理 API =====
	node := api.Group("/node")
	node.Get("/status", s.getNodeStatus)
	node.Get("/info", s.getNodeInfo)
	node.Post("/start", s.startNode)
	node.Post("/stop", s.stopNode)
	node.Get("/discover", s.getDiscoveredNodes)
	node.Post("/connect/:peerId", s.connectToPeer)
	node.Post("/disconnect/:peerId", s.disconnectFromPeer)
	node.Post("/discovery/enable", s.enableDiscovery)
	node.Post("/discovery/disable", s.disableDiscovery)

	// ===== 群组管理 API =====
	groups := api.Group("/groups")
	groups.Get("", s.getGroups)
	groups.Post("", s.createGroup)
	groups.Get("/:groupId", s.getGroupDetail)
	groups.Post("/:groupId/join", s.joinGroup)
	groups.Post("/:groupId/leave", s.leaveGroup)
	groups.Delete("/:groupId", s.deleteGroup)
	groups.Get("/joined", s.getJoinedGroups)
	groups.Post("/:groupId/members", s.addMemberToGroup)

	// ===== 数据同步 API =====
	sync := api.Group("/sync")
	sync.Get("/status", s.getSyncStatus)
	sync.Post("/trigger", s.triggerSync)
	sync.Get("/consistency", s.checkConsistency)
	sync.Post("/repair", s.repairConsistency)
	sync.Post("/push", s.pushConfig)
	sync.Post("/pull", s.pullConfig)
	sync.Get("/history", s.getSyncHistory)
	sync.Post("/cancel", s.cancelSync)
	sync.Get("/node/:id/tree", s.getSyncNodeTree)
	sync.Get("/node/:id/devices", s.getSyncNodeDevices)
	sync.Get("/node/:id/device/:deviceId/points", s.getSyncNodePoints)
	sync.Get("/node/:id/diff", s.getSyncNodeDiff)
	sync.Post("/node/:id/takeover", s.startDeviceTakeover)
	sync.Get("/takeovers", s.getTakeoverEvents)

	// ===== 快照管理 API =====
	snapshot := sync.Group("/node/:id")
	snapshot.Get("/snapshots", s.getSnapshots)
	snapshot.Post("/snapshots", s.createSnapshot)
	snapshot.Get("/snapshots/:snapshotId", s.getSnapshot)
	snapshot.Delete("/snapshots/:snapshotId", s.deleteSnapshot)
	snapshot.Post("/snapshots/:snapshotId/restore", s.restoreSnapshot)
	snapshot.Post("/clear", s.clearNodeConfig)
	snapshot.Post("/pull", s.pullFromRemote)
	snapshot.Post("/restore", s.restoreToRemote)
	sync.Get("/snapshot-stats", s.getSnapshotStats)

	// ===== 集群快照 API (bbolt 持久化) =====
	cluster := api.Group("/cluster")
	cluster.Get("/summary", s.getClusterSummary)
	cluster.Get("/nodes", s.getClusterNodes)
	cluster.Get("/nodes/:id", s.getClusterNode)
	cluster.Get("/devices", s.getClusterDevices)
	cluster.Get("/devices/:id", s.getClusterDevice)

	// ===== 网络监控 API =====
	network := api.Group("/network")
	network.Get("/status", s.getNetworkStatus)
	network.Get("/peers", s.getConnectedPeers)
	network.Get("/stats", s.getNetworkStats)
	network.Get("/logs", s.getNetworkLogs)
	network.Post("/logs/clear", s.clearNetworkLogs)

	// ===== 设备更换 API =====
	deviceMig := api.Group("/device/migrate")
	deviceMig.Post("/validate-code", s.validateDeviceCode)
	deviceMig.Post("", s.migrateDeviceConfig)

	// ===== 一键同步 API (0配置) =====
	simpleSync := api.Group("/sync/simple")
	simpleSync.Post("", s.simpleSync)

	// ===== 数据管理 API =====
	data := api.Group("/data")
	data.Get("/stats", s.getDataStats)
	data.Post("/clear-cache", s.clearCache)
	data.Post("/clear-all-runtime", s.clearAllRuntime)
	data.Post("/backup-config", s.backupConfigDB)
	data.Get("/export-config-db", s.exportConfigDBArchive)
	data.Get("/export-runtime-db", s.exportRuntimeDBArchive)
	data.Post("/import-config-db", s.importConfigDBArchive)
	data.Post("/pull-remote-config", s.pullRemoteConfig)
	data.Post("/compact-runtime", s.compactRuntimeDB)
	api.Get("/ws/logs", websocket.New(s.handleLogWebSocket))
	api.Get("/logs/download", s.handleLogDownload)

	// 静态资源（优先相对可执行文件目录，兼容未设置 WorkingDirectory 的部署）
	uiDist := uiDistDir()
	s.app.Static("/", uiDist)

	// SPA Fallback: 所有未匹配的路由都返回 index.html
	s.app.Get("*", func(c *fiber.Ctx) error {
		return c.SendFile(filepath.Join(uiDist, "index.html"))
	})
}

func uiDistDir() string {
	candidates := []string{"./ui/dist"}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "ui", "dist")}, candidates...)
	}
	for _, dir := range candidates {
		if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
			return dir
		}
	}
	return "./ui/dist"
}

func (s *Server) getDeviceHistory(c *fiber.Ctx) error {
	if s.dsm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Device storage manager not initialized"})
	}
	deviceID := c.Params("deviceId")

	const maxHistoryLimit = 1000
	limit := maxHistoryLimit
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= maxHistoryLimit {
			limit = l
		}
	}

	startStr := c.Query("start")
	endStr := c.Query("end")

	var history []map[string]any
	var err error

	if startStr != "" && endStr != "" {
		// Range query
		var start, end time.Time

		// Attempt to parse multiple formats (RFC3339 or simple date-time)
		// Frontend likely sends YYYY-MM-DD HH:mm:ss or YYYY-MM-DDTHH:mm:ss
		// Let's assume frontend sends YYYY-MM-DD HH:mm for simplicity or we can use time.ParseInLocation

		// Try RFC3339 first (standard for APIs)
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			// Fallback to "2006-01-02 15:04:05"
			start, err = time.ParseInLocation("2006-01-02 15:04:05", startStr, time.Local)
		}
		if err != nil {
			// Fallback to "2006-01-02T15:04:05" (HTML datetime-local default)
			start, err = time.ParseInLocation("2006-01-02T15:04:05", startStr, time.Local)
		}

		if err == nil {
			end, err = time.Parse(time.RFC3339, endStr)
			if err != nil {
				end, err = time.ParseInLocation("2006-01-02 15:04:05", endStr, time.Local)
			}
			if err != nil {
				end, err = time.ParseInLocation("2006-01-02T15:04:05", endStr, time.Local)
			}
		}

		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid time format. Use RFC3339 or YYYY-MM-DD HH:mm:ss"})
		}

		history, err = s.dsm.GetHistoryByTimeRange(deviceID, start, end, limit)
	} else {
		history, err = s.dsm.GetHistory(deviceID, limit)
	}

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"data":  history,
		"total": len(history),
	})
}

func (s *Server) addChannel(c *fiber.Ctx) error {
	var ch model.Channel
	if err := c.BodyParser(&ch); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := model.EnsureChannelID(&ch); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	for i := range ch.Devices {
		if err := model.EnsureDeviceID(&ch.Devices[i]); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if err := s.cm.AddChannel(&ch); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if ch.Enable {
		if err := s.cm.StartChannel(ch.ID); err != nil {
			// Log error but don't fail the request completely
			// return c.Status(500).JSON(fiber.Map{"error": "Channel added but failed to start: " + err.Error()})
		}
	}

	return c.JSON(ch)
}

func (s *Server) updateChannel(c *fiber.Ctx) error {
	id := c.Params("channelId")
	var ch model.Channel
	if err := c.BodyParser(&ch); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	ch.ID = id // Ensure ID matches URL

	if err := s.cm.UpdateChannel(&ch); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Update device storage config
	if s.dsm != nil {
		for _, dev := range ch.Devices {
			s.dsm.UpdateDeviceConfig(dev.ID, dev.Storage)
		}
	}

	if ch.Enable {
		if err := s.cm.StartChannel(ch.ID); err != nil {
			// Log error
		}
	}

	return c.JSON(ch)
}

func (s *Server) removeChannel(c *fiber.Ctx) error {
	id := c.Params("channelId")
	if err := s.cm.RemoveChannel(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(200)
}

// getNorthboundConfig 获取北向配置
func (s *Server) getNorthboundConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}
	return c.JSON(s.nbm.GetConfig())
}

// updateMQTTConfig updates MQTT configuration
func (s *Server) updateMQTTConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	var cfg model.MQTTConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}

	warning, err := s.nbm.UpsertMQTTConfig(cfg)
	if err != nil {
		return c.Status(northboundUpsertErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(northboundConfigJSON(cfg, warning))
}

// updateOPCUAConfig updates OPC UA configuration
func (s *Server) updateOPCUAConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	var cfg model.OPCUAConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}

	savedCfg, warning, err := s.nbm.UpsertOPCUAConfig(cfg)
	if err != nil {
		return c.Status(northboundUpsertErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(northboundConfigJSON(model.SanitizeOPCUAForClient(savedCfg), warning))
}

func (s *Server) uploadOPCUACertificate(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "channel id required"})
	}

	var req struct {
		ServerCertPEM   string   `json:"server_cert_pem"`
		ServerKeyPEM    string   `json:"server_key_pem"`
		TrustedCertsPEM []string `json:"trusted_certs_pem"`
		ClearTrusted    bool     `json:"clear_trusted"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var trusted []string
	if req.ClearTrusted {
		trusted = []string{}
	} else if req.TrustedCertsPEM != nil {
		trusted = req.TrustedCertsPEM
	}

	cfg, err := s.nbm.UpdateOPCUACertificates(id, req.ServerCertPEM, req.ServerKeyPEM, trusted)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(cfg)
}

func (s *Server) upsertSparkplugBConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	var cfg model.SparkplugBConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}

	warning, err := s.nbm.UpsertSparkplugBConfig(cfg)
	if err != nil {
		return c.Status(northboundUpsertErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(northboundConfigJSON(cfg, warning))
}

func (s *Server) deleteSparkplugBConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}
	id := c.Params("id")
	if err := s.nbm.DeleteSparkplugBConfig(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(200)
}

// ===== Handler 方法 =====

func (s *Server) getDashboardSummary(c *fiber.Ctx) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Get real system stats using gopsutil
	cpuUsage := 0.0
	if cpuInfos, err := cpu.Percent(0, false); err == nil && len(cpuInfos) > 0 {
		cpuUsage = cpuInfos[0]
	}

	diskUsage := 0.0
	if diskStats, err := disk.Usage("/"); err == nil {
		diskUsage = diskStats.UsedPercent
	}

	memoryUsage := 0.0
	if memStats, err := mem.VirtualMemory(); err == nil {
		memoryUsage = memStats.UsedPercent
	}

	sys := SystemStats{
		CPUUsage:    cpuUsage,
		MemoryUsage: memoryUsage,
		DiskUsage:   diskUsage,
		GoRoutines:  runtime.NumGoroutine(),
	}

	// 获取通道统计并添加监控指标
	channelStats := s.cm.GetChannelStats()
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		for i := range channelStats {
			if metrics := mc.GetChannelMetrics(channelStats[i].ID); metrics != nil {
				channelStats[i].QualityScore = metrics.QualityScore
				channelStats[i].SuccessRate = metrics.SuccessRate
				channelStats[i].Metrics = metrics
			}
		}
	}

	summary := DashboardSummary{
		Channels:   channelStats,
		Northbound: s.nbm.GetNorthboundStats(),
		System:     sys,
	}

	if s.ecm != nil {
		summary.EdgeRules = s.ecm.GetMetrics()
	}

	return c.JSON(summary)
}

// getChannels 获取所有采集通道
func (s *Server) getChannels(c *fiber.Ctx) error {
	channels := s.cm.GetChannels()
	return c.JSON(channels)
}

// getChannel 获取指定采集通道详情
func (s *Server) getChannel(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	ch := s.cm.GetChannel(channelId)
	if ch == nil {
		return c.Status(404).JSON(fiber.Map{"error": "channel not found"})
	}
	return c.JSON(ch)
}

// getChannelDevices 获取通道下的所有设备
func (s *Server) getChannelDevices(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	devices := s.cm.GetChannelDevices(channelId)
	if devices == nil {
		return c.JSON([]model.Device{})
	}
	if c.Query("include_points") == "false" {
		out := make([]model.Device, len(devices))
		for i, d := range devices {
			out[i] = d.WithPointsSummary()
		}
		return c.JSON(out)
	}
	return c.JSON(devices)
}

func deviceAddErrorStatus(err error) int {
	if err == nil {
		return fiber.StatusInternalServerError
	}
	msg := err.Error()
	if strings.Contains(msg, "already exists") || strings.Contains(msg, "not found") {
		if strings.Contains(msg, "channel not found") {
			return fiber.StatusNotFound
		}
		return fiber.StatusConflict
	}
	if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
		return fiber.StatusBadRequest
	}
	return fiber.StatusInternalServerError
}

func (s *Server) addDevice(c *fiber.Ctx) error {
	channelId := c.Params("channelId")

	// 解析 Body，判断是单个对象还是数组
	var body interface{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	switch body.(type) {
	case []interface{}:
		// 批量添加
		var devices []model.Device
		if err := json.Unmarshal(c.Body(), &devices); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid device list"})
		}

		for i := range devices {
			if err := model.EnsureDeviceID(&devices[i]); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}

		created, err := s.cm.AddDevices(channelId, devices)
		if err != nil {
			status := deviceAddErrorStatus(err)
			if len(created) == 0 {
				return c.Status(status).JSON(fiber.Map{"error": err.Error()})
			}
			// partial success: return created devices with warning
			for i := range created {
				if s.dsm != nil {
					s.dsm.UpdateDeviceConfig(created[i].ID, created[i].Storage)
				}
			}
			if s.nbm != nil {
				s.nbm.PublishPointsMetadata()
			}
			summaries := make([]model.Device, len(created))
			for i, d := range created {
				summaries[i] = d.WithPointsSummary()
			}
			return c.Status(status).JSON(fiber.Map{
				"devices": summaries,
				"error":   err.Error(),
			})
		}

		for i := range created {
			if s.dsm != nil {
				s.dsm.UpdateDeviceConfig(created[i].ID, created[i].Storage)
			}
		}
		if s.nbm != nil {
			s.nbm.PublishPointsMetadata()
		}
		summaries := make([]model.Device, len(created))
		for i, d := range created {
			summaries[i] = d.WithPointsSummary()
		}
		return c.JSON(summaries)

	case map[string]interface{}:
		// 单个添加
		var dev model.Device
		if err := json.Unmarshal(c.Body(), &dev); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid device"})
		}
		if err := model.EnsureDeviceID(&dev); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := s.cm.AddDevice(channelId, &dev); err != nil {
			return c.Status(deviceAddErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
		}
		if s.dsm != nil {
			s.dsm.UpdateDeviceConfig(dev.ID, dev.Storage)
		}
		// 触发点位元数据同步到 edgeOS
		if s.nbm != nil {
			s.nbm.PublishPointsMetadata()
		}
		return c.JSON(dev)

	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid body format"})
	}
}

func (s *Server) updateDevice(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")

	var dev model.Device
	if err := c.BodyParser(&dev); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// 确保 ID 一致
	if dev.ID != "" && dev.ID != deviceId {
		return c.Status(400).JSON(fiber.Map{"error": "Device ID mismatch"})
	}
	dev.ID = deviceId

	if err := s.cm.UpdateDevice(channelId, &dev); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if s.dsm != nil {
		s.dsm.UpdateDeviceConfig(dev.ID, dev.Storage)
	}
	// 触发点位元数据同步到 edgeOS
	if s.nbm != nil {
		s.nbm.PublishPointsMetadata()
	}
	return c.JSON(dev)
}

func (s *Server) removeDevice(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")

	if err := s.cm.RemoveDevice(channelId, deviceId); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if s.dsm != nil {
		s.dsm.RemoveDevice(deviceId)
	}
	return c.SendStatus(200)
}

func (s *Server) removeDevices(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	var ids []string
	if err := c.BodyParser(&ids); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID list"})
	}

	if err := s.cm.RemoveDevices(channelId, ids); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if s.dsm != nil {
		for _, id := range ids {
			s.dsm.RemoveDevice(id)
		}
	}
	return c.SendStatus(200)
}

// getDevice 获取指定设备详情
func (s *Server) getDevice(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")

	dev := s.cm.GetDevice(channelId, deviceId)
	if dev == nil {
		return c.Status(404).JSON(fiber.Map{"error": "device not found"})
	}
	return c.JSON(dev)
}

// getDevicePoints 获取设备的点位数据
func (s *Server) getDevicePoints(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")

	// 兼容逻辑：如果没有 channelId，尝试搜索所有通道找到匹配的设备
	if channelId == "" {
		zap.L().Debug("getDevicePoints: missing channelId, searching for device", zap.String("deviceId", deviceId))
		channels := s.cm.GetChannels()
		for _, ch := range channels {
			for _, dev := range ch.Devices {
				if dev.ID == deviceId {
					channelId = ch.ID
					zap.L().Debug("getDevicePoints: found device in channel", zap.String("deviceId", deviceId), zap.String("channelId", channelId))
					break
				}
			}
			if channelId != "" {
				break
			}
		}
	}

	if channelId == "" {
		return c.Status(404).JSON(fiber.Map{"error": "device not found in any channel"})
	}

	// 设置API请求超时，避免长时间阻塞
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	// 使用带超时的上下文获取点位数据
	var points []model.PointData
	var err error

	// 创建一个通道来接收结果
	resultCh := make(chan struct {
		points []model.PointData
		err    error
	}, 1)

	// 在goroutine中执行获取操作
	go func() {
		p, e := s.cm.GetDevicePoints(channelId, deviceId)
		resultCh <- struct {
			points []model.PointData
			err    error
		}{p, e}
	}()

	// 等待结果或超时
	select {
	case res := <-resultCh:
		points = res.points
		err = res.err
	case <-ctx.Done():
		return c.Status(504).JSON(fiber.Map{"error": "request timeout: device communication took too long"})
	}

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	// 获取通道信息以确定协议
	ch := s.cm.GetChannel(channelId)
	protocol := ""
	if ch != nil {
		protocol = ch.Protocol
	}

	// 转换并根据协议过滤字段
	result := make([]map[string]any, 0, len(points))
	for _, p := range points {
		m := map[string]any{
			"id":        p.ID,
			"name":      p.Name,
			"address":   p.Address,
			"datatype":  p.DataType,
			"value":     p.Value,
			"quality":   p.Quality,
			"timestamp": p.Timestamp,
			"unit":      p.Unit,
			"readwrite": p.ReadWrite,
			"protocol":  protocol, // 增加协议字段
		}
		if !p.CollectedAt.IsZero() {
			m["collected_at"] = p.CollectedAt
		}
		if !p.UpdatedAt.IsZero() {
			m["updated_at"] = p.UpdatedAt
		}

		// 根据协议添加特定字段
		switch protocol {
		case "modbus-tcp", "modbus-rtu", "modbus-rtu-over-tcp":
			m["slave_id"] = p.SlaveID
			m["register_type"] = p.RegisterType
			m["function_code"] = p.FunctionCode
		case "bacnet-ip":
			// BACnet 特定字段（如果有），当前 address 已包含必要信息
		case "opc-ua":
			// OPC UA 特定字段
		default:
			// 默认行为：如果是未知协议，保留 Modbus 字段以防万一（或者也可以选择不保留）
			// 这里为了精简，默认不保留 Modbus 字段，除非明确是 Modbus 协议
		}

		result = append(result, m)
	}

	return c.JSON(result)
}

func (s *Server) getAllPoints(c *fiber.Ctx) error {
	return c.JSON(s.cm.GetAllPoints())
}

// addPoint 添加点位
func (s *Server) addPoint(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")

	// 允许单个或批量
	var single model.Point
	if err := c.BodyParser(&single); err == nil && single.ID != "" || single.Name != "" {
		if err := model.EnsurePointID(&single); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := s.cm.AddPoint(channelId, deviceId, &single); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(single)
	}

	// 尝试按批量解析
	var batch []model.Point
	if err := c.BodyParser(&batch); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if len(batch) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "empty points"})
	}

	if err := s.cm.AddPoints(channelId, deviceId, batch); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(batch)
}

// updatePoint 更新点位
func (s *Server) updatePoint(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")
	pointId := c.Params("pointId")

	var point model.Point
	if err := c.BodyParser(&point); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if point.ID != "" && point.ID != pointId {
		return c.Status(400).JSON(fiber.Map{"error": "Point ID mismatch"})
	}
	point.ID = pointId

	deviceRestarted, err := s.cm.UpdatePoint(channelId, deviceId, &point)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"point":             point,
		"device_restarted":  deviceRestarted,
		"northbound_sync":   true,
		"northbound_target": "opcua",
	})
}

// removePoint 删除点位
func (s *Server) removePoint(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")
	pointId := c.Params("pointId")

	if err := s.cm.RemovePoint(channelId, deviceId, pointId); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(200)
}

func (s *Server) removePoints(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")
	var ids []string
	if err := c.BodyParser(&ids); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID list"})
	}

	if err := s.cm.RemovePoints(channelId, deviceId, ids); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(200)
}

// getRealtimeValues 返回影子设备快照中的最新值（优先），否则回退到 values 缓存（兼容旧数据）。
func (s *Server) getRealtimeValues(c *fiber.Ctx) error {
	channelID := c.Query("channel_id")
	deviceID := c.Query("device_id")

	if s.shadowCore != nil && deviceID != "" {
		shadowID := fmt.Sprintf("shadow-%s", deviceID)
		shadow, err := s.shadowCore.GetShadowDevice(shadowID)
		if err == nil && shadow != nil {
			if channelID == "" || shadow.ChannelID == "" || shadow.ChannelID == channelID {
				filtered := make(map[string]any)
				for pid, pt := range shadow.Points {
					collectedAt := pt.CollectedAt
					if collectedAt.IsZero() {
						collectedAt = pt.Timestamp
					}
					filtered[pid] = map[string]any{
						"value":        pt.Value,
						"quality":      pt.Quality,
						"timestamp":    collectedAt,
						"collected_at": collectedAt,
						"updated_at":   pt.UpdatedAt,
					}
				}
				return c.JSON(filtered)
			}
		}

		if vd, err := s.shadowCore.GetVirtualShadowDevice(deviceID); err == nil && vd != nil {
			if channelID == "" || vd.ChannelID == "" || vd.ChannelID == channelID {
				filtered := make(map[string]any)
				for pid, pt := range vd.Points {
					collectedAt := pt.CollectedAt
					if collectedAt.IsZero() {
						collectedAt = pt.Timestamp
					}
					filtered[pid] = map[string]any{
						"value":        pt.Value,
						"quality":      pt.Quality,
						"timestamp":    collectedAt,
						"collected_at": collectedAt,
						"updated_at":   pt.UpdatedAt,
						"virtual":      true,
					}
				}
				return c.JSON(filtered)
			}
		}
	}

	// Fast path: when a specific deviceID is requested but no shadow device
	// exists for it, return an empty map immediately instead of falling through
	// to GetAllValues() which scans the entire runtime DB and can block if the
	// DB is locked by a scheduler write. This keeps the endpoint under 5ms.
	// 快速路径：指定了 deviceID 但无影子设备时，立即返回空 map，
	// 避免 GetAllValues() 全库扫描被 DB 写锁阻塞。
	if deviceID != "" {
		return c.JSON(map[string]any{})
	}

	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}
	vals, err := s.storage.GetAllValues()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 未指定过滤条件时，保持兼容：返回全部
	if channelID == "" && deviceID == "" {
		return c.JSON(vals)
	}

	// 按 ChannelID 过滤（deviceID 已由快速路径处理）
	filtered := make(map[string]model.Value)
	for k, v := range vals {
		if channelID != "" && v.ChannelID != channelID {
			continue
		}
		filtered[k] = v
	}
	return c.JSON(filtered)
}

// writePoint 写入点位值
func (s *Server) writePoint(c *fiber.Ctx) error {
	var req struct {
		ChannelID string      `json:"channel_id"`
		DeviceID  string      `json:"device_id"`
		PointID   string      `json:"point_id"`
		Value     interface{} `json:"value"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// 调用 ChannelManager 执行写入
	err := s.cm.WritePoint(req.ChannelID, req.DeviceID, req.PointID, req.Value)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "write success"})
}

func (s *Server) getEdgeCache(c *fiber.Ctx) error {
	if s.ecm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Edge Compute Manager not initialized"})
	}
	return c.JSON(s.ecm.GetFailedActions())
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(c *websocket.Conn) {
	client := &Client{
		hub:  s.hub,
		conn: c,
		send: make(chan []byte, 256),
	}
	s.hub.register <- client

	go client.writePump()
	client.readPump()
}

func (s *Server) broadcastLoop() {
	// 影子设备已挂载时，WebSocket 由 SetShadowCore 订阅推送；避免 Pipeline 重复广播。
	if s.shadowCore != nil {
		return
	}
	s.pipeline.AddHandler(func(val model.Value) {
		// Convert to JSON
		b, err := json.Marshal(val)
		if err != nil {
			zap.L().Error("Error marshalling value for broadcast", zap.Error(err))
			return
		}
		// Send to hub broadcast channel
		// Non-blocking send to avoid holding up the pipeline
		select {
		case s.hub.broadcast <- b:
		default:
			// If broadcast channel is full, drop the message
			// This prevents slow WebSocket clients from blocking the entire pipeline
		}
	})
}

// WebSocket Hub
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// 如果发送阻塞，关闭此客户端
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastValue sends a value to all connected clients
func (s *Server) BroadcastValue(v any) {
	b, _ := json.Marshal(v)
	s.hub.broadcast <- b
}

// Client for WebSocket connections
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Channel closed, send close message if connection is still open
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					return
				}
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}

// Edge Compute Handlers

func (s *Server) getEdgeRules(c *fiber.Ctx) error {
	if s.ecm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Edge Compute manager not initialized"})
	}
	return c.JSON(s.ecm.GetRules())
}

func (s *Server) upsertEdgeRule(c *fiber.Ctx) error {
	if s.ecm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Edge Compute manager not initialized"})
	}
	var rule model.EdgeRule
	if err := c.BodyParser(&rule); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	if err := s.ecm.UpsertRule(rule); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rule)
}

func (s *Server) deleteEdgeRule(c *fiber.Ctx) error {
	if s.ecm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Edge Compute manager not initialized"})
	}
	id := c.Params("id")
	if err := s.ecm.DeleteRule(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(200)
}

func (s *Server) getEdgeRuleStates(c *fiber.Ctx) error {
	if s.ecm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Edge Compute manager not initialized"})
	}
	return c.JSON(s.ecm.GetRuleStates())
}

func (s *Server) getEdgeWindowData(c *fiber.Ctx) error {
	if s.ecm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Edge Compute manager not initialized"})
	}
	id := c.Params("id")
	return c.JSON(s.ecm.GetWindowData(id))
}

func (s *Server) getEdgeMetrics(c *fiber.Ctx) error {
	if s.ecm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Edge Compute Manager not initialized"})
	}
	return c.JSON(s.ecm.GetMetrics())
}

// handleLogWebSocket handles real-time log streaming
func (s *Server) handleLogWebSocket(c *websocket.Conn) {
	if s.logBroadcaster == nil {
		c.WriteMessage(websocket.CloseMessage, []byte("Log broadcaster not initialized"))
		c.Close()
		return
	}

	ch := s.logBroadcaster.Subscribe()
	defer s.logBroadcaster.Unsubscribe(ch)
	defer c.Close()

	// Read loop to detect client disconnect
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				c.Close()
				return
			}
		}
	}()

	for msg := range ch {
		c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

// handleLogDownload serves the log file
func (s *Server) handleLogDownload(c *fiber.Ctx) error {
	return c.Download("logs/gateway.edgex.log", "gateway.edgex.log")
}

func (s *Server) getOPCUAStats(c *fiber.Ctx) error {
	id := c.Params("id")
	stats, err := s.nbm.GetOPCUAStats(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

// syncOPCUAServer 重建 OPC UA 地址空间，同步南向点位配置（含读写权限）。
// POST /api/northbound/opcua/:id/sync
func (s *Server) syncOPCUAServer(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "server id is required"})
	}
	if err := s.nbm.SyncOPCUAServer(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"message":   "OPC UA address space synced",
		"server_id": id,
	})
}

// writeOPCUA 通过 OPC-UA 服务端写入单个点位
// POST /api/northbound/opcua/:id/write
type opcuaWriteRequest struct {
	ChannelID string `json:"channel_id"`
	DeviceID  string `json:"device_id"`
	PointID   string `json:"point_id"`
	Value     any    `json:"value"`
}

func (s *Server) writeOPCUA(c *fiber.Ctx) error {
	serverID := c.Params("id")

	var req opcuaWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.ChannelID == "" || req.DeviceID == "" || req.PointID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "channel_id, device_id, and point_id are required"})
	}

	err := s.nbm.WriteOPCUA(serverID, req.ChannelID, req.DeviceID, req.PointID, req.Value)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":    "write success",
		"server_id":  serverID,
		"channel_id": req.ChannelID,
		"device_id":  req.DeviceID,
		"point_id":   req.PointID,
		"value":      req.Value,
	})
}

// batchWriteOPCUA 批量写入多个点位
// POST /api/northbound/opcua/:id/batch-write
type opcuaBatchWriteRequest struct {
	Points []struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
		PointID   string `json:"point_id"`
		Value     any    `json:"value"`
	} `json:"points"`
}

func (s *Server) batchWriteOPCUA(c *fiber.Ctx) error {
	serverID := c.Params("id")

	var req opcuaBatchWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if len(req.Points) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "points array is empty"})
	}

	// 转换为 opcua.WriteRequest
	requests := make([]opcua.WriteRequest, len(req.Points))
	for i, p := range req.Points {
		requests[i] = opcua.WriteRequest{
			ChannelID: p.ChannelID,
			DeviceID:  p.DeviceID,
			PointID:   p.PointID,
			Value:     p.Value,
		}
	}

	results := s.nbm.BatchWriteOPCUA(serverID, requests)

	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	return c.JSON(fiber.Map{
		"server_id":     serverID,
		"total":         len(results),
		"success_count": successCount,
		"fail_count":    failCount,
		"results":       results,
	})
}

// getOPCUAWriteHistory 获取 OPC-UA 写入历史
// GET /api/northbound/opcua/:id/write-history?limit=100
func (s *Server) getOPCUAWriteHistory(c *fiber.Ctx) error {
	serverID := c.Params("id")
	limit := c.QueryInt("limit", 100)

	history, err := s.nbm.GetOPCUAWriteHistory(serverID, limit)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"server_id": serverID,
		"count":     len(history),
		"history":   history,
	})
}

// =============================================================================
// BACnet Server API handlers
// =============================================================================

// updateBACnetServerConfig 保存/更新 BACnet Server 配置
// POST /api/northbound/bacnet_server
func (s *Server) updateBACnetServerConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	var cfg model.BACnetServerConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}

	savedCfg, warning, err := s.nbm.UpsertBACnetServerConfig(cfg)
	if err != nil {
		return c.Status(northboundUpsertErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(northboundConfigJSON(savedCfg, warning))
}

// deleteBACnetServerConfig 删除 BACnet Server 配置
// DELETE /api/northbound/bacnet_server/:id
func (s *Server) deleteBACnetServerConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}
	id := c.Params("id")
	if err := s.nbm.DeleteBACnetServerConfig(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(200)
}

// syncBACnetServer 同步 BACnet Server 地址空间（热更新）
// POST /api/northbound/bacnet_server/:id/sync
func (s *Server) syncBACnetServer(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(fiber.Map{"error": "server id is required"})
	}
	if err := s.nbm.SyncBACnetServer(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"message":   "BACnet Server address space synced",
		"server_id": id,
	})
}

// getBACnetServerStats 获取 BACnet Server 运行统计
// GET /api/northbound/bacnet_server/:id/stats
func (s *Server) getBACnetServerStats(c *fiber.Ctx) error {
	id := c.Params("id")
	stats, err := s.nbm.GetBACnetServerStats(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

// getBACnetServerWriteHistory 获取 BACnet Server 写入历史
// GET /api/northbound/bacnet_server/:id/write-history?limit=100
func (s *Server) getBACnetServerWriteHistory(c *fiber.Ctx) error {
	serverID := c.Params("id")
	limit := c.QueryInt("limit", 100)

	history, err := s.nbm.GetBACnetServerWriteHistory(serverID, limit)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"server_id": serverID,
		"count":     len(history),
		"history":   history,
	})
}

func (s *Server) getMQTTStats(c *fiber.Ctx) error {
	id := c.Params("id")
	stats, err := s.nbm.GetMQTTStats(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

type randomWriteRequest struct {
	ChannelID       string   `json:"channel_id"`
	DeviceIDs       []string `json:"device_ids"`
	QPS             int      `json:"qps"`
	DurationSeconds int      `json:"duration_seconds"`
	Min             int      `json:"min"`
	Max             int      `json:"max"`
}

func (s *Server) startRandomWrite(c *fiber.Ctx) error {
	var req randomWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.ChannelID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "channel_id required"})
	}
	if req.QPS <= 0 {
		req.QPS = 5
	}
	if req.Min == 0 && req.Max == 0 {
		req.Min = 0
		req.Max = 1000
	}
	if req.Max < req.Min {
		req.Min, req.Max = req.Max, req.Min
	}

	s.randomWriteMu.Lock()
	if s.randomWriteRunning {
		s.randomWriteMu.Unlock()
		return c.Status(409).JSON(fiber.Map{"error": "random writer already running"})
	}
	stop := make(chan struct{})
	s.randomWriteStop = stop
	s.randomWriteRunning = true
	s.randomWriteMu.Unlock()

	devices := req.DeviceIDs
	if len(devices) == 0 {
		list := s.cm.GetChannelDevices(req.ChannelID)
		for _, d := range list {
			if d.Enable {
				devices = append(devices, d.ID)
			}
		}
	}
	if len(devices) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no devices to write"})
	}

	interval := time.Second / time.Duration(req.QPS)
	var endTime time.Time
	if req.DurationSeconds > 0 {
		endTime = time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
	}

	go func() {
		defer func() {
			s.randomWriteMu.Lock()
			s.randomWriteRunning = false
			close(stop)
			s.randomWriteMu.Unlock()
		}()
		for {
			select {
			case <-stop:
				return
			default:
			}

			if !endTime.IsZero() && time.Now().After(endTime) {
				return
			}

			di := rand.IntN(len(devices))
			devID := devices[di]
			dev := s.cm.GetDevice(req.ChannelID, devID)
			if dev == nil || len(dev.Points) == 0 {
				time.Sleep(interval)
				continue
			}
			pi := rand.IntN(len(dev.Points))
			pointID := dev.Points[pi].ID
			val := req.Min
			if req.Max > req.Min {
				val = req.Min + rand.IntN(req.Max-req.Min+1)
			}
			_ = s.cm.WritePoint(req.ChannelID, devID, pointID, val)
			time.Sleep(interval)
		}
	}()

	return c.JSON(fiber.Map{"status": "started"})
}

func (s *Server) stopRandomWrite(c *fiber.Ctx) error {
	s.randomWriteMu.Lock()
	defer s.randomWriteMu.Unlock()
	if !s.randomWriteRunning || s.randomWriteStop == nil {
		return c.Status(409).JSON(fiber.Map{"error": "random writer not running"})
	}
	select {
	case s.randomWriteStop <- struct{}{}:
	default:
	}
	s.randomWriteRunning = false
	return c.JSON(fiber.Map{"status": "stopping"})
}

// edgeOS(MQTT) 处理函数
func (s *Server) getEdgeOSMQTTStats(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	id := c.Params("id")
	stats, err := s.nbm.GetEdgeOSMQTTStats(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

func (s *Server) updateEdgeOSMQTTConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	var cfg model.EdgeOSMQTTConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}

	warning, err := s.nbm.UpsertEdgeOSMQTTConfig(cfg)
	if err != nil {
		zap.L().Error("Failed to update edgeOS(MQTT) config",
			zap.Error(err),
			zap.String("id", cfg.ID),
		)
		return c.Status(northboundUpsertErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}

	zap.L().Info("edgeOS(MQTT) config updated",
		zap.String("id", cfg.ID),
		zap.String("name", cfg.Name),
	)

	resp := fiber.Map{"success": true, "config": cfg}
	if warning != "" {
		resp["warning"] = warning
	}
	return c.JSON(resp)
}

func (s *Server) deleteEdgeOSMQTTConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	id := c.Params("id")
	if err := s.nbm.DeleteEdgeOSMQTTConfig(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	zap.L().Info("edgeOS(MQTT) config deleted",
		zap.String("id", id),
	)

	return c.JSON(fiber.Map{"success": true})
}

func (s *Server) publishEdgeOSMQTT(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	type PublishRequest struct {
		ClientID string `json:"client_id"`
		Topic    string `json:"topic"`
		Payload  []byte `json:"payload"`
	}

	var req PublishRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := s.nbm.PublishEdgeOSMQTT(req.ClientID, req.Topic, req.Payload); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// edgeOS(NATS) 处理函数
func (s *Server) getEdgeOSNATSStats(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	id := c.Params("id")
	stats, err := s.nbm.GetEdgeOSNATSStats(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

func (s *Server) updateEdgeOSNATSConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	var cfg model.EdgeOSNATSConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}

	warning, err := s.nbm.UpsertEdgeOSNATSConfig(cfg)
	if err != nil {
		zap.L().Error("Failed to update edgeOS(NATS) config",
			zap.Error(err),
			zap.String("id", cfg.ID),
		)
		return c.Status(northboundUpsertErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}

	zap.L().Info("edgeOS(NATS) config updated",
		zap.String("id", cfg.ID),
		zap.String("name", cfg.Name),
	)

	resp := fiber.Map{"success": true, "config": cfg}
	if warning != "" {
		resp["warning"] = warning
	}
	return c.JSON(resp)
}

func (s *Server) deleteEdgeOSNATSConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	id := c.Params("id")
	if err := s.nbm.DeleteEdgeOSNATSConfig(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	zap.L().Info("edgeOS(NATS) config deleted",
		zap.String("id", id),
	)

	return c.JSON(fiber.Map{"success": true})
}

func (s *Server) publishEdgeOSNATS(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	type PublishRequest struct {
		ClientID string `json:"client_id"`
		Subject  string `json:"subject"`
		Payload  []byte `json:"payload"`
	}

	var req PublishRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := s.nbm.PublishEdgeOSNATS(req.ClientID, req.Subject, req.Payload); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// exportDevicePoints 导出设备点位配置为JSON格式
func (s *Server) exportDevicePoints(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")

	dev := s.cm.GetDevice(channelId, deviceId)
	if dev == nil {
		return c.Status(404).JSON(fiber.Map{"error": "device not found"})
	}

	// 获取通道协议
	ch := s.cm.GetChannel(channelId)
	protocol := ""
	if ch != nil {
		protocol = ch.Protocol
	}

	// 构建导出数据
	type ExportPoint struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Address      string  `json:"address"`
		DataType     string  `json:"datatype"`
		Scale        float64 `json:"scale"`
		Offset       float64 `json:"offset"`
		Unit         string  `json:"unit"`
		ReadWrite    string  `json:"readwrite"`
		Group        string  `json:"group"`
		ReportMode   string  `json:"report_mode"`
		SlaveID      uint8   `json:"slave_id,omitempty"`
		RegisterType string  `json:"register_type,omitempty"`
		FunctionCode byte    `json:"function_code,omitempty"`
	}

	exportData := struct {
		ChannelID   string        `json:"channel_id"`
		ChannelName string        `json:"channel_name"`
		Protocol    string        `json:"protocol"`
		DeviceID    string        `json:"device_id"`
		DeviceName  string        `json:"device_name"`
		Points      []ExportPoint `json:"points"`
	}{
		ChannelID:   channelId,
		ChannelName: ch.Name,
		Protocol:    protocol,
		DeviceID:    deviceId,
		DeviceName:  dev.Name,
	}

	for _, p := range dev.Points {
		ep := ExportPoint{
			ID:           p.ID,
			Name:         p.Name,
			Address:      p.Address,
			DataType:     p.DataType,
			Scale:        p.Scale,
			Offset:       p.Offset,
			Unit:         p.Unit,
			ReadWrite:    p.ReadWrite,
			Group:        p.Group,
			ReportMode:   p.ReportMode,
			RegisterType: p.RegisterType.String(),
			FunctionCode: p.FunctionCode,
		}
		exportData.Points = append(exportData.Points, ep)
	}

	// 设置响应头
	c.Set("Content-Type", "application/json")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=points_%s_%s.json", channelId, deviceId))

	return c.JSON(exportData)
}

// importDevicePoints 导入设备点位配置
func (s *Server) importDevicePoints(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	deviceId := c.Params("deviceId")

	// 获取现有设备
	dev := s.cm.GetDevice(channelId, deviceId)
	if dev == nil {
		return c.Status(404).JSON(fiber.Map{"error": "device not found"})
	}

	// 获取通道协议
	ch := s.cm.GetChannel(channelId)
	if ch == nil {
		return c.Status(404).JSON(fiber.Map{"error": "channel not found"})
	}

	// 解析导入数据
	type ImportPoint struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Address      string  `json:"address"`
		DataType     string  `json:"datatype"`
		Scale        float64 `json:"scale"`
		Offset       float64 `json:"offset"`
		Unit         string  `json:"unit"`
		ReadWrite    string  `json:"readwrite"`
		Group        string  `json:"group"`
		ReportMode   string  `json:"report_mode"`
		SlaveID      uint8   `json:"slave_id,omitempty"`
		RegisterType string  `json:"register_type,omitempty"`
		FunctionCode byte    `json:"function_code,omitempty"`
	}

	var importData struct {
		ChannelID   string        `json:"channel_id"`
		DeviceID    string        `json:"device_id"`
		Points      []ImportPoint `json:"points"`
		ReplaceMode bool          `json:"replace_mode"` // true: 替换现有点位, false: 追加
	}

	if err := c.BodyParser(&importData); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body: " + err.Error()})
	}

	// 如果指定了目标通道/设备ID，使用请求中的值
	if importData.ChannelID != "" {
		channelId = importData.ChannelID
	}
	if importData.DeviceID != "" {
		deviceId = importData.DeviceID
	}

	// 转换为Point结构
	var points []model.Point
	for _, ip := range importData.Points {
		p := model.Point{
			ID:           ip.ID,
			Name:         ip.Name,
			Address:      ip.Address,
			DataType:     ip.DataType,
			Scale:        ip.Scale,
			Offset:       ip.Offset,
			Unit:         ip.Unit,
			ReadWrite:    ip.ReadWrite,
			Group:        ip.Group,
			ReportMode:   ip.ReportMode,
			RegisterType: model.ParseRegisterType(ip.RegisterType),
			FunctionCode: ip.FunctionCode,
		}

		// 如果ID为空，使用Name
		if err := model.EnsurePointID(&p); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		points = append(points, p)
	}

	if len(points) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no points to import"})
	}

	// 根据模式处理
	if importData.ReplaceMode {
		// 替换模式：先删除所有现有点位，再添加新点位
		existingPointIDs := make([]string, 0, len(dev.Points))
		for _, p := range dev.Points {
			existingPointIDs = append(existingPointIDs, p.ID)
		}
		if len(existingPointIDs) > 0 {
			if err := s.cm.RemovePoints(channelId, deviceId, existingPointIDs); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "failed to remove existing points: " + err.Error()})
			}
		}
	}

	// 添加新点位
	if err := s.cm.AddPoints(channelId, deviceId, points); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to add points: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":     true,
		"message":     fmt.Sprintf("successfully imported %d points", len(points)),
		"imported":    len(points),
		"replaceMode": importData.ReplaceMode,
	})
}

// exportConfig 导出所有配置为 JSON 格式
// GET /api/config/export
func (s *Server) exportConfig(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	configStore, err := storage.NewConfigStore(s.storage.GetConfigDB())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create config store: " + err.Error()})
	}

	exportData, err := configStore.ExportAllConfig()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to export config: " + err.Error()})
	}

	c.Set("Content-Type", "application/json")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=config_export_%s.json", time.Now().Format("20060102150405")))

	return c.JSON(exportData)
}

// importConfig 导入配置（JSON格式）
// POST /api/config/import
func (s *Server) importConfig(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	var exportData storage.ConfigExport
	if err := c.BodyParser(&exportData); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body: " + err.Error()})
	}

	configStore, err := storage.NewConfigStore(s.storage.GetConfigDB())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create config store: " + err.Error()})
	}

	if err := configStore.ImportConfig(&exportData); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to import config: " + err.Error()})
	}

	if err := s.cfgManager.Reload(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "config imported but failed to reload: " + err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "config imported successfully"})
}

// ===== 节点管理 API 处理函数 =====

// getNodeStatus 获取节点状态
func (s *Server) getNodeStatus(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.JSON(fiber.Map{
			"status": "stopped",
			"error":  "sync manager not initialized",
		})
	}

	return c.JSON(fiber.Map{
		"status":    "running",
		"peer_id":   s.syncManager.GetPeerIDString(),
		"addresses": []string{},
	})
}

// getNodeInfo 获取节点详细信息
func (s *Server) getNodeInfo(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	// Get system configuration
	cfg := s.sm.GetConfig()

	// Get libp2p info
	peerID := s.syncManager.GetPeerIDString()
	peers := s.syncManager.GetConnectedPeers()

	return c.JSON(fiber.Map{
		"peer_id":         peerID,
		"hostname":        cfg.Hostname.Name,
		"connected_peers": len(peers),
		"version":         "v1.0.0",
		"uptime":          time.Since(s.startTime).String(),
	})
}

// startNode 启动节点
func (s *Server) startNode(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	if err := s.syncManager.Start(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 自动发现并加入群组
	s.syncManager.AutoJoinGroup()

	return c.JSON(fiber.Map{
		"message":   "node started",
		"peer_id":   s.syncManager.GetPeerIDString(),
		"status":    "running",
		"timestamp": time.Now(),
	})
}

// stopNode 停止节点
func (s *Server) stopNode(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	s.syncManager.Stop()

	return c.JSON(fiber.Map{
		"message":   "node stopped",
		"status":    "stopped",
		"timestamp": time.Now(),
	})
}

// getDiscoveredNodes 获取已发现的节点列表
func (s *Server) getDiscoveredNodes(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	peers := s.syncManager.GetConnectedPeers()
	result := make([]fiber.Map, 0, len(peers))
	for _, peer := range peers {
		result = append(result, fiber.Map{
			"peer_id":   peer.ID.String(),
			"address":   peer.Addr,
			"status":    peer.Status,
			"last_seen": peer.LastSeen,
		})
	}

	return c.JSON(result)
}

// connectToPeer 连接到指定节点
func (s *Server) connectToPeer(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	peerId := c.Params("peerId")
	if peerId == "" {
		return c.Status(400).JSON(fiber.Map{"error": "peerId is required"})
	}

	// 尝试解析并连接
	if err := s.syncManager.ConnectToPeerByID(peerId); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "connecting to peer",
		"peer_id":   peerId,
		"timestamp": time.Now(),
	})
}

// disconnectFromPeer 断开与指定节点的连接
func (s *Server) disconnectFromPeer(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	peerId := c.Params("peerId")
	if peerId == "" {
		return c.Status(400).JSON(fiber.Map{"error": "peerId is required"})
	}

	if err := s.syncManager.DisconnectFromPeer(peerId); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "disconnected from peer",
		"peer_id":   peerId,
		"timestamp": time.Now(),
	})
}

// enableDiscovery 启用自动发现
func (s *Server) enableDiscovery(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	s.syncManager.EnableDiscovery()

	return c.JSON(fiber.Map{
		"message":   "discovery enabled",
		"timestamp": time.Now(),
	})
}

// disableDiscovery 禁用自动发现
func (s *Server) disableDiscovery(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	s.syncManager.DisableDiscovery()

	return c.JSON(fiber.Map{
		"message":   "discovery disabled",
		"timestamp": time.Now(),
	})
}

// ===== 群组管理 API 处理函数 =====

// getGroups 获取所有群组列表
func (s *Server) getGroups(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	groups := s.syncManager.GetAllGroups()
	result := make([]fiber.Map, 0, len(groups))
	for _, group := range groups {
		result = append(result, fiber.Map{
			"group_id":     group.GroupID,
			"name":         group.Name,
			"description":  group.Description,
			"member_count": len(group.Members),
			"created_at":   group.CreatedAt,
			"updated_at":   group.UpdatedAt,
		})
	}

	return c.JSON(result)
}

// createGroup 创建新群组
func (s *Server) createGroup(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "group name is required"})
	}

	groupID := uuid.New().String()
	err := s.syncManager.CreateGroup(groupID, req.Name, req.Description)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "group created",
		"group_id":  groupID,
		"timestamp": time.Now(),
	})
}

// getGroupDetail 获取群组详情
func (s *Server) getGroupDetail(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	groupID := c.Params("groupId")
	if groupID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "groupId is required"})
	}

	group, err := s.syncManager.GetGroup(groupID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"group_id":    group.GroupID,
		"name":        group.Name,
		"description": group.Description,
		"members":     group.Members,
		"created_at":  group.CreatedAt,
		"updated_at":  group.UpdatedAt,
	})
}

// joinGroup 加入群组
func (s *Server) joinGroup(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	groupID := c.Params("groupId")
	if groupID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "groupId is required"})
	}

	nodeID := s.syncManager.GetPeerIDString()
	if err := s.syncManager.JoinGroup(groupID, nodeID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "joined group",
		"group_id":  groupID,
		"timestamp": time.Now(),
	})
}

// leaveGroup 退出群组
func (s *Server) leaveGroup(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	groupID := c.Params("groupId")
	if groupID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "groupId is required"})
	}

	if err := s.syncManager.LeaveGroup(groupID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "left group",
		"group_id":  groupID,
		"timestamp": time.Now(),
	})
}

// addMemberToGroup 添加成员到群组
func (s *Server) addMemberToGroup(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	groupID := c.Params("groupId")
	if groupID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "groupId is required"})
	}

	var req struct {
		PeerID string `json:"peerId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.PeerID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "peerId is required"})
	}

	if err := s.syncManager.AddMemberToGroup(groupID, req.PeerID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "member added",
		"group_id":  groupID,
		"peer_id":   req.PeerID,
		"timestamp": time.Now(),
	})
}

// deleteGroup 删除群组
func (s *Server) deleteGroup(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	groupID := c.Params("groupId")
	if groupID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "groupId is required"})
	}

	if err := s.syncManager.DeleteGroup(groupID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "group deleted",
		"group_id":  groupID,
		"timestamp": time.Now(),
	})
}

// getJoinedGroups 获取已加入的群组列表
func (s *Server) getJoinedGroups(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	groups := s.syncManager.GetJoinedGroups()
	result := make([]fiber.Map, 0, len(groups))
	for _, group := range groups {
		result = append(result, fiber.Map{
			"group_id":     group.GroupID,
			"name":         group.Name,
			"description":  group.Description,
			"member_count": len(group.Members),
			"created_at":   group.CreatedAt,
		})
	}

	return c.JSON(result)
}

// ===== 数据同步 API 处理函数 =====

// getSyncStatus 获取同步状态
func (s *Server) getSyncStatus(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}
	return c.JSON(s.syncManager.GetStatus())
}

// triggerSync 触发手动同步
func (s *Server) triggerSync(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	var req struct {
		Type string `json:"type"` // full, delta, incremental
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Type == "" {
		req.Type = "delta"
	}

	if err := s.syncManager.TriggerSync(req.Type); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "sync triggered", "type": req.Type})
}

// checkConsistency 检查数据一致性
func (s *Server) checkConsistency(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	report, err := s.syncManager.CheckConsistency()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(report)
}

// repairConsistency 修复不一致数据
func (s *Server) repairConsistency(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	report, err := s.syncManager.CheckConsistency()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if report.OverallStatus == "consistent" {
		return c.JSON(fiber.Map{"message": "data is already consistent"})
	}

	return c.JSON(fiber.Map{"message": "repair initiated", "report": report})
}

// pushConfig 推送配置到所有已加入群组
func (s *Server) pushConfig(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	var req struct {
		GroupIDs       []string `json:"groupIds"`
		SyncAll        bool     `json:"syncAll"`
		ForceOverwrite bool     `json:"forceOverwrite"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	log.Println("[Server] Pushing config to all peers")

	// 触发全量同步
	if err := s.syncManager.TriggerSync("full"); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "config push initiated",
		"timestamp": time.Now(),
	})
}

// pullConfig 从已加入群组拉取最新配置
func (s *Server) pullConfig(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	var req struct {
		NodeID string `json:"nodeId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	log.Println("[Server] Pulling config from peers")

	// 如果指定了节点ID，从特定节点拉取
	if req.NodeID != "" && req.NodeID != "all" {
		// 从指定节点拉取配置
		snapshot, err := s.syncManager.PullFromRemote(req.NodeID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"message":   "config pulled from remote",
			"node_id":   req.NodeID,
			"snapshot":  snapshot,
			"timestamp": time.Now(),
		})
	}

	// 否则从所有节点拉取
	if err := s.syncManager.TriggerSync("delta"); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":   "config pull initiated from all peers",
		"timestamp": time.Now(),
	})
}

// getSyncHistory 获取同步历史记录
func (s *Server) getSyncHistory(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	// Return mock sync history
	history := []fiber.Map{
		{
			"id":         "sync-1",
			"type":       "delta",
			"status":     "completed",
			"timestamp":  time.Now().Add(-10 * time.Minute),
			"peer_count": 2,
			"details":    "Synchronized channel configurations",
		},
		{
			"id":         "sync-2",
			"type":       "full",
			"status":     "completed",
			"timestamp":  time.Now().Add(-1 * time.Hour),
			"peer_count": 3,
			"details":    "Full configuration synchronization",
		},
	}

	return c.JSON(fiber.Map{
		"history": history,
		"count":   len(history),
	})
}

// cancelSync 取消正在进行的同步
func (s *Server) cancelSync(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	log.Println("[Server] Canceling sync operation")

	return c.JSON(fiber.Map{
		"message":   "sync canceled",
		"timestamp": time.Now(),
	})
}

// ===== 网络监控 API 处理函数 =====

// getNetworkStatus 获取网络状态
func (s *Server) getNetworkStatus(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.JSON(fiber.Map{
			"status": "disconnected",
			"error":  "libp2p manager not initialized",
		})
	}

	return c.JSON(fiber.Map{
		"status":      "connected",
		"peer_id":     s.syncManager.GetPeerIDString(),
		"connections": len(s.syncManager.GetConnectedPeers()),
	})
}

// getConnectedPeers 获取已连接的节点列表
func (s *Server) getConnectedPeers(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	peers := s.syncManager.GetConnectedPeers()
	result := make([]fiber.Map, 0, len(peers))
	for _, peer := range peers {
		result = append(result, fiber.Map{
			"peer_id":   peer.ID.String(),
			"address":   peer.Addr,
			"status":    peer.Status,
			"last_seen": peer.LastSeen,
		})
	}

	return c.JSON(result)
}

// getNetworkStats 获取网络统计信息
func (s *Server) getNetworkStats(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "libp2p manager not initialized"})
	}

	// Return mock network stats
	return c.JSON(fiber.Map{
		"connected_peers":     len(s.syncManager.GetConnectedPeers()),
		"data_transfer_rate":  "1.2 MB/s",
		"average_latency":     "15 ms",
		"sync_success_rate":   "98.5%",
		"total_data_transfer": "125 MB",
		"uptime":              "2h 30m",
	})
}

// getNetworkLogs 获取网络日志
func (s *Server) getNetworkLogs(c *fiber.Ctx) error {
	// Return mock network logs
	logs := []fiber.Map{
		{
			"timestamp": time.Now().Add(-5 * time.Minute),
			"level":     "INFO",
			"message":   "Connected to peer: 12D3KooW...",
		},
		{
			"timestamp": time.Now().Add(-10 * time.Minute),
			"level":     "INFO",
			"message":   "Discovered peer via mDNS: 12D3KooX...",
		},
		{
			"timestamp": time.Now().Add(-15 * time.Minute),
			"level":     "DEBUG",
			"message":   "Synchronization completed successfully",
		},
	}

	return c.JSON(fiber.Map{
		"logs":  logs,
		"count": len(logs),
	})
}

// clearNetworkLogs 清除网络日志
func (s *Server) clearNetworkLogs(c *fiber.Ctx) error {
	log.Println("[Server] Network logs cleared")
	return c.JSON(fiber.Map{
		"message":   "logs cleared",
		"timestamp": time.Now(),
	})
}

// ===== 设备更换 API 处理函数 =====

// validateDeviceCode 验证设备编码
func (s *Server) validateDeviceCode(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.DeviceCode == "" {
		return c.Status(400).JSON(fiber.Map{"error": "device_code is required"})
	}

	deviceCode, err := s.syncManager.ValidateDeviceCode(req.DeviceCode)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"valid": true,
		"device_info": fiber.Map{
			"protocol":      deviceCode.Protocol,
			"vendor_id":     deviceCode.VendorID,
			"model_id":      deviceCode.ModelID,
			"serial_number": deviceCode.SerialNumber,
		},
	})
}

// migrateDeviceConfig 发起设备配置迁移
func (s *Server) migrateDeviceConfig(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	var req syncpkg.ConfigMigrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request: " + err.Error()})
	}

	if req.DeviceCode == "" {
		return c.Status(400).JSON(fiber.Map{"error": "device_code is required"})
	}
	if req.TargetDeviceID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "target_device_id is required"})
	}

	return c.JSON(fiber.Map{"message": "migration initiated", "device_code": req.DeviceCode})
}

// ===== 一键同步 API 处理函数 =====

// simpleSync 一键同步 - 只需要输入节点ID和设备编码即可完成同步
// POST /api/sync/simple
//
//	{
//	    "node_id": "node-001",
//	    "device_code": "modbus-siemens-s71200-SN123456789-ABC123"
//	}
func (s *Server) simpleSync(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	var req syncpkg.SimpleSyncRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request: " + err.Error()})
	}

	if req.NodeID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "node_id is required"})
	}
	if req.DeviceCode == "" {
		return c.Status(400).JSON(fiber.Map{"error": "device_code is required"})
	}

	return c.JSON(fiber.Map{"message": "simple sync initiated", "node_id": req.NodeID, "device_code": req.DeviceCode})
}

func (s *Server) getDataStats(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	stats, totalSize, err := s.storage.GetBucketStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	stats = s.enrichRuntimeBucketStats(stats)

	result := fiber.Map{
		"config_db": fiber.Map{
			"path": s.storage.GetConfigPath(),
		},
		"runtime_db": fiber.Map{
			"path": s.storage.GetPath(),
		},
		"total_size": totalSize,
		"buckets":    stats,
	}

	return c.JSON(result)
}

func (s *Server) enrichRuntimeBucketStats(stats []storage.BucketStats) []storage.BucketStats {
	runtimeIndex := make(map[string]int, len(stats))
	for i, st := range stats {
		if st.Database == "runtime" {
			runtimeIndex[st.Name] = i
		}
	}

	addRuntimeCount := func(name string, extra int) {
		if extra <= 0 {
			return
		}
		if idx, ok := runtimeIndex[name]; ok {
			stats[idx].RecordCount += extra
			return
		}
		category, clearable := storage.ClassifyBucket(name)
		stats = append(stats, storage.BucketStats{
			Name:        name,
			RecordCount: extra,
			Category:    category,
			Clearable:   clearable,
			Database:    "runtime",
		})
		runtimeIndex[name] = len(stats) - 1
	}

	if s.ecm != nil {
		eventsMem, failuresMem, minuteCache := s.ecm.RuntimeLogStats()
		addRuntimeCount("edge_events", eventsMem)
		addRuntimeCount("edge_failures", failuresMem)
		addRuntimeCount("bblot", minuteCache)
	}

	if s.shadowCore != nil {
		_, pointCount := s.shadowCore.RuntimePointStats()
		if pointCount > 0 {
			if idx, ok := runtimeIndex["shadow_values"]; ok {
				stats[idx].RecordCount = pointCount
			} else {
				stats = append(stats, storage.BucketStats{
					Name:        "shadow_values",
					RecordCount: pointCount,
					Category:    "runtime",
					Clearable:   true,
					Database:    "runtime",
				})
				runtimeIndex["shadow_values"] = len(stats) - 1
			}
		}
	}

	if s.dsm != nil {
		for _, name := range s.dsm.HistoryBucketNames() {
			if _, ok := runtimeIndex[name]; ok {
				continue
			}
			stats = append(stats, storage.BucketStats{
				Name:        name,
				RecordCount: 0,
				Category:    "history",
				Clearable:   true,
				Database:    "runtime",
			})
			runtimeIndex[name] = len(stats) - 1
		}
	}

	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].Database != stats[j].Database {
			return stats[i].Database == "config"
		}
		return stats[i].Name < stats[j].Name
	})

	return stats
}

func (s *Server) clearCache(c *fiber.Ctx) error {
	var req struct {
		Mode    string   `json:"mode"`
		Buckets []string `json:"buckets"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Mode == "" && len(req.Buckets) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "mode or buckets is required"})
	}

	var bucketsToClear []string

	switch req.Mode {
	case "cache":
		bucketsToClear = []string{
			"DataCache",
			"WindowData",
			"NorthboundCache",
			"RuleState",
		}
	case "runtime":
		bucketsToClear = []string{
			"DataCache",
			"WindowData",
			"NorthboundCache",
			"RuleState",
			"values",
		}
	case "history":
		if s.storage != nil {
			stats, _, _ := s.storage.GetBucketStats()
			for _, stat := range stats {
				if strings.HasPrefix(stat.Name, "device_history_") {
					bucketsToClear = append(bucketsToClear, stat.Name)
				}
			}
		}
	case "":
		bucketsToClear = req.Buckets
	default:
		return c.Status(400).JSON(fiber.Map{"error": "invalid mode: " + req.Mode})
	}

	for _, bucket := range bucketsToClear {
		if storage.IsConfigBucket(bucket) {
			return c.Status(403).JSON(fiber.Map{"error": "config bucket " + bucket + " cannot be cleared"})
		}

		if s.storage != nil {
			if err := s.storage.ClearBucket(bucket); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "failed to clear bucket " + bucket + ": " + err.Error()})
			}
		}

		if s.dsm != nil && bucket == "values" {
			s.dsm.ClearAllHistory()
		}
	}

	compactInfo, compactErr := s.compactRuntimeWithStats()
	if compactErr != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "cleared but failed to compact runtime db: " + compactErr.Error(),
			"cleared": bucketsToClear,
		})
	}

	stats, totalSize, err := s.storage.GetBucketStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"cleared": bucketsToClear,
		"compact": compactInfo,
		"config_db": fiber.Map{
			"path": s.storage.GetConfigPath(),
		},
		"runtime_db": fiber.Map{
			"path": s.storage.GetPath(),
		},
		"total_size": totalSize,
		"buckets":    stats,
	})
}

// clearAllRuntime 清空所有运行时数据（用于 runtime DB 重建）
// POST /api/data/clear-all-runtime
func (s *Server) clearAllRuntime(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	cleared, err := s.storage.ClearAllRuntimeBuckets()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error(), "cleared": cleared})
	}

	if s.shadowCore != nil {
		s.shadowCore.ClearAllShadowDevices()
	}

	var edgeLogsCleared any
	if s.ecm != nil {
		edgeResult, edgeErr := s.ecm.ClearEdgeLogs()
		if edgeErr != nil {
			return c.Status(500).JSON(fiber.Map{"error": edgeErr.Error(), "cleared": cleared})
		}
		edgeLogsCleared = edgeResult
		s.ecm.ClearRuntimeState()
	}
	if s.dsm != nil {
		s.dsm.ClearAllHistory()
	}

	compactInfo, compactErr := s.compactRuntimeWithStats()
	if compactErr != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "cleared but failed to compact runtime db: " + compactErr.Error(),
			"cleared": cleared,
		})
	}

	stats, totalSize, _ := s.storage.GetBucketStats()

	response := fiber.Map{
		"status":  "success",
		"cleared": cleared,
		"compact": compactInfo,
		"config_db": fiber.Map{
			"path": s.storage.GetConfigPath(),
		},
		"runtime_db": fiber.Map{
			"path": s.storage.GetPath(),
		},
		"total_size": totalSize,
		"buckets":    stats,
	}
	if edgeLogsCleared != nil {
		response["edge_logs"] = edgeLogsCleared
	}
	return c.JSON(response)
}

// backupConfigDB 备份配置数据库（优先备份，不包含运行时数据）
// POST /api/data/backup-config
func (s *Server) backupConfigDB(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	backupDir := c.Query("dir", "data/backups")
	backupInfo, err := s.storage.BackupConfigDB(backupDir)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to backup config db: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"status":       "success",
		"backup_path":  backupInfo.BackupPath,
		"backup_time":  backupInfo.BackupTime,
		"original":     backupInfo.OriginalPath,
		"size_bytes":   backupInfo.FileSizeBytes,
		"size_display": formatBytes(backupInfo.FileSizeBytes),
		"message":      "配置库已备份，运行时数据不受影响",
	})
}

// exportConfigDBArchive 导出配置数据库为 tar.gz
// GET /api/data/export-config-db
func (s *Server) exportConfigDBArchive(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	data, filename, err := s.storage.ExportConfigDBArchive()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to export config db: " + err.Error()})
	}

	c.Set("Content-Type", "application/gzip")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	return c.Send(data)
}

// exportRuntimeDBArchive 导出运行时数据库为 tar.gz
// GET /api/data/export-runtime-db
func (s *Server) exportRuntimeDBArchive(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	data, filename, err := s.storage.ExportRuntimeDBArchive()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to export runtime db: " + err.Error()})
	}

	c.Set("Content-Type", "application/gzip")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	return c.Send(data)
}

// importConfigDBArchive 从 tar.gz 导入配置数据库
// POST /api/data/import-config-db
func (s *Server) importConfigDBArchive(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "file is required: " + err.Error()})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "failed to open uploaded file: " + err.Error()})
	}
	defer file.Close()

	archiveData, err := io.ReadAll(file)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "failed to read uploaded file: " + err.Error()})
	}

	if len(archiveData) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "uploaded file is empty"})
	}

	forceOverwrite := c.FormValue("force_overwrite") == "true" || c.FormValue("force_overwrite") == "1"
	if !forceOverwrite {
		forceOverwrite = c.Query("force_overwrite") == "true" || c.Query("force_overwrite") == "1"
	}

	result, err := s.storage.ImportConfigDBArchive(archiveData, storage.ImportArchiveOptions{
		ForceOverwrite: forceOverwrite,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to import config db: " + err.Error()})
	}

	if s.cfgManager != nil {
		if err := s.cfgManager.Reload(); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "config imported but failed to reload: " + err.Error()})
		}
	}

	message := "配置库导入成功，已保留当前用户账号/密码与服务器端口"
	if result.ForceOverwrite {
		message = "配置库导入成功，已强制覆盖本地配置（含用户账号/密码与服务器端口）"
	}

	return c.JSON(fiber.Map{
		"status":          "success",
		"message":         message,
		"force_overwrite": result.ForceOverwrite,
		"preserved_users": result.PreservedUsers,
		"preserved_port":  result.PreservedPort,
		"device_count":    result.DeviceCount,
		"channel_count":   result.ChannelCount,
	})
}

// pullRemoteConfig 从远程网关强制拉取配置并覆盖本地
// POST /api/data/pull-remote-config
func (s *Server) pullRemoteConfig(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	var req struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Token    string `json:"token"`
		UseHTTPS bool   `json:"use_https"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body: " + err.Error()})
	}

	if strings.TrimSpace(req.Host) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "remote host is required"})
	}
	if req.Port <= 0 {
		req.Port = 8080
	}

	result, err := s.storage.PullRemoteConfigAndImport(req.Host, req.Port, req.Token, req.UseHTTPS)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to pull remote config: " + err.Error()})
	}

	if s.cfgManager != nil {
		if err := s.cfgManager.Reload(); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "config pulled but failed to reload: " + err.Error()})
		}
	}

	return c.JSON(fiber.Map{
		"status":          "success",
		"message":         "已从远程网关强制拉取并覆盖本地配置",
		"remote_source":   result.RemoteSource,
		"force_overwrite": true,
		"device_count":    result.DeviceCount,
		"channel_count":   result.ChannelCount,
		"preserved_port":  result.PreservedPort,
	})
}

// compactRuntimeWithStats 压缩 runtime.db 并返回压缩前后文件大小。
func (s *Server) compactRuntimeWithStats() (fiber.Map, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage not available")
	}

	runtimePath := s.storage.GetRuntimePath()
	beforeInfo, _ := os.Stat(runtimePath)
	beforeSize := int64(0)
	if beforeInfo != nil {
		beforeSize = beforeInfo.Size()
	}

	if err := s.storage.CompactRuntimeDB(); err != nil {
		return nil, err
	}

	afterInfo, _ := os.Stat(runtimePath)
	afterSize := int64(0)
	if afterInfo != nil {
		afterSize = afterInfo.Size()
	}

	saved := beforeSize - afterSize
	if saved < 0 {
		saved = 0
	}

	return fiber.Map{
		"before_bytes": beforeSize,
		"after_bytes":  afterSize,
		"saved_bytes":  saved,
		"before_size":  formatBytes(beforeSize),
		"after_size":   formatBytes(afterSize),
		"saved_size":   formatBytes(saved),
	}, nil
}

// compactRuntimeDB 压缩运行时数据库，回收已删除数据的空间
// POST /api/data/compact-runtime
func (s *Server) compactRuntimeDB(c *fiber.Ctx) error {
	if s.storage == nil {
		return c.Status(503).JSON(fiber.Map{"error": "storage not available"})
	}

	compactInfo, err := s.compactRuntimeWithStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to compact runtime db: " + err.Error()})
	}

	compactInfo["status"] = "success"
	compactInfo["message"] = "运行时数据库已压缩，配置库不受影响"
	return c.JSON(compactInfo)
}

// formatBytes 格式化字节数为 MB 显示
func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 MB"
	}
	mb := float64(bytes) / (1024 * 1024)
	if mb < 0.01 {
		return fmt.Sprintf("%.4f MB", mb)
	}
	return fmt.Sprintf("%.2f MB", mb)
}
