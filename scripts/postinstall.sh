#!/bin/bash
# 安装后处理脚本（安全版）
set -e

DATA_DIR="/usr/local/bin/edgeCore/data"
CONFIG_DIR="/usr/local/bin/edgeCore/config"
BACKUP_DIR="/tmp/edgeCore_backup"

echo "[postinstall] Running postinstall script..."

restore_dir() {
    local src="$1"
    local dst="$2"

    if [ -d "$src" ]; then
        echo "[postinstall] Restoring $dst from backup..."
        mkdir -p "$dst"
        cp -rf "$src/"* "$dst/" 2>/dev/null || true
    else
        echo "[postinstall] No backup found for $dst, ensuring directory exists."
        mkdir -p "$dst"
    fi
}

# 恢复 data 和 config
restore_dir "$BACKUP_DIR/data" "$DATA_DIR"
restore_dir "$BACKUP_DIR/config" "$CONFIG_DIR"

# 仅在 systemctl 存在时执行 systemd 操作
if command -v systemctl >/dev/null 2>&1; then
    echo "[postinstall] Reloading systemd..."
    systemctl daemon-reload || true

    echo "[postinstall] Enabling service..."
    systemctl enable edgeCore || true

    echo "[postinstall] Starting service..."
    systemctl restart edgeCore || true
else
    echo "[postinstall] systemctl not found, skipping systemd service operations."
fi

echo "[postinstall] Completed."
