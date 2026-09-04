package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"go.etcd.io/bbolt"
)

// BackupDB 将指定数据库文件备份到目标目录，返回备份信息。
// 用于配置库 / 运行库的备份能力。
func BackupDB(dbPath, backupDir string) (*model.BackupInfo, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file does not exist: %s", dbPath)
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	backupTime := time.Now()
	backupFileName := fmt.Sprintf("config-backup-%s.db", backupTime.Format("20060102-150405"))
	backupPath := filepath.Join(backupDir, backupFileName)

	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{
		Timeout:  30 * time.Second,
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	backupFile, err := os.Create(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer backupFile.Close()

	if err := db.View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(backupFile)
		return err
	}); err != nil {
		os.Remove(backupPath)
		return nil, fmt.Errorf("failed to write backup: %w", err)
	}

	fileInfo, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup file info: %w", err)
	}

	return &model.BackupInfo{
		BackupPath:    backupPath,
		BackupTime:    backupTime,
		OriginalPath:  dbPath,
		FileSizeBytes: fileInfo.Size(),
		Version:       "1.0",
	}, nil
}

// RestoreDB 将备份文件恢复到目标数据库路径。
func RestoreDB(backupPath, targetPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %s", backupPath)
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing database: %w", err)
	}

	backupFile, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer backupFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target database: %w", err)
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, backupFile); err != nil {
		os.Remove(targetPath)
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return nil
}
