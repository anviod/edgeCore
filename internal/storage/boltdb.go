package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"go.uber.org/zap"
)

type Storage struct {
	configDB  KVDB
	runtimeDB KVDB
	backend   string
	// runtimeMu 保护 runtimeDB 的 compact 换绑窗口（bbolt 后端关库→换文件→重开）。
	// 读/写路径持 RLock 执行事务，compact 持 Lock 完成换绑：换绑窗口内的
	// 写入会短暂阻塞而不是拿到 "database not open" 被上层吞掉
	// （P0 数据丢失防护，2026-10-13 审查）。
	runtimeMu sync.RWMutex
	// compactMu 串行化 compact（定时循环与手动 API 可能并发触发）。
	compactMu sync.Mutex
	dataDir   string
}

// runtimeUpdate / runtimeView：RLock 保护下的 runtime.db 事务。
func (s *Storage) runtimeUpdate(fn func(tx KVTx) error) error {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtimeDB.Update(fn)
}

func (s *Storage) runtimeView(fn func(tx KVTx) error) error {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtimeDB.View(fn)
}

func (s *Storage) GetPath() string {
	return s.GetRuntimePath()
}

// GetRuntimePath returns the on-disk path of runtime.db.
func (s *Storage) GetRuntimePath() string {
	return s.runtimeDB.Path()
}

func (s *Storage) GetConfigPath() string {
	return s.configDB.Path()
}

// GetDB 暴露运行时库句柄（统一 KVDB 接口）。
func (s *Storage) GetDB() KVDB {
	return s.runtimeDB
}

// GetConfigDB 暴露配置库句柄（统一 KVDB 接口）。
func (s *Storage) GetConfigDB() KVDB {
	return s.configDB
}

// Backend 返回存储后端标识（"bbolt" 或 "sqlite"）。
func (s *Storage) Backend() string {
	return s.backend
}

func (s *Storage) getDBByBucket(bucketName string) KVDB {
	if IsConfigBucket(bucketName) {
		return s.configDB
	}
	return s.runtimeDB
}

func (s *Storage) SaveOfflineMessage(configID string, data []byte, maxCount int) error {
	return s.runtimeUpdate(func(tx KVTx) error {
		b := tx.Cursor(BucketNorthboundCache)

		key := fmt.Sprintf("%s_%d", configID, time.Now().UnixNano())

		if err := tx.Put(BucketNorthboundCache, []byte(key), data); err != nil {
			return err
		}

		prefix := []byte(configID + "_")
		count := 0
		var keysToDelete [][]byte

		for k, _ := b.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = b.Next() {
			count++
		}

		if count > maxCount {
			toDelete := count - maxCount
			for k, _ := b.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = b.Next() {
				if toDelete <= 0 {
					break
				}
				keysToDelete = append(keysToDelete, append([]byte{}, k...))
				toDelete--
			}
		}

		for _, k := range keysToDelete {
			if err := tx.Delete(BucketNorthboundCache, k); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Storage) GetOfflineMessages(configID string, limit int) ([]OfflineMessage, error) {
	var messages []OfflineMessage
	err := s.runtimeView(func(tx KVTx) error {
		c := tx.Cursor(BucketNorthboundCache)
		prefix := []byte(configID + "_")

		for k, v := c.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			if len(messages) >= limit {
				break
			}
			dataCopy := make([]byte, len(v))
			copy(dataCopy, v)

			messages = append(messages, OfflineMessage{
				Key:  string(k),
				Data: dataCopy,
			})
		}
		return nil
	})
	return messages, err
}

func (s *Storage) RemoveOfflineMessage(key string) error {
	return s.runtimeUpdate(func(tx KVTx) error {
		return tx.Delete(BucketNorthboundCache, []byte(key))
	})
}

const (
	BucketValues          = "values"
	BucketRuleState       = "RuleState"
	BucketDataCache       = "DataCache"
	BucketWindow          = "WindowData"
	BucketNorthboundCache = "NorthboundCache"

	// legacyShadowWALBucket is a removed ShadowCore WAL bucket; dropped on startup.
	legacyShadowWALBucket = "shadow_wal"
)

type OfflineMessage struct {
	Key  string
	Data []byte
}

var configBucketNames = []string{
	BucketConfigVersion,
	BucketChannels,
	BucketDevices,
	BucketNorthbound,
	BucketEdgeRules,
	BucketSystem,
	BucketUsers,
	BucketServer,
	BucketVirtualShadows,
}

var runtimeBucketNames = []string{
	BucketValues,
	BucketRuleState,
	BucketDataCache,
	BucketWindow,
	BucketNorthboundCache,
}

var edgeLogBucketNames = []string{
	"edge_events",
	"edge_failures",
	"bblot",
}

var knownRuntimeBucketNames = append(append([]string(nil), runtimeBucketNames...), edgeLogBucketNames...)

// configDBFilename / runtimeDBFilename 按后端返回数据库文件名。
func configDBFilename(backend string) string {
	if backend == BackendSQLite {
		return "config.sqlite"
	}
	return "config.db"
}

func runtimeDBFilename(backend string) string {
	if backend == BackendSQLite {
		return "runtime.sqlite"
	}
	return "runtime.db"
}

// NewStorage 以默认后端（bbolt）打开存储（保持既有调用方/测试兼容）。
func NewStorage(dataDir string) (*Storage, error) {
	return NewStorageWithBackend(dataDir, BackendBbolt)
}

// NewStorageWithBackend 以指定后端打开存储。backend ∈ {"bbolt","sqlite"}。
// 选择结果写入 data/storage.meta，供后续启动恢复同一后端。
func NewStorageWithBackend(dataDir, backend string) (*Storage, error) {
	if dataDir == "" {
		dataDir = "data"
	}
	if backend != BackendBbolt && backend != BackendSQLite {
		return nil, fmt.Errorf("unknown storage backend %q (supported: bbolt, sqlite)", backend)
	}

	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	configPath := filepath.Join(dataDir, configDBFilename(backend))
	runtimePath := filepath.Join(dataDir, runtimeDBFilename(backend))

	// 打开配置数据库（强一致写入）
	configDB, err := openKV(configPath, false, backend)
	if err != nil {
		return nil, fmt.Errorf("failed to open config database %s: %w", configPath, err)
	}

	// 初始化配置 bucket
	err = configDB.Update(func(tx KVTx) error {
		for _, bucket := range configBucketNames {
			if err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		currentVersion := tx.Get(BucketConfigVersion, []byte(ConfigVersionKey))
		if currentVersion == nil {
			return tx.Put(BucketConfigVersion, []byte(ConfigVersionKey), []byte(ConfigVersionValue))
		}
		return nil
	})
	if err != nil {
		configDB.Close()
		return nil, fmt.Errorf("failed to init config buckets: %w", err)
	}

	// 打开运行时数据库（允许清理、compact、重建）
	// P0 自愈（2026-10-13 审查，仅 bbolt 后端）：CompactRuntimeDB 的两步 rename
	// 之间若进程崩溃/断电，runtime.db 会缺失但 .pre-compact.bak 完整存在——
	// 此时必须恢复，否则网关 Fatal 无限重启（数据明明还在磁盘上）。
	// sqlite 后端 Compact 为原地 VACUUM，无此风险。
	if backend == BackendBbolt {
		if _, statErr := os.Stat(runtimePath); os.IsNotExist(statErr) {
			backupPath := runtimePath + ".pre-compact.bak"
			if _, bakErr := os.Stat(backupPath); bakErr == nil {
				if renameErr := os.Rename(backupPath, runtimePath); renameErr == nil {
					zap.L().Warn("runtime.db missing after interrupted compact, restored from .pre-compact.bak",
						zap.String("path", runtimePath))
				}
			}
		}
	}
	runtimeDB, err := openKV(runtimePath, true, backend)
	if err != nil {
		configDB.Close()
		return nil, fmt.Errorf("failed to open runtime database %s: %w", runtimePath, err)
	}

	// 初始化运行时 bucket
	err = runtimeDB.Update(func(tx KVTx) error {
		for _, bucket := range runtimeBucketNames {
			if err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		runtimeDB.Close()
		configDB.Close()
		return nil, fmt.Errorf("failed to init runtime buckets: %w", err)
	}

	storage := &Storage{
		configDB:  configDB,
		runtimeDB: runtimeDB,
		backend:   backend,
		dataDir:   dataDir,
	}
	if err := storage.dropLegacyShadowWALBucket(); err != nil {
		runtimeDB.Close()
		configDB.Close()
		return nil, fmt.Errorf("failed to drop legacy shadow WAL bucket: %w", err)
	}

	if err := WriteBackendChoice(dataDir, backend); err != nil {
		zap.L().Warn("failed to persist storage backend choice", zap.Error(err))
	}

	return storage, nil
}

// openKV 按后端打开数据库。
func openKV(path string, allowCleanup bool, backend string) (KVDB, error) {
	if backend == BackendSQLite {
		return openSQLiteKV(path)
	}
	return openBoltKV(path, allowCleanup)
}

// WriteBackendChoice 把后端选择写入 dataDir/storage.meta（启动时读取恢复）。
func WriteBackendChoice(dataDir, backend string) error {
	if dataDir == "" {
		dataDir = "data"
	}
	return os.WriteFile(filepath.Join(dataDir, BackendMetaFile), []byte(backend+"\n"), 0644)
}

// ReadBackendChoice 读取已保存的后端选择；无记录时返回默认 bbolt。
func ReadBackendChoice(dataDir string) string {
	if dataDir == "" {
		dataDir = "data"
	}
	// #nosec G304 -- dataDir 为服务端内部数据目录，BackendMetaFile 为固定文件名常量
	raw, err := os.ReadFile(filepath.Join(dataDir, BackendMetaFile))
	if err != nil {
		return BackendBbolt
	}
	backend := strings.TrimSpace(string(raw))
	if backend != BackendBbolt && backend != BackendSQLite {
		return BackendBbolt
	}
	return backend
}

// IsBackendInstalled 判断 dataDir 下是否已存在某个后端的配置库
// （安装模式检测：首次安装时两者都不存在）。
func IsBackendInstalled(dataDir string) bool {
	if dataDir == "" {
		dataDir = "data"
	}
	for _, backend := range []string{BackendBbolt, BackendSQLite} {
		if _, err := os.Stat(filepath.Join(dataDir, configDBFilename(backend))); err == nil {
			return true
		}
	}
	return false
}

func (s *Storage) Close() error {
	err1 := s.configDB.Close()
	s.runtimeMu.Lock()
	err2 := s.runtimeDB.Close()
	s.runtimeMu.Unlock()
	if err1 != nil {
		return err1
	}
	return err2
}

func (s *Storage) SaveData(bucketName string, key string, data interface{}) error {
	put := func(tx KVTx) error {
		if err := tx.CreateBucketIfNotExists(bucketName); err != nil {
			return err
		}
		bytes, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return tx.Put(bucketName, []byte(key), bytes)
	}
	if IsConfigBucket(bucketName) {
		return s.configDB.Update(put)
	}
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtimeDB.Update(put)
}

func (s *Storage) GetData(bucketName string, key string, result interface{}) error {
	db := s.getDBByBucket(bucketName)
	if !IsConfigBucket(bucketName) {
		s.runtimeMu.RLock()
		defer s.runtimeMu.RUnlock()
	}
	return db.View(func(tx KVTx) error {
		data := tx.Get(bucketName, []byte(key))
		if data == nil {
			return fmt.Errorf("key %s not found in bucket %s", key, bucketName)
		}
		return json.Unmarshal(data, result)
	})
}

func (s *Storage) DeleteData(bucketName string, key string) error {
	db := s.getDBByBucket(bucketName)
	if !IsConfigBucket(bucketName) {
		s.runtimeMu.RLock()
		defer s.runtimeMu.RUnlock()
	}
	return db.Update(func(tx KVTx) error {
		return tx.Delete(bucketName, []byte(key))
	})
}

func (s *Storage) PruneOldest(bucketName string, maxRecords int) error {
	db := s.getDBByBucket(bucketName)
	if !IsConfigBucket(bucketName) {
		s.runtimeMu.RLock()
		defer s.runtimeMu.RUnlock()
	}
	return db.Update(func(tx KVTx) error {
		count, err := tx.CountKeys(bucketName)
		if err != nil {
			return err
		}
		if count <= maxRecords {
			return nil
		}

		deleteCount := count - maxRecords
		c := tx.Cursor(bucketName)
		for i := 0; i < deleteCount; i++ {
			k, _ := c.First()
			if k == nil {
				break
			}
			if err := tx.Delete(bucketName, k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Storage) LoadLatest(bucketName string, limit int, callback func(k, v []byte) error) error {
	db := s.getDBByBucket(bucketName)
	if !IsConfigBucket(bucketName) {
		s.runtimeMu.RLock()
		defer s.runtimeMu.RUnlock()
	}
	return db.View(func(tx KVTx) error {
		c := tx.Cursor(bucketName)
		count := 0
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			if limit > 0 && count >= limit {
				break
			}
			if err := callback(k, v); err != nil {
				return err
			}
			count++
		}
		return nil
	})
}

func (s *Storage) LoadAll(bucketName string, callback func(k, v []byte) error) error {
	db := s.getDBByBucket(bucketName)
	if !IsConfigBucket(bucketName) {
		s.runtimeMu.RLock()
		defer s.runtimeMu.RUnlock()
	}
	return db.View(func(tx KVTx) error {
		return tx.ForEach(bucketName, callback)
	})
}

func (s *Storage) LoadRange(bucketName string, minKey, maxKey string, callback func(k, v []byte) error) error {
	db := s.getDBByBucket(bucketName)
	if !IsConfigBucket(bucketName) {
		s.runtimeMu.RLock()
		defer s.runtimeMu.RUnlock()
	}
	return db.View(func(tx KVTx) error {
		c := tx.Cursor(bucketName)
		for k, v := c.Seek([]byte(minKey)); k != nil && string(k) <= maxKey; k, v = c.Next() {
			if err := callback(k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Storage) SaveValue(val model.Value) error {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()

	return s.runtimeDB.Update(func(tx KVTx) error {
		data, err := json.Marshal(val)
		if err != nil {
			return err
		}
		return tx.Put(BucketValues, []byte(val.PointID), data)
	})
}

func (s *Storage) GetLastValue(pointID string) (*model.Value, error) {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()

	var val model.Value
	err := s.runtimeDB.View(func(tx KVTx) error {
		data := tx.Get(BucketValues, []byte(pointID))
		if data == nil {
			return fmt.Errorf("not found")
		}
		return json.Unmarshal(data, &val)
	})
	return &val, err
}

func (s *Storage) GetAllValues() (map[string]model.Value, error) {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()

	result := make(map[string]model.Value)
	err := s.runtimeDB.View(func(tx KVTx) error {
		return tx.ForEach(BucketValues, func(k, v []byte) error {
			var val model.Value
			if err := json.Unmarshal(v, &val); err == nil {
				result[string(k)] = val
			}
			return nil
		})
	})
	return result, err
}

type BucketStats struct {
	Name        string `json:"name"`
	RecordCount int    `json:"record_count"`
	TotalSize   int64  `json:"total_size"`
	Category    string `json:"category"`
	Clearable   bool   `json:"clearable"`
	Database    string `json:"database"`
}

var (
	configBucketMap = map[string]bool{
		"ConfigVersion":  true,
		"Channels":       true,
		"Devices":        true,
		"Northbound":     true,
		"EdgeRules":      true,
		"System":         true,
		"Users":          true,
		"Server":         true,
		"VirtualShadows": true,
		"ai_copilot":     true,
	}

	cacheBucketMap = map[string]bool{
		"DataCache":       true,
		"WindowData":      true,
		"NorthboundCache": true,
		"RuleState":       true,
	}

	runtimeBucketMap = map[string]bool{
		"values":        true,
		"shadow_values": true,
	}

	edgeLogBucketMap = map[string]bool{
		"edge_events":   true,
		"edge_failures": true,
		"bblot":         true,
	}
)

func ClassifyBucket(name string) (category string, clearable bool) {
	return classifyBucket(name)
}

func classifyBucket(name string) (category string, clearable bool) {
	if configBucketMap[name] {
		return "config", false
	}
	if cacheBucketMap[name] {
		return "cache", true
	}
	if runtimeBucketMap[name] {
		return "runtime", true
	}
	if edgeLogBucketMap[name] {
		return "edge_log", true
	}
	if strings.HasPrefix(name, "device_history_") {
		return "history", true
	}
	if name == legacyShadowWALBucket {
		return "legacy", true
	}
	if name == "WAL" {
		return "cache", true
	}
	return "unknown", false
}

// dropLegacyShadowWALBucket removes the obsolete ShadowCore WAL bucket.
// Shadow devices are memory-only; leftover data is reclaimed after CompactRuntimeDB.
func (s *Storage) dropLegacyShadowWALBucket() error {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtimeDB.Update(func(tx KVTx) error {
		return tx.DeleteBucket(legacyShadowWALBucket)
	})
}

func IsConfigBucket(name string) bool {
	return configBucketMap[name]
}

// dbOnDiskSize 统计主文件与（sqlite 后端的）WAL/SHM 附件占用。
func dbOnDiskSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	total := info.Size()
	for _, suffix := range []string{"-wal", "-shm"} {
		if extra, err := os.Stat(path + suffix); err == nil {
			total += extra.Size()
		}
	}
	return total, nil
}

// DBDiskSize 供外部（HTTP 层）统计数据库实际磁盘占用，含 sqlite 的 WAL/SHM 附件。
func DBDiskSize(path string) (int64, error) {
	return dbOnDiskSize(path)
}

func (s *Storage) GetBucketStats() ([]BucketStats, int64, error) {
	var stats []BucketStats
	var totalSize int64

	configSize, err := dbOnDiskSize(s.configDB.Path())
	if err != nil {
		return nil, 0, err
	}
	totalSize += configSize

	runtimeSize, err := dbOnDiskSize(s.runtimeDB.Path())
	if err != nil {
		return nil, 0, err
	}
	totalSize += runtimeSize

	err = s.configDB.View(func(tx KVTx) error {
		names, err := tx.Buckets()
		if err != nil {
			return err
		}
		for _, name := range names {
			count, size := bucketCountAndSize(tx, name)
			category, clearable := classifyBucket(name)
			stats = append(stats, BucketStats{
				Name:        name,
				RecordCount: count,
				TotalSize:   size,
				Category:    category,
				Clearable:   clearable,
				Database:    "config",
			})
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	err = s.runtimeView(func(tx KVTx) error {
		names, err := tx.Buckets()
		if err != nil {
			return err
		}
		for _, name := range names {
			count, size := bucketCountAndSize(tx, name)
			category, clearable := classifyBucket(name)
			stats = append(stats, BucketStats{
				Name:        name,
				RecordCount: count,
				TotalSize:   size,
				Category:    category,
				Clearable:   clearable,
				Database:    "runtime",
			})
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	stats = ensureKnownRuntimeBuckets(stats)
	sortBucketStats(stats)

	return stats, totalSize, nil
}

func bucketCountAndSize(tx KVTx, name string) (int, int64) {
	count := 0
	size := int64(0)
	_ = tx.ForEach(name, func(k, v []byte) error {
		count++
		size += int64(len(k) + len(v))
		return nil
	})
	return count, size
}

func ensureKnownRuntimeBuckets(stats []BucketStats) []BucketStats {
	existing := make(map[string]struct{}, len(stats))
	for _, st := range stats {
		if st.Database == "runtime" {
			existing[st.Name] = struct{}{}
		}
	}
	for _, name := range knownRuntimeBucketNames {
		if _, ok := existing[name]; ok {
			continue
		}
		category, clearable := classifyBucket(name)
		stats = append(stats, BucketStats{
			Name:        name,
			RecordCount: 0,
			TotalSize:   0,
			Category:    category,
			Clearable:   clearable,
			Database:    "runtime",
		})
	}
	return stats
}

func sortBucketStats(stats []BucketStats) {
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].Database != stats[j].Database {
			return stats[i].Database == "config"
		}
		return stats[i].Name < stats[j].Name
	})
}

func (s *Storage) ClearBucket(bucketName string) error {
	if IsConfigBucket(bucketName) {
		return fmt.Errorf("config bucket %s cannot be cleared", bucketName)
	}
	return s.runtimeUpdate(func(tx KVTx) error {
		return clearBucketInTx(tx, bucketName)
	})
}

func clearBucketInTx(tx KVTx, bucketName string) error {
	if !tx.HasBucket(bucketName) {
		return fmt.Errorf("bucket %s not found", bucketName)
	}
	c := tx.Cursor(bucketName)
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		if err := c.Delete(); err != nil {
			return err
		}
	}
	return nil
}

// ClearAllRuntimeBuckets 清空 runtime.db 中的全部 bucket（含 values、缓存、历史、遗留 WAL 等）。
// 配置 bucket 不应出现在 runtime.db；若检测到则报错。
func (s *Storage) ClearAllRuntimeBuckets() ([]string, error) {
	var cleared []string

	err := s.runtimeUpdate(func(tx KVTx) error {
		bucketNames, err := tx.Buckets()
		if err != nil {
			return err
		}

		for _, bucketName := range bucketNames {
			if IsConfigBucket(bucketName) {
				return fmt.Errorf("config bucket %s found in runtime db", bucketName)
			}

			if bucketName == legacyShadowWALBucket {
				if err := tx.DeleteBucket(bucketName); err != nil {
					return err
				}
				cleared = append(cleared, bucketName)
				continue
			}

			if err := clearBucketInTx(tx, bucketName); err != nil {
				return err
			}
			cleared = append(cleared, bucketName)
		}
		return nil
	})

	return cleared, err
}

// CompactRuntimeDB 回收运行时数据库空间。
//   - bbolt：整文件重写 + 毫秒级换绑（换绑窗口写入在 runtimeMu 排队），
//     两步 rename 间崩溃由 NewStorage 的 .pre-compact.bak 自愈兜底；
//   - sqlite：原地 VACUUM（SQLite 内建保证原子性，无换绑窗口）。
//
// 两后端 Compact 期间数据面写入均不中断服务语义（短暂排队）。
func (s *Storage) CompactRuntimeDB() error {
	s.compactMu.Lock()
	defer s.compactMu.Unlock()

	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	if err := s.runtimeDB.Compact(); err != nil {
		return err
	}

	if err := s.runtimeDB.Update(func(tx KVTx) error {
		for _, bucket := range runtimeBucketNames {
			if err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to reinit runtime buckets after compact: %w", err)
	}

	return nil
}

// BackupConfigDB 备份配置数据库到指定目录。
// 配置库优先备份，不包含运行时数据。
func (s *Storage) BackupConfigDB(backupDir string) (*model.BackupInfo, error) {
	return s.backupKVToDir(s.configDB, backupDir)
}

// BackupRuntimeDB 备份运行时数据库到指定目录（可选诊断用途）。
func (s *Storage) BackupRuntimeDB(backupDir string) (*model.BackupInfo, error) {
	runtimeBackupDir := filepath.Join(backupDir, "runtime")
	return s.backupKVToDir(s.runtimeDB, runtimeBackupDir)
}

func (s *Storage) backupKVToDir(kv KVDB, backupDir string) (*model.BackupInfo, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	backupTime := time.Now()
	ext := ".db"
	if s.backend == BackendSQLite {
		ext = ".sqlite"
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("config-backup-%s%s", backupTime.Format("20060102-150405"), ext))

	if err := kv.BackupTo(backupPath); err != nil {
		return nil, err
	}

	fileInfo, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup file info: %w", err)
	}

	return &model.BackupInfo{
		BackupPath:    backupPath,
		BackupTime:    backupTime,
		OriginalPath:  kv.Path(),
		FileSizeBytes: fileInfo.Size(),
		Version:       "1.0",
	}, nil
}

// SyncConfigDB 对配置数据库执行强制同步，确保持久化到磁盘。
// 配置库使用强一致写入策略。
func (s *Storage) SyncConfigDB() error {
	return s.configDB.Sync()
}
