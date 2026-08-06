#!/bin/bash

# Edge Gateway Sync Setup Wizard
# Interactive script to set up multi-node synchronization

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

print_header() {
    echo -e "${CYAN}"
    echo "=========================================="
    echo "  Edge Gateway Sync Setup Wizard"
    echo "=========================================="
    echo -e "${NC}"
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

print_step() {
    echo -e "${CYAN}[Step $1]${NC} $2"
}

# Check prerequisites
check_prerequisites() {
    print_step "1" "Checking prerequisites..."
    
    local missing=()
    
    if ! command -v go &> /dev/null; then
        missing+=("Go")
    fi
    
    if ! command -v ssh &> /dev/null; then
        missing+=("SSH")
    fi
    
    if ! command -v scp &> /dev/null; then
        missing+=("SCP")
    fi
    
    if [ ${#missing[@]} -ne 0 ]; then
        print_error "Missing prerequisites: ${missing[*]}"
        exit 1
    fi
    
    print_success "All prerequisites met"
    echo ""
}

# Build project
build_project() {
    print_step "2" "Building project..."
    
    print_info "Building for local (Windows)..."
    go build -o bin/edgeCore.exe ./cmd/main.go
    
    print_info "Building for remote (Linux ARM64)..."
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/edgeCore-linux-arm64 ./cmd/main.go
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/sync-test-linux-arm64 ./cmd/sync-test/main.go
    
    print_success "Build completed"
    echo ""
}

# Test SSH connection
test_ssh() {
    local host=$1
    
    print_info "Testing SSH connection to $host..."
    
    if ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$host" "echo 'OK'" &>/dev/null; then
        print_success "SSH connection successful"
        return 0
    else
        print_error "SSH connection failed"
        return 1
    fi
}

# Deploy to remote
deploy_remote() {
    local host=$1
    local node_id=$2
    
    print_step "3" "Deploying to remote host..."
    print_info "Host: $host"
    print_info "Node ID: $node_id"
    
    # Test connection
    if ! test_ssh "$host"; then
        print_info "Please set up SSH key authentication:"
        print_info "  ssh-copy-id $host"
        return 1
    fi
    
    # Deploy
    print_info "Deploying files..."
    
    ssh "$host" "mkdir -p /opt/edgeCore/{bin,conf,data,logs}"
    
    scp bin/edgeCore-linux-arm64 "$host:/opt/edgeCore/bin/edgeCore"
    scp bin/sync-test-linux-arm64 "$host:/opt/edgeCore/bin/sync-test"
    scp -r conf/* "$host:/opt/edgeCore/conf/" 2>/dev/null || true
    
    ssh "$host" "chmod +x /opt/edgeCore/bin/*"
    
    # Create service
    print_info "Creating systemd service..."
    ssh "$host" << EOF
cat > /etc/systemd/system/edgeCore.service << 'SERVICE'
[Unit]
Description=Edge Gateway Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/edgeCore
ExecStart=/opt/edgeCore/bin/edgeCore -conf conf
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable edgeCore
systemctl restart edgeCore
EOF
    
    print_success "Deployment completed"
    echo ""
}

# Start local node
start_local() {
    local node_id=$1
    
    print_step "4" "Starting local node..."
    print_info "Node ID: $node_id"
    
    # Create data directory
    mkdir -p data logs
    
    print_info "Starting node in background..."
    ./bin/sync-test.exe start "$node_id" > "logs/$node_id.log" 2>&1 &
    local pid=$!
    
    echo $pid > "data/$node_id.pid"
    
    sleep 3
    
    if ps -p $pid > /dev/null 2>&1; then
        print_success "Local node started (PID: $pid)"
        print_info "Log file: logs/$node_id.log"
    else
        print_error "Failed to start local node"
        return 1
    fi
    
    echo ""
}

# Show status
show_status() {
    print_step "5" "Checking status..."
    
    print_info "Local nodes:"
    for pid_file in data/*.pid; do
        if [ -f "$pid_file" ]; then
            local node_id=$(basename "$pid_file" .pid)
            local pid=$(cat "$pid_file")
            if ps -p "$pid" > /dev/null 2>&1; then
                print_success "$node_id: Running (PID: $pid)"
            else
                print_error "$node_id: Not running"
            fi
        fi
    done
    
    echo ""
    print_info "Remote nodes:"
    ssh root@192.168.3.230 "systemctl is-active edgeCore" &>/dev/null && \
        print_success "NODE-2@192.168.3.230: Running" || \
        print_error "NODE-2@192.168.3.230: Not running"
    
    echo ""
}

# Main menu
show_menu() {
    echo ""
    echo "=========================================="
    echo "  What would you like to do?"
    echo "=========================================="
    echo ""
    echo "  1) Full Setup (Deploy + Start)"
    echo "  2) Deploy to Remote Only"
    echo "  3) Start Local Node Only"
    echo "  4) Check Status"
    echo "  5) View Logs"
    echo "  6) Stop All Nodes"
    echo "  7) Cleanup"
    echo "  0) Exit"
    echo ""
    echo -n "Enter your choice [0-7]: "
}

# Full setup
full_setup() {
    print_header
    
    echo "This will:"
    echo "  1. Build the project"
    echo "  2. Deploy to remote host (192.168.3.230)"
    echo "  3. Start local node (NODE-1)"
    echo ""
    read -p "Continue? [Y/n]: " confirm
    
    if [[ "$confirm" =~ ^[Nn]$ ]]; then
        return
    fi
    
    check_prerequisites
    build_project
    deploy_remote "root@192.168.3.230" "NODE-2"
    start_local "NODE-1"
    
    echo ""
    echo "=========================================="
    print_success "Setup completed!"
    echo "=========================================="
    echo ""
    echo "Local Node: NODE-1 (Running)"
    echo "Remote Node: NODE-2@192.168.3.230 (Running)"
    echo ""
    echo "Next steps:"
    echo "  - Wait 30 seconds for nodes to discover each other"
    echo "  - Check status: ./setup-sync.sh (option 4)"
    echo "  - View logs: ./setup-sync.sh (option 5)"
    echo ""
}

# View logs
view_logs() {
    echo ""
    echo "Select log to view:"
    echo "  1) Local NODE-1"
    echo "  2) Remote NODE-2"
    echo "  3) Back"
    echo ""
    echo -n "Choice: "
    read choice
    
    case $choice in
        1)
            if [ -f "logs/NODE-1.log" ]; then
                tail -f "logs/NODE-1.log"
            else
                print_error "Log file not found"
            fi
            ;;
        2)
            ssh root@192.168.3.230 "tail -f /opt/edgeCore/logs/edgeCore.log"
            ;;
        *)
            return
            ;;
    esac
}

# Stop all nodes
stop_all() {
    print_info "Stopping all nodes..."
    
    # Stop local nodes
    for pid_file in data/*.pid; do
        if [ -f "$pid_file" ]; then
            local node_id=$(basename "$pid_file" .pid)
            local pid=$(cat "$pid_file")
            
            if ps -p "$pid" > /dev/null 2>&1; then
                print_info "Stopping $node_id (PID: $pid)..."
                kill "$pid" 2>/dev/null || true
            fi
            
            rm -f "$pid_file"
        fi
    done
    
    # Stop remote node
    print_info "Stopping remote node..."
    ssh root@192.168.3.230 "systemctl stop edgeCore" 2>/dev/null || true
    
    print_success "All nodes stopped"
}

# Cleanup
cleanup() {
    print_warning "This will delete all data and logs!"
    read -p "Are you sure? [y/N]: " confirm
    
    if [[ "$confirm" =~ ^[Yy]$ ]]; then
        stop_all
        rm -rf data logs
        print_success "Cleanup completed"
    fi
}

# Main
main() {
    if [ "$1" == "--full" ]; then
        full_setup
        exit 0
    fi
    
    while true; do
        show_menu
        read choice
        
        case $choice in
            1)
                full_setup
                ;;
            2)
                build_project
                deploy_remote "root@192.168.3.230" "NODE-2"
                ;;
            3)
                start_local "NODE-1"
                ;;
            4)
                show_status
                ;;
            5)
                view_logs
                ;;
            6)
                stop_all
                ;;
            7)
                cleanup
                ;;
            0)
                echo "Goodbye!"
                exit 0
                ;;
            *)
                print_error "Invalid choice"
                ;;
        esac
    done
}

# Run main
main "$@"
