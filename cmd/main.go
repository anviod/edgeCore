package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/core"
	_ "github.com/anviod/edgeCore/internal/driver/bacnet"
	_ "github.com/anviod/edgeCore/internal/driver/dlt645"
	_ "github.com/anviod/edgeCore/internal/driver/ethercat"
	_ "github.com/anviod/edgeCore/internal/driver/ethernetip"
	_ "github.com/anviod/edgeCore/internal/driver/ice104"
	_ "github.com/anviod/edgeCore/internal/driver/knxnetip"
	_ "github.com/anviod/edgeCore/internal/driver/mitsubishi"
	_ "github.com/anviod/edgeCore/internal/driver/modbus"
	_ "github.com/anviod/edgeCore/internal/driver/omron"
	_ "github.com/anviod/edgeCore/internal/driver/opcua"
	_ "github.com/anviod/edgeCore/internal/driver/profinetio"
	_ "github.com/anviod/edgeCore/internal/driver/s7"
	_ "github.com/anviod/edgeCore/internal/driver/snmp"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/pkg/logger"
	"github.com/anviod/edgeCore/internal/server"
	"github.com/anviod/edgeCore/internal/storage"

	"go.uber.org/zap"
)

func startPprof() func() {
	addr := os.Getenv("PPROF_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6060"
	}
	if addr == "off" || addr == "0" {
		return func() {}
	}

	srv := &http.Server{Addr: addr}
	go func() {
		zap.L().Info("pprof server starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Warn("pprof server failed", zap.String("addr", addr), zap.Error(err))
		}
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

func main() {
	// Parse command-line flags

	// Create LogBroadcaster
	logBroadcaster := logger.NewLogBroadcaster()

	// Init Logger (Console only for startup)
	logger.InitLogger("info", "", nil)
	zap.L().Info("Starting Industrial Edge Gateway...")

	// 1. 初始化流程：检测任一后端的配置库是否存在且为空
	// （bbolt: config.db / sqlite: config.sqlite，后端选择记录在 data/storage.meta）
	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		zap.L().Fatal("Failed to create data directory", zap.String("dir", dataDir), zap.Error(err))
	}

	backend := storage.ReadBackendChoice(dataDir)
	configExists := storage.IsBackendInstalled(dataDir)

	zap.L().Info("Database check",
		zap.String("backend", backend),
		zap.Bool("config_exists", configExists))

	var store *storage.Storage
	var cfg *config.Config
	var cfgManager *config.ConfigManager
	var runtimeReady bool
	var err error

	if configExists {
		zap.L().Info("Initializing storage...", zap.String("dir", dataDir), zap.String("backend", backend))
		store, err = storage.NewStorageWithBackend(dataDir, backend)
		if err != nil {
			zap.L().Fatal("Failed to init storage", zap.Error(err))
		}
		zap.L().Info("Storage initialized successfully",
			zap.String("config_db", store.GetConfigPath()),
			zap.String("runtime_db", store.GetPath()))

		// config.db 存在但无业务配置时进入安装模式，仅启动 Web 服务
		configStore, _ := storage.NewConfigStore(store.GetConfigDB())
		runtimeReady, _ = configStore.HasConfigData()
		if runtimeReady {
			zap.L().Info("Configuration data found in config.db")
		} else {
			zap.L().Info("config.db is empty, entering install mode (web UI only)")
		}

		zap.L().Info("Loading configuration from database...")
		cfgManager, err = config.NewConfigManagerWithDB(store.GetConfigDB())
		if err != nil {
			zap.L().Fatal("Failed to load config with DB", zap.Error(err))
		}
		cfg = cfgManager.GetConfig()
		zap.L().Info("Configuration loaded successfully from database")
	} else {
		zap.L().Info("config.db not found, starting with default empty config (install mode)")
		cfgManager = config.NewConfigManagerWithEmptyConfig()
		cfg = cfgManager.GetConfig()
		runtimeReady = false
		zap.L().Info("Default config loaded for installation")
	}

	// Re-init Logger with config and broadcaster
	if err := os.MkdirAll("logs", 0755); err != nil {
		zap.L().Warn("Failed to create logs directory", zap.Error(err))
	}
	logger.InitLogger(cfg.Server.LogLevel, "logs/gateway.edgeCore.log", logBroadcaster)
	zap.L().Info("Logger initialized", zap.String("level", cfg.Server.LogLevel), zap.String("file", "logs/gateway.edgeCore.log"))
	zap.L().Info("Build info",
		zap.String("version", model.Version),
		zap.String("build_time", model.FormatBuildTime()),
		zap.String("commit_id", model.CommitID),
	)

	zap.L().Info("Data directory ready", zap.String("path", dataDir))

	var shadowCore *core.ShadowCore
	var shadowIngress *core.ShadowIngress
	var virtualShadow *core.VirtualShadowEngine
	var vsm *core.VirtualShadowManager
	if store != nil {
		defer store.Close()
	}

	// 3. Init Core Components
	zap.L().Info("Initializing data pipeline...")
	pipeline := core.NewDataPipeline(100)
	zap.L().Info("Data pipeline initialized")

	storeForward := core.NewStoreForwardManager(store, core.StoreForwardPolicy{
		MaxSouthboundRecords: 10000,
		MaxNorthboundPerID:   1000,
	})
	pipeline.AddBatchHandler(storeForward.HandleBatch)

	// Init Edge Compute Manager
	zap.L().Info("Initializing Edge Compute Manager...")
	ecm := core.NewEdgeComputeManager(pipeline, store, func(rules []model.EdgeRule) error {
		current := cfgManager.GetConfig()
		current.EdgeRules = rules
		return cfgManager.SaveConfig(current)
	})
	ecm.LoadRules(cfg.EdgeRules)
	ecm.Start()
	zap.L().Info("Edge Compute Manager started")

	// 4. Init Channel Manager (Before Northbound)
	zap.L().Info("Initializing Channel Manager...")
	cm := core.NewChannelManager(pipeline, func(channels []model.Channel) error {
		current := cfgManager.GetConfig()
		current.Channels = channels
		return cfgManager.SaveConfig(current)
	})

	dsm := core.NewDeviceStorageManager(store, pipeline)

	wireShadowStack := func(sc *core.ShadowCore) {
		core.NewShadowBridge(pipeline).Attach(sc)
		dsm.SetShadowCore(sc)
		virtualShadow = core.NewVirtualShadowEngine(sc)
		shadowIngress = core.NewShadowIngress(sc, 256, 8*time.Millisecond)
		shadowIngress.Start()
	}

	if store != nil {
		shadowCore = core.NewShadowCore()
		shadowCore.Start()
		wireShadowStack(shadowCore)
		cm.SetShadowIngress(shadowIngress)
		defer func() {
			if shadowIngress != nil {
				shadowIngress.Stop()
			}
			shadowCore.Stop()
		}()
		zap.L().Info("ShadowCore, ShadowIngress and VirtualShadowEngine initialized")
	}

	// 5. Init Northbound Manager
	zap.L().Info("Initializing Northbound Manager...")
	nbm := core.NewNorthboundManager(cfg.Northbound, pipeline, cm, store, func(nbCfg model.NorthboundConfig) error {
		current := cfgManager.GetConfig()
		current.Northbound = nbCfg
		return cfgManager.SaveConfig(current)
	})
	nbm.SetChannelManager(cm)
	if shadowCore != nil {
		nbm.BindShadowCore(shadowCore)
	}
	cm.SetStatusHandler(func(deviceID string, status int) {
		nbm.OnDeviceStatusChange(deviceID, status)
	})
	cm.SetTopologyChangeHandler(func() {
		nbm.RebuildOPCUAServers()
		if vsm != nil {
			vsm.ReloadAll()
		}
	})

	// Connect Edge Compute to Northbound
	ecm.SetNorthboundManager(nbm)

	// Init System Manager
	sm := core.NewSystemManager(cfg)
	sm.SetConfigManager(cfgManager)

	// P0 数据丢失防护（2026-10-13 审查）：导入/恢复后 Reload 只替换 cfgManager.Config，
	// 各持有旧引用/旧快照的组件必须重新挂载，否则下一次全量保存会把
	// 刚导入的配置整体回滚覆盖。导入后的 DB 状态是权威状态。
	cfgManager.OnReload(func(newCfg *config.Config) {
		sm.SetConfig(newCfg)
		nbm.UpdateConfig(newCfg.Northbound)
		ecm.ReloadRules(newCfg.EdgeRules)
		if vsm != nil {
			if configs, err := cfgManager.LoadVirtualShadows(); err == nil {
				vsm.Load(configs)
			} else {
				zap.L().Warn("Failed to reload virtual shadows after config import", zap.Error(err))
			}
		}
	})

	for _, ch := range cfg.Channels {
		for _, dev := range ch.Devices {
			dsm.UpdateDeviceConfig(dev.ID, dev.Storage)
		}
	}
	zap.L().Info("Core components initialized")

	// 4. Init Web Server
	zap.L().Info("Initializing Web Server...")
	srv := server.NewServer(cm, store, pipeline, nbm, ecm, sm, dsm, cfgManager, logBroadcaster)
	if shadowCore != nil {
		srv.SetShadowCore(shadowCore)
	}
	if virtualShadow != nil {
		srv.SetVirtualShadowEngine(virtualShadow)
	}
	if virtualShadow != nil && shadowCore != nil && cfgManager != nil {
		vsm = core.NewVirtualShadowManager(virtualShadow, cm, shadowCore, func(devices []model.VirtualShadowDeviceConfig) error {
			return cfgManager.SaveVirtualShadows(devices)
		})
		if configs, err := cfgManager.LoadVirtualShadows(); err == nil {
			vsm.Load(configs)
		} else {
			zap.L().Warn("Failed to load virtual shadows", zap.Error(err))
			// P1-5 防护：加载失败后 persist 会用"只含新条目"的列表整体覆盖
			// DB 单 key blob，原有全部影子配置将被静默清空。置位守卫，
			// 拒绝覆盖直到 Load 成功。
			vsm.MarkLoadFailed()
		}
		srv.SetVirtualShadowManager(vsm)
		nbm.SetVirtualShadowManager(vsm)
	}
	srv.SetStorageAttachHook(func(st *storage.Storage) {
		if ecm != nil {
			ecm.SetStorage(st)
		}
		if dsm != nil {
			dsm.SetStorage(st)
		}
		storeForward.SetStorage(st)
		if shadowCore == nil {
			shadowCore = core.NewShadowCore()
			shadowCore.Start()
			wireShadowStack(shadowCore)
			cm.SetShadowIngress(shadowIngress)
			srv.SetShadowCore(shadowCore)
			nbm.BindShadowCore(shadowCore)
			srv.SetVirtualShadowEngine(virtualShadow)
			if vsm == nil && cfgManager != nil {
				vsm = core.NewVirtualShadowManager(virtualShadow, cm, shadowCore, func(devices []model.VirtualShadowDeviceConfig) error {
					return cfgManager.SaveVirtualShadows(devices)
				})
				srv.SetVirtualShadowManager(vsm)
				nbm.SetVirtualShadowManager(vsm)
			}
			zap.L().Info("ShadowCore initialized after install")
		}
	})
	zap.L().Info("Web Server initialized")

	startDataPlane := func() {
		go func() {
			current := cfgManager.GetConfig()
			var wg sync.WaitGroup
			for _, chConfig := range current.Channels {
				ch := chConfig

				err := cm.AddChannel(&ch)
				if err != nil {
					zap.L().Error("Failed to add channel", zap.String("channel", ch.Name), zap.Error(err))
					// 启动加载失败的通道进入隔离区：保留在持久化视图中，
					// 防止后续任意一次全量保存把它从 DB 静默删除（P0 数据丢失防护）。
					cm.QuarantineChannel(&ch, err.Error())
					continue
				}

				// 并行启动每个通道，避免 Modbus 60s cooldown 阻塞 BACnet
				wg.Add(1)
				go func(ch model.Channel) {
					defer wg.Done()
					if err := cm.StartChannel(ch.ID); err != nil {
						zap.L().Error("Failed to start channel", zap.String("channel", ch.Name), zap.Error(err))
					}
				}(ch)
			}
			wg.Wait()
			zap.L().Info("All channels initialization completed")

			nbm.Start()
			nbm.RebuildOPCUAServers()
			zap.L().Info("Northbound manager started")
		}()
	}

	var dataPlaneOnce sync.Once
	startDataPlaneOnce := func() {
		dataPlaneOnce.Do(startDataPlane)
	}
	srv.SetRuntimeStartHook(startDataPlaneOnce)

	// P0 数据丢失防护（2026-10-13 审查）：Web 重启端点改走与 SIGTERM
	// 完全相同的优雅关停路径（pipeline 最终 drain → MQTT/影子 flush → store.Close），
	// 不再 os.Exit(0) 硬退出。
	restartChan := make(chan struct{}, 1)
	srv.SetRestartSignal(restartChan)

	pipeline.Start()
	stopPprof := startPprof()
	defer stopPprof()

	// 5. Start Web Server first (before any potentially blocking operations)
	go func() {
		port := 8080
		if cfg.Server.Port != 0 {
			port = cfg.Server.Port
		}
		addr := ":" + strconv.Itoa(port)
		zap.L().Info("Web server starting", zap.String("addr", addr))
		if err := srv.Start(addr); err != nil {
			zap.L().Fatal("Web server failed", zap.Error(err))
		}
	}()

	// 6. Start channels and northbound only after install completes (config persisted in DB)
	if runtimeReady {
		startDataPlaneOnce()
		defer nbm.Stop()
	} else {
		zap.L().Info("Skipping channel and northbound startup (install mode)")
	}

	// 7. Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigChan:
	case <-restartChan:
		zap.L().Info("Restart requested via web API, performing graceful shutdown")
	}

	zap.L().Info("Shutting down...")

	// 有序关闭：HTTP → EdgeCompute → Channels → Pipeline，
	// 之后再由 defer 依次关闭 Northbound、Shadow、Store。
	srv.StopBackgroundTasks()
	srv.Shutdown()
	ecm.Stop()
	cm.Shutdown()
	pipeline.Stop()
}
