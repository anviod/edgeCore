package server

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

var (
	installProgress    = 0
	installCurrentStep = 0
	installTotalSteps  = 5
	installStatus      = "idle"
	installLogMessages = []string{}
	installStatusMutex = sync.Mutex{}
	isInstalling       = false
	installMutex       = sync.Mutex{}
	installConfigPort  = 0
)

func (s *Server) checkInstallStatus(c *fiber.Ctx) error {
	installStatusMutex.Lock()
	defer installStatusMutex.Unlock()

	hasData, err := s.checkIfInstalled()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "500",
			"message": "检查安装状态失败",
			"data":    nil,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"isInstalled": hasData,
			"status":      installStatus,
		},
		"error": nil,
	})
}

func (s *Server) checkPort(c *fiber.Ctx) error {
	port := c.QueryInt("port", -1)

	if port == -1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "400",
			"message": "参数错误",
			"data":    nil,
			"error":   "端口号不能为空",
		})
	}

	if port < 80 || port > 65535 {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data": fiber.Map{
				"available": false,
				"port":      port,
				"error":     "端口号必须在80-65535范围内",
			},
			"error": nil,
		})
	}

	// 安装模式下 Web 服务已在监听；保留当前端口视为可用
	if port == s.GetListenPort() {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data": fiber.Map{
				"available": true,
				"port":      port,
				"error":     "",
			},
			"error": nil,
		})
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data": fiber.Map{
				"available": false,
				"port":      port,
				"error":     "端口已被占用",
			},
			"error": nil,
		})
	}
	ln.Close()

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"available": true,
			"port":      port,
			"error":     "",
		},
		"error": nil,
	})
}

func (s *Server) checkPath(c *fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "400",
			"message": "参数错误",
			"data":    nil,
			"error":   "无效的路径",
		})
	}

	if req.Path == "" {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data": fiber.Map{
				"accessible": false,
				"path":       req.Path,
				"error":      "路径不能为空",
			},
			"error": nil,
		})
	}

	pathInfo, err := os.Stat(req.Path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(req.Path, 0755); err != nil {
				return c.JSON(fiber.Map{
					"code":    "0",
					"message": "success",
					"data": fiber.Map{
						"accessible": false,
						"path":       req.Path,
						"error":      "无法创建目录: " + err.Error(),
					},
					"error": nil,
				})
			}
			pathInfo, _ = os.Stat(req.Path)
		} else {
			return c.JSON(fiber.Map{
				"code":    "0",
				"message": "success",
				"data": fiber.Map{
					"accessible": false,
					"path":       req.Path,
					"error":      "访问路径失败: " + err.Error(),
				},
				"error": nil,
			})
		}
	}

	if !pathInfo.IsDir() {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data": fiber.Map{
				"accessible": false,
				"path":       req.Path,
				"error":      "指定的路径不是目录",
			},
			"error": nil,
		})
	}

	testFile := filepath.Join(req.Path, ".test_write_access")
	file, err := os.Create(testFile)
	if err != nil {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data": fiber.Map{
				"accessible": false,
				"path":       req.Path,
				"error":      "目录不可写: " + err.Error(),
			},
			"error": nil,
		})
	}
	file.Close()
	os.Remove(testFile)

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"accessible": true,
			"path":       req.Path,
			"error":      "",
		},
		"error": nil,
	})
}

func (s *Server) validateConfig(c *fiber.Ctx) error {
	var config model.InstallConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "400",
			"message": "参数错误",
			"data":    nil,
			"error":   "无效的配置数据",
		})
	}

	errors := []string{}

	if config.Port < 80 || config.Port > 65535 {
		errors = append(errors, "端口号必须在80-65535范围内")
	}

	if config.Username == "" {
		errors = append(errors, "用户名不能为空")
	}

	if len(config.Password) < 8 {
		errors = append(errors, "密码长度至少8位")
	}

	upperCount := 0
	lowerCount := 0
	numCount := 0
	specialCount := 0
	for _, ch := range config.Password {
		if ch >= 'A' && ch <= 'Z' {
			upperCount++
		} else if ch >= 'a' && ch <= 'z' {
			lowerCount++
		} else if ch >= '0' && ch <= '9' {
			numCount++
		} else {
			specialCount++
		}
	}

	typesCount := 0
	if upperCount > 0 {
		typesCount++
	}
	if lowerCount > 0 {
		typesCount++
	}
	if numCount > 0 {
		typesCount++
	}
	if specialCount > 0 {
		typesCount++
	}

	if typesCount < 3 {
		errors = append(errors, "密码需包含大写字母、小写字母、数字、特殊符号中的至少三种组合")
	}

	if config.StoragePath == "" {
		errors = append(errors, "数据存储路径不能为空")
	}

	if config.GatewayName == "" {
		errors = append(errors, "网关名称不能为空")
	}

	if len(errors) > 0 {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "验证失败",
			"data": fiber.Map{
				"valid":  false,
				"errors": errors,
			},
			"error": nil,
		})
	}

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"valid":  true,
			"errors": []string{},
		},
		"error": nil,
	})
}

func (s *Server) startInstall(c *fiber.Ctx) error {
	installMutex.Lock()
	if isInstalling {
		installMutex.Unlock()
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"code":    "409",
			"message": "正在执行初始化",
			"data":    nil,
			"error":   "系统正在初始化中，请稍后再试",
		})
	}
	isInstalling = true
	installMutex.Unlock()

	var cfg model.InstallConfig
	if err := c.BodyParser(&cfg); err != nil {
		isInstalling = false
		s.logger.Error("Failed to parse install config", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "400",
			"message": "参数错误",
			"data":    nil,
			"error":   "无效的配置数据: " + err.Error(),
		})
	}

	go s.executeInstall(&cfg)

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "初始化流程已启动",
		"data": fiber.Map{
			"status": "running",
		},
		"error": nil,
	})
}

func (s *Server) getInstallStatus(c *fiber.Ctx) error {
	installStatusMutex.Lock()
	defer installStatusMutex.Unlock()

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"currentStep": installCurrentStep,
			"totalSteps":  installTotalSteps,
			"progress":    installProgress,
			"status":      installStatus,
			"logMessages": installLogMessages,
			"configPort":  installConfigPort,
		},
		"error": nil,
	})
}

func (s *Server) executeInstall(cfg *model.InstallConfig) {
	defer func() {
		isInstalling = false
	}()

	installConfigPort = cfg.Port

	totalSteps := 4

	installStatusMutex.Lock()
	installProgress = 0
	installCurrentStep = 0
	installTotalSteps = totalSteps
	installLogMessages = []string{}
	installStatus = "running"
	installStatusMutex.Unlock()

	addLog("开始初始化安装流程...")

	step(1, "验证配置参数")
	if err := validateInstallConfig(cfg); err != nil {
		addLog(fmt.Sprintf("配置验证失败: %v", err))
		setStatus("failed", err.Error())
		return
	}
	addLog("配置参数验证通过")

	step(2, "初始化数据库")
	if err := s.initStorage(); err != nil {
		addLog(fmt.Sprintf("初始化数据库失败: %v", err))
		setStatus("failed", err.Error())
		return
	}
	addLog("数据库初始化成功")

	step(3, "保存系统配置")
	if err := s.saveSystemConfigToDB(cfg); err != nil {
		addLog(fmt.Sprintf("保存系统配置失败: %v", err))
		setStatus("failed", err.Error())
		return
	}
	addLog("系统配置保存成功")

	step(4, "创建管理员账户")
	if err := s.createAdminUserInDB(cfg.Username, cfg.Password); err != nil {
		addLog(fmt.Sprintf("创建管理员账户失败: %v", err))
		setStatus("failed", err.Error())
		return
	}
	addLog("管理员账户创建成功")

	installStatusMutex.Lock()
	installProgress = 100
	installStatus = "completed"
	installStatusMutex.Unlock()
	addLog("初始化安装流程完成")

	if s.runtimeStartHook != nil {
		s.runtimeStartHook()
		addLog("数据采集与北向服务已启动")
	}

	if cfg.Port != 0 && s.GetListenPort() != 0 && cfg.Port != s.GetListenPort() {
		addLog(fmt.Sprintf("正在切换到新端口: %d", cfg.Port))
		go func() {
			time.Sleep(2 * time.Second)
			if err := s.SwitchPort(cfg.Port); err != nil {
				addLog(fmt.Sprintf("端口切换失败: %v", err))
			} else {
				addLog(fmt.Sprintf("已切换到端口 %d", cfg.Port))
			}
		}()
	}
}

func (s *Server) initStorage() error {
	if s.storage != nil {
		return nil
	}

	dataDir := "data"
	store, err := storage.NewStorage(dataDir)
	if err != nil {
		return fmt.Errorf("创建数据库失败: %w", err)
	}

	s.storage = store
	s.cfgManager.AttachDB(store.GetConfigDB())

	if s.storageAttachHook != nil {
		s.storageAttachHook(store)
	}

	return nil
}

func (s *Server) saveSystemConfigToDB(cfg *model.InstallConfig) error {
	if s.storage == nil {
		return fmt.Errorf("存储未初始化")
	}

	db := s.storage.GetConfigDB()
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	currentCfg := s.cfgManager.GetConfig()
	currentCfg.Server.Port = cfg.Port
	currentCfg.System.Hostname.Name = cfg.GatewayName
	currentCfg.System.Hostname.HTTPPort = cfg.Port
	currentCfg.System.Hostname.EnableMDNS = true
	currentCfg.System.Hostname.EnableBare = true
	currentCfg.System.Location = cfg.GatewayLocation

	return config.SaveConfigToDB(db, currentCfg)
}

func (s *Server) createAdminUserInDB(username, password string) error {
	if s.storage == nil {
		return fmt.Errorf("存储未初始化")
	}

	db := s.storage.GetConfigDB()
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	configStore, err := storage.NewConfigStore(db)
	if err != nil {
		return fmt.Errorf("创建配置存储失败: %w", err)
	}

	users := []model.UserConfig{
		{
			Username: username,
			Password: password,
			Role:     "admin",
		},
	}

	if err := configStore.SaveUsers(users); err != nil {
		return err
	}

	s.cfgManager.GetConfig().Users = users
	return nil
}

func step(stepNum int, stepName string) {
	installStatusMutex.Lock()
	installCurrentStep = stepNum
	installProgress = (stepNum - 1) * (100 / installTotalSteps)
	installStatus = fmt.Sprintf("正在执行: %s", stepName)
	installStatusMutex.Unlock()
	addLog(fmt.Sprintf("[步骤 %d/%d] %s", stepNum, installTotalSteps, stepName))
	time.Sleep(500 * time.Millisecond)
}

func addLog(message string) {
	installStatusMutex.Lock()
	defer installStatusMutex.Unlock()
	installLogMessages = append(installLogMessages, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), message))
	if len(installLogMessages) > 100 {
		installLogMessages = installLogMessages[len(installLogMessages)-100:]
	}
}

func setStatus(status, errorMsg string) {
	installStatusMutex.Lock()
	defer installStatusMutex.Unlock()
	installStatus = status
	if errorMsg != "" {
		installLogMessages = append(installLogMessages, fmt.Sprintf("[ERROR] %s", errorMsg))
	}
}

func validateInstallConfig(cfg *model.InstallConfig) error {
	if cfg.Port < 80 || cfg.Port > 65535 {
		return fmt.Errorf("端口号必须在80-65535范围内")
	}
	if cfg.Username == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if len(cfg.Password) < 8 {
		return fmt.Errorf("密码长度至少8位")
	}
	if cfg.StoragePath == "" {
		return fmt.Errorf("数据存储路径不能为空")
	}
	if cfg.GatewayName == "" {
		return fmt.Errorf("网关名称不能为空")
	}
	return nil
}

func (s *Server) checkIfInstalled() (bool, error) {
	if s.storage == nil {
		return false, nil
	}

	db := s.storage.GetConfigDB()
	if db == nil {
		return false, nil
	}

	configStore, err := storage.NewConfigStore(db)
	if err != nil {
		return false, err
	}

	isInitialized, err := configStore.IsSystemInitialized()
	if err != nil {
		return false, err
	}

	if !isInitialized {
		return false, nil
	}

	return true, nil
}
