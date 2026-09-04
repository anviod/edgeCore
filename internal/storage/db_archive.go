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

	"go.etcd.io/bbolt"
)

const (
	ArchiveManifestName = "manifest.json"
	ConfigDBEntryName   = "config.db"
	RuntimeDBEntryName  = "runtime.db"
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

// ExportDBAsTarGz exports a bbolt database file as a tar.gz archive.
// Used when the database file is not already open (e.g. tests).
func ExportDBAsTarGz(dbPath, entryName, archiveType string) ([]byte, string, error) {
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{
		Timeout:  30 * time.Second,
		ReadOnly: true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	return exportOpenDBAsTarGz(db, dbPath, entryName, archiveType)
}

// exportOpenDBAsTarGz snapshots an open bbolt database into tar.gz.
func exportOpenDBAsTarGz(db *bbolt.DB, sourcePath, entryName, archiveType string) ([]byte, string, error) {
	if strings.TrimSpace(entryName) == "" {
		return nil, "", fmt.Errorf("archive entry name is required")
	}

	var dbBytes bytes.Buffer
	if err := db.View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(&dbBytes)
		return err
	}); err != nil {
		return nil, "", fmt.Errorf("failed to snapshot database: %w", err)
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
		entryName:           dbBytes.Bytes(),
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

// ExportConfigDBArchive exports config.db as tar.gz.
func (s *Storage) ExportConfigDBArchive() ([]byte, string, error) {
	return exportOpenDBAsTarGz(s.configDB, s.configDB.Path(), ConfigDBEntryName, "config")
}

// ExportRuntimeDBArchive exports runtime.db as tar.gz.
func (s *Storage) ExportRuntimeDBArchive() ([]byte, string, error) {
	return exportOpenDBAsTarGz(s.runtimeDB, s.runtimeDB.Path(), RuntimeDBEntryName, "runtime")
}

// ImportConfigDBArchive imports configuration from a tar.gz archive.
// By default existing user accounts/passwords and server port are preserved.
func (s *Storage) ImportConfigDBArchive(archiveData []byte, opts ImportArchiveOptions) (*ConfigImportResult, error) {
	files, err := extractTarGz(archiveData)
	if err != nil {
		return nil, err
	}

	dbContent, err := findArchiveEntry(files, ConfigDBEntryName)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "edgeCore-config-import-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempDBPath := filepath.Join(tempDir, ConfigDBEntryName)
	if err := os.WriteFile(tempDBPath, dbContent, 0600); err != nil {
		return nil, fmt.Errorf("failed to write temp database: %w", err)
	}

	importDB, err := bbolt.Open(tempDBPath, 0600, &bbolt.Options{
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid config database in archive: %w", err)
	}
	defer importDB.Close()

	importStore, err := NewConfigStore(importDB)
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

// PullRemoteConfigAndImport force-pulls configuration from a remote gateway and overwrites local config.
func (s *Storage) PullRemoteConfigAndImport(host string, port int, token string, useHTTPS bool) (*ConfigImportResult, error) {
	archiveData, source, err := FetchRemoteConfigArchive(host, port, token, useHTTPS)
	if err != nil {
		return nil, err
	}

	result, err := s.ImportConfigDBArchive(archiveData, ImportArchiveOptions{ForceOverwrite: true})
	if err != nil {
		return nil, err
	}
	result.RemoteSource = source
	return result, nil
}
