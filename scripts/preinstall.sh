#!/bin/bash
# preinstall.sh - 软件安装前备份用户配置和参数
set -e

ROOT_DIR="/usr/local/bin/edgeCore"
CONFIG_DIR="$ROOT_DIR/config"
BACKUP_DIR="/tmp/edgeCore_backup"

echo "[preinstall] Starting preinstall script..."

# 清理旧备份，确保每次安装都是干净的
rm -rf "$BACKUP_DIR" 2>/dev/null || true
mkdir -p "$BACKUP_DIR/data"
mkdir -p "$BACKUP_DIR/config"

backup_dir() {
    local src="$1"
    local dst="$2"

    if [ -d "$src" ]; then
        echo "[preinstall] Backing up $src → $dst"
        mkdir -p "$dst"
        # 若目录为空 cp 会报错，因此忽略错误
        cp -a "$src/"* "$dst/" 2>/dev/null || true
    else
        echo "[preinstall] Directory not found: $src (skip)"
    fi
}

# 备份目录
backup_dir "$CONFIG_DIR" "$BACKUP_DIR/config"

echo "[preinstall] Completed."
