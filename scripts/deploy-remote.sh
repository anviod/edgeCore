#!/bin/bash
# edgeCore 远程自动部署脚本
# 使用方式: bash scripts/deploy-remote.sh <远程主机> <节点名称> [包路径]

set -e

# 默认配置
DEFAULT_HOST="root@192.168.3.230"
DEFAULT_PACKAGE=""
DEFAULT_NODE_NAME="NODE-REMOTE"

# 参数处理
HOST="${1:-$DEFAULT_HOST}"
NODE_NAME="${2:-$DEFAULT_NODE_NAME}"
PACKAGE="${3:-$DEFAULT_PACKAGE}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# 检查SSH连接
check_ssh() {
    info "检查SSH连接..."
    if ! ssh -q -o ConnectTimeout=5 "$HOST" "echo 'SSH连接正常'" > /dev/null 2>&1; then
        error "无法连接到远程主机: $HOST"
    fi
    info "SSH连接正常"
}

# 检测远程架构
detect_arch() {
    info "检测远程主机架构..."
    ARCH=$(ssh "$HOST" "uname -m")
    info "远程架构: $ARCH"
    
    case "$ARCH" in
        aarch64)
            DEB_ARCH="arm64"
            ;;
        armv7l)
            DEB_ARCH="arm"
            ;;
        x86_64)
            DEB_ARCH="amd64"
            ;;
        *)
            error "不支持的架构: $ARCH"
            ;;
    esac
    
    info "目标包架构: $DEB_ARCH"
}

# 查找安装包
find_package() {
    if [ -n "$PACKAGE" ]; then
        if [ -f "$PACKAGE" ]; then
            info "使用指定包: $PACKAGE"
            return
        fi
        error "指定的包不存在: $PACKAGE"
    fi
    
    info "自动查找最新的 $DEB_ARCH 包..."
    LATEST_PACKAGE=$(ls -1 dist/edgeCore-v*-"$DEB_ARCH".deb 2>/dev/null | sort -V | tail -1)
    
    if [ -z "$LATEST_PACKAGE" ]; then
        LATEST_PACKAGE=$(ls -1 dist/edgeCore-*-"$DEB_ARCH".deb 2>/dev/null | sort -V | tail -1)
    fi
    
    if [ -z "$LATEST_PACKAGE" ]; then
        error "未找到 $DEB_ARCH 架构的安装包，请先构建"
    fi
    
    PACKAGE="$LATEST_PACKAGE"
    info "找到安装包: $PACKAGE"
}

# 复制安装包到远程
copy_package() {
    info "复制安装包到远程主机..."
    scp "$PACKAGE" "$HOST:/tmp/"
    REMOTE_PACKAGE="/tmp/$(basename "$PACKAGE")"
    info "包已复制到: $HOST:$REMOTE_PACKAGE"
}

# 备份现有配置
backup_config() {
    info "备份现有配置..."
    ssh "$HOST" <<EOF
        if [ -d /usr/local/bin/edgeCore ]; then
            mkdir -p /tmp/edgeCore_backup
            cp -rf /usr/local/bin/edgeCore/data /tmp/edgeCore_backup/ 2>/dev/null || true
            cp -rf /usr/local/bin/edgeCore/config /tmp/edgeCore_backup/ 2>/dev/null || true
            echo "备份完成"
        else
            echo "无现有配置需要备份"
        fi
EOF
}

# 停止服务
stop_service() {
    info "停止现有服务..."
    ssh "$HOST" "systemctl stop edgeCore 2>/dev/null || true"
    info "服务已停止"
}

# 卸载旧版本
uninstall_old() {
    info "卸载旧版本..."
    ssh "$HOST" "apt remove -y edgeCore 2>/dev/null || true"
    info "旧版本已卸载"
}

# 安装新版本
install_new() {
    info "安装新版本..."
    ssh "$HOST" "dpkg -i $REMOTE_PACKAGE || apt-get install -f -y"
    info "新版本安装完成"
}

# 配置节点名称
configure_node() {
    info "配置节点名称: $NODE_NAME..."
    ssh "$HOST" <<EOF
        if [ -f /usr/local/bin/edgeCore/config/sync.yaml ]; then
            sed -i "s/node_name:.*/node_name: $NODE_NAME/" /usr/local/bin/edgeCore/config/sync.yaml
            echo "节点名称已配置"
        else
            echo "配置文件不存在，跳过节点名称配置"
        fi
EOF
}

# 启动服务
start_service() {
    info "启动服务..."
    ssh "$HOST" "systemctl daemon-reload && systemctl enable edgeCore && systemctl start edgeCore"
    info "服务已启动"
}

# 验证服务状态
verify_service() {
    info "验证服务状态..."
    sleep 5
    STATUS=$(ssh "$HOST" "systemctl is-active edgeCore")
    if [ "$STATUS" = "active" ]; then
        info "服务状态: ${GREEN}运行中${NC}"
    else
        warn "服务状态: ${RED}未运行${NC}"
        warn "查看日志: journalctl -u edgeCore -n 50"
    fi
}

# 主流程
main() {
    info "========== edgeCore 远程部署开始 =========="
    
    check_ssh
    detect_arch
    find_package
    copy_package
    backup_config
    stop_service
    uninstall_old
    install_new
    configure_node
    start_service
    verify_service
    
    info "========== edgeCore 远程部署完成 =========="
    info "远程主机: $HOST"
    info "节点名称: $NODE_NAME"
    info "安装包: $PACKAGE"
}

main "$@"