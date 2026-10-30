package storage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ArchiveManifestName   = "manifest.json"
	ConfigDBEntryName     = "config.db"
	ConfigDBEntrySQLite   = "config.sqlite"
	RuntimeDBEntryName    = "runtime.db"
	RuntimeDBEntrySQLite  = "runtime.sqlite"

	// bbolt meta 页魔数 0xED0CDAED（小端）位于文件头 page header（16 字节）之后。
	bboltMagicPrefix = "\xED\xDA\x0C\xED"
	sqliteMagic      = "SQLite format 3\x00"
)

// ArchiveManifest describes export package metadata.
type ArchiveManifest struct {
	Type       string    `json:"type"`
	Version    string    `json:"version"`
	ExportTime time.Time `json:"export_time"`
	SourcePath string    `json:"source_path,omitempty"`
}

// ConfigImportResult summarizes a config database import operation.
type ConfigImportResult struct {
	ForceOverwrite bool   `json:"force_overwrite"`
	PreservedUsers bool   `json:"preserved_users"`
	PreservedPort  int    `json:"preserved_port"`
	DeviceCount    int    `json:"device_count"`
	ChannelCount   int    `json:"channel_count"`
	RemoteSource   string `json:"remote_source,omitempty"`
}

// ImportArchiveOptions controls import behavior.
type ImportArchiveOptions struct {
	ForceOverwrite bool
}

// ImportPreserveOptions controls fields preserved during full config import.
type ImportPreserveOptions struct {
	PreserveUsers      bool
	PreserveServerPort bool
}

// ExportDBAsTarGz exports a database file as a tar.gz archive.
// Used when the database file is not already open (e.g. tests).
// 按文件头魔数自动识别 bbolt / SQLite 格式，与本地后端无关。
func ExportDBAsTarGz(dbPath, entryName, archiveType string) ([]byte, string, error) {
	db, err := openKVFileByMagic(dbPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	return exportKVAsTarGz(db, dbPath, entryName, archiveType)
}

// openKVFileByMagic 按文件头魔数打开磁盘上任意后端的数据库文件（跨后端读取）。
func openKVFileByMagic(dbPath string) (KVDB, error) {
	head := make([]byte, 32)
	// #nosec G304 -- dbPath 为服务端内部配置库路径（导出/校验流程自产），非外部输入
	f, err := os.Open(dbPath)
	if err != nil {
		return nil, err
	}
	n, readErr := io.ReadFull(f, head)
	closeErr := f.Close()
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read database header: %w", readErr)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	head = head[:n]

	if isSQLiteDB(head) {
		return openSQLiteKV(dbPath)
	}
	if isBoltDB(head) {
		return openBoltKV(dbPath, false)
	}
	return nil, fmt.Errorf("unrecognized database format: %s", dbPath)
}

// exportKVAsTarGz snapshots an open KV database into tar.gz.
// 快照统一走 KVDB.BackupTo（bbolt: WriteTo / sqlite: VACUUM INTO），
// 与具体后端解耦。
func exportKVAsTarGz(db KVDB, sourcePath, entryName, archiveType string) ([]byte, string, error) {
	if strings.TrimSpace(entryName) == "" {
		return nil, "", fmt.Errorf("archive entry name is required")
	}

	tmpFile, err := os.CreateTemp("", "edgeCore-snapshot-*")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create snapshot temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := db.BackupTo(tmpPath); err != nil {
		return nil, "", fmt.Errorf("failed to snapshot database: %w", err)
	}
	// #nosec G304 -- tmpPath 由 os.CreateTemp 以固定前缀生成，完全受信
	dbBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read database snapshot: %w", err)
	}

	manifest, err := json.Marshal(ArchiveManifest{
		Type:       archiveType,
		Version:    "1.0",
		ExportTime: time.Now(),
		SourcePath: sourcePath,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode manifest: %w", err)
	}

	archiveData, err := buildTarGz(map[string][]byte{
		entryName:           dbBytes,
		ArchiveManifestName: manifest,
	})
	if err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("edgeCore-%s-%s.tar.gz", archiveType, time.Now().Format("20060102-150405"))
	return archiveData, filename, nil
}

func buildTarGz(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for name, content := range files {
		header := &tar.Header{
			Name:    name,
			Mode:    0600,
			Size:    int64(len(content)),
			ModTime: time.Now(),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("failed to write tar header for %s: %w", name, err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write tar entry for %s: %w", name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize tar archive: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize gzip archive: %w", err)
	}

	return buf.Bytes(), nil
}

func extractTarGz(data []byte) (map[string][]byte, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	files := make(map[string][]byte)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry %s: %w", header.Name, err)
		}

		name := filepath.ToSlash(header.Name)
		name = strings.TrimPrefix(name, "./")
		files[name] = content
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("archive is empty")
	}

	return files, nil
}

// isSQLiteDB / isBoltDB 按文件头魔数识别数据库格式。
// SQLite 魔数位于偏移 0；bbolt 魔数位于 page header（16 字节）之后。
func isSQLiteDB(data []byte) bool {
	return len(data) >= len(sqliteMagic) && string(data[:len(sqliteMagic)]) == sqliteMagic
}

func isBoltDB(data []byte) bool {
	return len(data) >= 20 && string(data[16:20]) == bboltMagicPrefix
}

func findArchiveEntry(files map[string][]byte, entryName string) ([]byte, error) {
	if content, ok := files[entryName]; ok {
		return content, nil
	}

	suffix := "/" + entryName
	for name, content := range files {
		if name == entryName || strings.HasSuffix(name, suffix) {
			return content, nil
		}
	}

	return nil, fmt.Errorf("archive does not contain %s", entryName)
}

// ExportConfigDBArchive exports the config database as tar.gz（条目名随后端）。
func (s *Storage) ExportConfigDBArchive() ([]byte, string, error) {
	entryName := ConfigDBEntryName
	if s.backend == BackendSQLite {
		entryName = ConfigDBEntrySQLite
	}
	return exportKVAsTarGz(s.configDB, s.configDB.Path(), entryName, "config")
}

// ExportRuntimeDBArchive exports the runtime database as tar.gz。
func (s *Storage) ExportRuntimeDBArchive() ([]byte, string, error) {
	entryName := RuntimeDBEntryName
	if s.backend == BackendSQLite {
		entryName = RuntimeDBEntrySQLite
	}
	return exportKVAsTarGz(s.runtimeDB, s.runtimeDB.Path(), entryName, "runtime")
}

// ImportConfigDBData 从上传数据导入配置（统一入口，"恢复备份"走这里）。
// data 可以是：
//   - tar.gz 归档（export-config-db 产物，内含 config.db / config.sqlite 条目）
//   - 原始配置库备份文件（BackupConfigDB 产出的 .db / .sqlite）
//
// 按文件头魔数自动识别格式，与本地后端无关（bbolt 系统可恢复 SQLite 备份，反之亦然）。
// By default existing user accounts/passwords and server port are preserved.
func (s *Storage) ImportConfigDBData(data []byte, opts ImportArchiveOptions) (*ConfigImportResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("uploaded data is empty")
	}
	if isSQLiteDB(data) || isBoltDB(data) {
		return s.importConfigFromDBBytes(data, opts, "")
	}
	return s.ImportConfigDBArchive(data, opts)
}

// ImportConfigDBArchive imports configuration from a tar.gz archive.
// By default existing user accounts/passwords and server port are preserved.
// 归档内容按魔数自动识别 bbolt / SQLite 格式，与本地后端无关（跨后端导入）。
func (s *Storage) ImportConfigDBArchive(archiveData []byte, opts ImportArchiveOptions) (*ConfigImportResult, error) {
	files, err := extractTarGz(archiveData)
	if err != nil {
		return nil, err
	}

	var dbContent []byte
	for _, entry := range []string{ConfigDBEntryName, ConfigDBEntrySQLite} {
		if content, err := findArchiveEntry(files, entry); err == nil {
			dbContent = content
			break
		}
	}
	if dbContent == nil {
		return nil, fmt.Errorf("archive does not contain a config database entry")
	}

	return s.importConfigFromDBBytes(dbContent, opts, "")
}

// importConfigFromDBBytes 把一段配置库内容（原始 bbolt / SQLite 文件字节）
// 中的配置导入当前配置库，全程 KVDB 接口，与后端无关。
func (s *Storage) importConfigFromDBBytes(dbContent []byte, opts ImportArchiveOptions, remoteSource string) (*ConfigImportResult, error) {
	if !isSQLiteDB(dbContent) && !isBoltDB(dbContent) {
		return nil, fmt.Errorf("unrecognized config database format")
	}

	tempDir, err := os.MkdirTemp("", "edgeCore-config-import-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// 按魔数判定格式，落到对应扩展名
	tempDBName := ConfigDBEntryName
	if isSQLiteDB(dbContent) {
		tempDBName = ConfigDBEntrySQLite
	}
	tempDBPath := filepath.Join(tempDir, tempDBName)
	if err := os.WriteFile(tempDBPath, dbContent, 0600); err != nil {
		return nil, fmt.Errorf("failed to write temp database: %w", err)
	}

	var importKV KVDB
	if isSQLiteDB(dbContent) {
		importKV, err = openSQLiteKV(tempDBPath)
	} else {
		importKV, err = openBoltKV(tempDBPath, false)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid config database in archive: %w", err)
	}
	defer importKV.Close()

	importStore, err := NewConfigStore(importKV)
	if err != nil {
		return nil, fmt.Errorf("invalid config database structure: %w", err)
	}

	exportData, err := importStore.ExportAllConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config from archive: %w", err)
	}

	if len(exportData.Channels) == 0 && len(exportData.Devices) == 0 {
		return nil, fmt.Errorf("archive does not contain valid configuration data")
	}

	currentStore, err := NewConfigStore(s.configDB)
	if err != nil {
		return nil, fmt.Errorf("failed to open current config store: %w", err)
	}

	preserveUsers := !opts.ForceOverwrite
	preservePort := !opts.ForceOverwrite

	preservedPort := 0
	if preservePort {
		if serverConfig, err := currentStore.LoadServerConfig(); err == nil && serverConfig != nil {
			preservedPort = serverConfig.Port
		}
	} else if exportData.Server.Port > 0 {
		preservedPort = exportData.Server.Port
	}

	importOpts := ImportPreserveOptions{
		PreserveUsers:      preserveUsers,
		PreserveServerPort: preservePort,
	}

	if err := currentStore.ImportConfigReplace(exportData, importOpts); err != nil {
		return nil, fmt.Errorf("failed to import configuration: %w", err)
	}

	if err := s.SyncConfigDB(); err != nil {
		return nil, fmt.Errorf("config imported but failed to sync: %w", err)
	}

	return &ConfigImportResult{
		ForceOverwrite: opts.ForceOverwrite,
		PreservedUsers: preserveUsers,
		PreservedPort:  preservedPort,
		DeviceCount:    len(exportData.Devices),
		ChannelCount:   len(exportData.Channels),
		RemoteSource:   remoteSource,
	}, nil
}

// FetchRemoteConfigArchive downloads a config tar.gz from a remote edgeCore gateway.
func FetchRemoteConfigArchive(host string, port int, token string, useHTTPS bool) ([]byte, string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, "", fmt.Errorf("remote host is required")
	}
	if port <= 0 {
		port = 8080
	}

	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/api/data/export-config-db", scheme, host, port)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create remote request: %w", err)
	}
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("token", token)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to connect remote gateway: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read remote response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if len(body) > 512 {
			body = body[:512]
		}
		return nil, "", fmt.Errorf("remote gateway returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, "", fmt.Errorf("remote gateway returned empty archive")
	}

	source := fmt.Sprintf("%s://%s:%d", scheme, host, port)
	return body, source, nil
}

// PullRemoteConfigAndImport 从远程网关拉取配置并导入本地。
// P1-17 防护（2026-10-13 审查）：此前硬编码 ForceOverwrite=true——远端的
// 用户账号/密码与 System（含网络）配置直接覆盖本地，无任何开关与确认。
// 现在默认保留本地用户与端口（与 import-config-db 一致），仅当调用方显式
// overwriteUsers=true 才整体覆盖。
func (s *Storage) PullRemoteConfigAndImport(host string, port int, token string, useHTTPS, overwriteUsers bool) (*ConfigImportResult, error) {
	archiveData, source, err := FetchRemoteConfigArchive(host, port, token, useHTTPS)
	if err != nil {
		return nil, err
	}

	result, err := s.ImportConfigDBArchive(archiveData, ImportArchiveOptions{ForceOverwrite: overwriteUsers})
	if err != nil {
		return nil, err
	}
	result.RemoteSource = source
	return result, nil
}
