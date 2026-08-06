package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"go.etcd.io/bbolt"
)

type Storage struct {
	configDB  *bbolt.DB
	runtimeDB *bbolt.DB
	dataDir   string
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

func (s *Storage) GetDB() *bbolt.DB {
	return s.runtimeDB
}

func (s *Storage) GetConfigDB() *bbolt.DB {
	return s.configDB
}

func (s *Storage) getDBByBucket(bucketName string) *bbolt.DB {
	if IsConfigBucket(bucketName) {
		return s.configDB
	}
	return s.runtimeDB
}

func (s *Storage) SaveOfflineMessage(configID string, data []byte, maxCount int) error {
	return s.runtimeDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketNorthboundCache))
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketNorthboundCache)
		}

		key := fmt.Sprintf("%s_%d", configID, time.Now().UnixNano())

		if err := b.Put([]byte(key), data); err != nil {
			return err
		}

		c := b.Cursor()
		prefix := []byte(configID + "_")
		count := 0
		var keysToDelete [][]byte

		for k, _ := c.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			count++
		}

		if count > maxCount {
			toDelete := count - maxCount
			for k, _ := c.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
				if toDelete <= 0 {
					break
				}
				keysToDelete = append(keysToDelete, append([]byte{}, k...))
				toDelete--
			}
		}

		for _, k := range keysToDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Storage) GetOfflineMessages(configID string, limit int) ([]OfflineMessage, error) {
	var messages []OfflineMessage
	err := s.runtimeDB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketNorthboundCache))
		if b == nil {
			return nil
		}

		c := b.Cursor()
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
	return s.runtimeDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketNorthboundCache))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(key))
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

func NewStorage(dataDir string) (*Storage, error) {
	if dataDir == "" {
		dataDir = "data"
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	configPath := filepath.Join(dataDir, "config.db")
	runtimePath := filepath.Join(dataDir, "runtime.db")

	// 打开配置数据库（强一致写入）
	configDB, err := openBoltDB(configPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to open config database %s: %w", configPath, err)
	}

	// 初始化配置 bucket
	err = configDB.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range configBucketNames {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return err
			}
		}
		b := tx.Bucket([]byte(BucketConfigVersion))
		if b != nil {
			currentVersion := b.Get([]byte(ConfigVersionKey))
			if currentVersion == nil {
				return b.Put([]byte(ConfigVersionKey), []byte(ConfigVersionValue))
			}
		}
		return nil
	})
	if err != nil {
		configDB.Close()
		return nil, fmt.Errorf("failed to init config buckets: %w", err)
	}

	// 打开运行时数据库（允许清理、compact、重建）
	runtimeDB, err := openBoltDB(runtimePath, true)
	if err != nil {
		configDB.Close()
		return nil, fmt.Errorf("failed to open runtime database %s: %w", runtimePath, err)
	}

	// 初始化运行时 bucket
	err = runtimeDB.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range runtimeBucketNames {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
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
		dataDir:   dataDir,
	}
	if err := storage.dropLegacyShadowWALBucket(); err != nil {
		runtimeDB.Close()
		configDB.Close()
		return nil, fmt.Errorf("failed to drop legacy shadow WAL bucket: %w", err)
	}

	return storage, nil
}

func (s *Storage) Close() error {
	err1 := s.configDB.Close()
	err2 := s.runtimeDB.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (s *Storage) SaveData(bucketName string, key string, data interface{}) error {
	db := s.getDBByBucket(bucketName)
	return db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return err
		}
		bytes, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return b.Put([]byte(key), bytes)
	})
}

func (s *Storage) GetData(bucketName string, key string, result interface{}) error {
	db := s.getDBByBucket(bucketName)
	return db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucketName)
		}
		data := b.Get([]byte(key))
		if data == nil {
			return fmt.Errorf("key %s not found in bucket %s", key, bucketName)
		}
		return json.Unmarshal(data, result)
	})
}

func (s *Storage) DeleteData(bucketName string, key string) error {
	db := s.getDBByBucket(bucketName)
	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(key))
	})
}

func (s *Storage) PruneOldest(bucketName string, maxRecords int) error {
	db := s.getDBByBucket(bucketName)
	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}

		count := b.Stats().KeyN
		if count <= maxRecords {
			return nil
		}

		deleteCount := count - maxRecords
		c := b.Cursor()
		for i := 0; i < deleteCount; i++ {
			k, _ := c.First()
			if k == nil {
				break
			}
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Storage) LoadLatest(bucketName string, limit int, callback func(k, v []byte) error) error {
	db := s.getDBByBucket(bucketName)
	return db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}

		c := b.Cursor()
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
	return db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		return b.ForEach(callback)
	})
}

func (s *Storage) LoadRange(bucketName string, minKey, maxKey string, callback func(k, v []byte) error) error {
	db := s.getDBByBucket(bucketName)
	return db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.Seek([]byte(minKey)); k != nil && string(k) <= maxKey; k, v = c.Next() {
			if err := callback(k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Storage) SaveValue(val model.Value) error {
	return s.runtimeDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketValues))

		data, err := json.Marshal(val)
		if err != nil {
			return err
		}

		return b.Put([]byte(val.PointID), data)
	})
}

func (s *Storage) GetLastValue(pointID string) (*model.Value, error) {
	var val model.Value
	err := s.runtimeDB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketValues))
		data := b.Get([]byte(pointID))
		if data == nil {
			return fmt.Errorf("not found")
		}
		return json.Unmarshal(data, &val)
	})
	return &val, err
}

func (s *Storage) GetAllValues() (map[string]model.Value, error) {
	result := make(map[string]model.Value)
	err := s.runtimeDB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketValues))
		return b.ForEach(func(k, v []byte) error {
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
	return s.runtimeDB.Update(func(tx *bbolt.Tx) error {
		if tx.Bucket([]byte(legacyShadowWALBucket)) == nil {
			return nil
		}
		return tx.DeleteBucket([]byte(legacyShadowWALBucket))
	})
}

func IsConfigBucket(name string) bool {
	return configBucketMap[name]
}

func (s *Storage) GetBucketStats() ([]BucketStats, int64, error) {
	var stats []BucketStats
	var totalSize int64

	configFileInfo, err := os.Stat(s.configDB.Path())
	if err != nil {
		return nil, 0, err
	}
	totalSize += configFileInfo.Size()

	runtimeFileInfo, err := os.Stat(s.runtimeDB.Path())
	if err != nil {
		return nil, 0, err
	}
	totalSize += runtimeFileInfo.Size()

	err = s.configDB.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
			count := 0
			size := int64(0)
			b.ForEach(func(k, v []byte) error {
				count++
				size += int64(len(k) + len(v))
				return nil
			})
			category, clearable := classifyBucket(string(name))
			stats = append(stats, BucketStats{
				Name:        string(name),
				RecordCount: count,
				TotalSize:   size,
				Category:    category,
				Clearable:   clearable,
				Database:    "config",
			})
			return nil
		})
	})
	if err != nil {
		return nil, 0, err
	}

	err = s.runtimeDB.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
			count := 0
			size := int64(0)
			b.ForEach(func(k, v []byte) error {
				count++
				size += int64(len(k) + len(v))
				return nil
			})
			category, clearable := classifyBucket(string(name))
			stats = append(stats, BucketStats{
				Name:        string(name),
				RecordCount: count,
				TotalSize:   size,
				Category:    category,
				Clearable:   clearable,
				Database:    "runtime",
			})
			return nil
		})
	})
	if err != nil {
		return nil, 0, err
	}

	stats = ensureKnownRuntimeBuckets(stats)
	sortBucketStats(stats)

	return stats, totalSize, err
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
	return s.runtimeDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucketName)
		}

		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if err := c.Delete(); err != nil {
				return err
			}
		}
		return nil
	})
}

// ClearAllRuntimeBuckets 清空 runtime.db 中的全部 bucket（含 values、缓存、历史、遗留 WAL 等）。
// 配置 bucket 不应出现在 runtime.db；若检测到则报错。
func (s *Storage) ClearAllRuntimeBuckets() ([]string, error) {
	var cleared []string

	err := s.runtimeDB.Update(func(tx *bbolt.Tx) error {
		var bucketNames []string
		if err := tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
			bucketNames = append(bucketNames, string(name))
			return nil
		}); err != nil {
			return err
		}

		for _, bucketName := range bucketNames {
			if IsConfigBucket(bucketName) {
				return fmt.Errorf("config bucket %s found in runtime db", bucketName)
			}

			if bucketName == legacyShadowWALBucket {
				if err := tx.DeleteBucket([]byte(bucketName)); err != nil {
					return err
				}
				cleared = append(cleared, bucketName)
				continue
			}

			b := tx.Bucket([]byte(bucketName))
			if b == nil {
				continue
			}
			c := b.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if err := c.Delete(); err != nil {
					return err
				}
			}
			cleared = append(cleared, bucketName)
		}
		return nil
	})

	return cleared, err
}

const compactTxMaxSize = 65536 // matches bbolt CLI default; avoids OOM during large compacts

// CompactRuntimeDB 压缩运行时数据库文件，回收已删除数据的空间。
// 使用 bbolt.Compact 仅复制仍存活的数据，替换原文件。
func (s *Storage) CompactRuntimeDB() error {
	runtimePath := s.runtimeDB.Path()

	if err := s.runtimeDB.Close(); err != nil {
		return fmt.Errorf("failed to close runtime db: %w", err)
	}

	tmpPath := runtimePath + ".compact.tmp"
	_ = os.Remove(tmpPath)

	src, err := bbolt.Open(runtimePath, 0600, &bbolt.Options{
		Timeout:  boltOpenTimeout,
		ReadOnly: true,
	})
	if err != nil {
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to open runtime db for compact: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to open runtime db for compact: %w", err)
	}

	dst, err := openBoltDB(tmpPath, true)
	if err != nil {
		_ = src.Close()
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to create compact temp db: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to create compact temp db: %w", err)
	}

	if err := bbolt.Compact(dst, src, compactTxMaxSize); err != nil {
		_ = dst.Close()
		_ = src.Close()
		_ = os.Remove(tmpPath)
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to compact runtime db: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to compact runtime db: %w", err)
	}

	if err := dst.Close(); err != nil {
		_ = src.Close()
		_ = os.Remove(tmpPath)
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to close compact temp db: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to close compact temp db: %w", err)
	}
	if err := src.Close(); err != nil {
		_ = os.Remove(tmpPath)
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to close runtime db source: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to close runtime db source: %w", err)
	}

	tmpInfo, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to stat compacted file: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to stat compacted file: %w", err)
	}
	if tmpInfo.Size() == 0 {
		_ = os.Remove(tmpPath)
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("compacted file is empty, aborting (reopen: %v)", reopenErr)
		}
		return fmt.Errorf("compacted file is empty, aborting")
	}

	backupPath := runtimePath + ".pre-compact.bak"
	if err := os.Rename(runtimePath, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to backup original runtime db: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to backup original runtime db: %w", err)
	}

	if err := os.Rename(tmpPath, runtimePath); err != nil {
		_ = os.Rename(backupPath, runtimePath)
		if reopenErr := s.reopenRuntimeDB(runtimePath); reopenErr != nil {
			return fmt.Errorf("failed to replace runtime db with compacted file: %w (reopen: %v)", err, reopenErr)
		}
		return fmt.Errorf("failed to replace runtime db with compacted file: %w", err)
	}

	if err := s.reopenRuntimeDB(runtimePath); err != nil {
		return fmt.Errorf("failed to reopen compacted runtime db: %w", err)
	}

	if err := s.runtimeDB.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range runtimeBucketNames {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to reinit runtime buckets after compact: %w", err)
	}

	_ = os.Remove(backupPath)
	return nil
}

func (s *Storage) reopenRuntimeDB(path string) error {
	db, err := openBoltDB(path, true)
	if err != nil {
		return err
	}
	s.runtimeDB = db
	return nil
}

// BackupConfigDB 备份配置数据库到指定目录。
// 配置库优先备份，不包含运行时数据。
func (s *Storage) BackupConfigDB(backupDir string) (*model.BackupInfo, error) {
	return BackupDB(s.configDB.Path(), backupDir)
}

// BackupRuntimeDB 备份运行时数据库到指定目录（可选诊断用途）。
func (s *Storage) BackupRuntimeDB(backupDir string) (*model.BackupInfo, error) {
	runtimeBackupDir := filepath.Join(backupDir, "runtime")
	return BackupDB(s.runtimeDB.Path(), runtimeBackupDir)
}

// SyncConfigDB 对配置数据库执行强制同步，确保持久化到磁盘。
// 配置库使用强一致写入策略。
func (s *Storage) SyncConfigDB() error {
	return s.configDB.Sync()
}
