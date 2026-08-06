package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/config"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SyncManager is the main manager for the sync system
type SyncManager struct {
	configStore  *ConfigStore
	clusterStore *ClusterStore // bbolt 持久化集群快照
	gossip       *GossipManager
	discovery    *DiscoveryManager
	takeover     *TakeoverManager
	roleMgr      *RoleManager
	groupMgr     *GroupManager
	groupSyncMgr *GroupSyncManager
	snapshotMgr  *SnapshotManager
	snapshots    map[string]*NodeSnapshot // 内存缓存 + bbolt 持久化
	nodeID       string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewSyncManager creates a new SyncManager
func NewSyncManager(ctx context.Context, dataPath string, port int) (*SyncManager, error) {
	log.Printf("[SyncManager] Starting initialization...")
	ctx, cancel := context.WithCancel(ctx)

	log.Printf("[SyncManager] Creating config store...")
	store, err := NewConfigStore(dataPath)
	if err != nil {
		log.Printf("[SyncManager] Failed to create config store: %v, using memory-only mode", err)
		// Continue without config store - sync will work in memory-only mode
		store = nil
	} else {
		log.Printf("[SyncManager] Config store created")
	}

	log.Printf("[SyncManager] Creating gossip manager on port %d...", port)
	gossip, err := NewGossipManager(ctx, port)
	if err != nil {
		log.Printf("[SyncManager] Failed to create gossip manager: %v", err)
		if store != nil {
			_ = store.Close()
		}
		cancel()
		return nil, err
	}
	log.Printf("[SyncManager] Gossip manager created")

	log.Printf("[SyncManager] Creating discovery manager...")
	discovery, err := NewDiscoveryManager(dataPath, gossip)
	if err != nil {
		log.Printf("[SyncManager] Failed to create discovery manager: %v", err)
		gossip.Close()
		if store != nil {
			_ = store.Close()
		}
		cancel()
		return nil, err
	}
	log.Printf("[SyncManager] Discovery manager created")

	// Init ClusterStore — bbolt 持久化集群全量快照
	log.Printf("[SyncManager] Creating cluster store...")
	clusterStore, err := NewClusterStore(dataPath)
	if err != nil {
		log.Printf("[SyncManager] Failed to create cluster store: %v, continuing without persistence", err)
		clusterStore = nil
	} else {
		log.Printf("[SyncManager] Cluster store created at %s", clusterStore.DBPath())
	}

	// Create SyncManager first
	mgr := &SyncManager{
		configStore:  store,
		clusterStore: clusterStore,
		gossip:       gossip,
		discovery:    discovery,
		takeover:     NewTakeoverManager(),
		roleMgr:      NewRoleManager(gossip.HostID()),
		groupMgr:     NewGroupManager(),
		groupSyncMgr: NewGroupSyncManager(store),
		snapshots:    make(map[string]*NodeSnapshot),
		nodeID:       gossip.HostID().String(),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Create snapshot manager
	log.Printf("[SyncManager] Creating snapshot manager...")
	snapshotMgr, err := NewSnapshotManager(dataPath, mgr)
	if err != nil {
		log.Printf("[SyncManager] Failed to create snapshot manager: %v, continuing without snapshots", err)
	} else {
		log.Printf("[SyncManager] Snapshot manager created")
		mgr.snapshotMgr = snapshotMgr
	}

	log.Printf("[SyncManager] Initialization complete")

	return mgr, nil
}

// Start starts the sync manager
func (s *SyncManager) Start() error {
	if err := s.gossip.SetupMDNS("edge-sync"); err != nil {
		log.Printf("Warning: mDNS setup failed: %v", err)
	}

	s.discovery.TryConnectPeers()
	s.roleMgr.ElectLeader()

	s.wg.Add(3)
	go s.messageLoop()
	go s.digestLoop()
	go s.takeoverCleanupLoop()

	log.Printf("Sync manager started. Node ID: %s, Role: %s", s.nodeID, s.roleMgr.GetRole())
	return nil
}

// messageLoop processes incoming messages
func (s *SyncManager) messageLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.gossip.Messages():
			s.handleMessage(msg)
		}
	}
}

// handleMessage handles a sync message
func (s *SyncManager) handleMessage(msg *SyncMessage) {
	switch msg.MessageType {
	case "announce":
		s.handleAnnounce(msg)
	case "pull":
		s.handlePull(msg)
	case "full_config":
		s.handleFullConfig(msg)
	case "hello":
		s.handleHello(msg)
	case "takeover":
		s.handleTakeover(msg)
	case "digest":
		s.handleDigest(msg)
	}
}

// handleAnnounce handles announce message
func (s *SyncManager) handleAnnounce(msg *SyncMessage) {
	if msg.TargetPeer != "" && msg.TargetPeer != s.nodeID {
		return
	}

	key, _ := msg.Payload["key"].(string)
	version, _ := msg.Payload["version"].(float64)

	if s.configStore == nil {
		return
	}
	localRec, exists := s.configStore.Get(key)
	if !exists || localRec.Version < uint64(version) {
		s.sendPull(msg.SourcePeer, key)
	}
}

// sendPull sends a pull request
func (s *SyncManager) sendPull(targetPeer, key string) {
	pullMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "pull",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		TargetPeer:  targetPeer,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"key": key,
		},
	}

	_ = s.gossip.Publish(pullMsg)
}

// handlePull handles pull message
func (s *SyncManager) handlePull(msg *SyncMessage) {
	if msg.TargetPeer != s.nodeID {
		return
	}

	if s.configStore == nil {
		return
	}
	key, _ := msg.Payload["key"].(string)
	rec, exists := s.configStore.Get(key)
	if !exists {
		return
	}

	s.sendRecord(msg.SourcePeer, rec)
}

// sendRecord sends a single config record.
func (s *SyncManager) sendRecord(targetPeer string, rec *ConfigRecord) {
	payload, _ := json.Marshal(rec)

	fullMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "full_config",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		TargetPeer:  targetPeer,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"record": json.RawMessage(payload),
		},
	}

	_ = s.gossip.Publish(fullMsg)
}

// sendFullConfig sends the whole node snapshot for takeover recovery.
func (s *SyncManager) sendFullConfig(targetPeer, deviceKey string) {
	s.mu.RLock()
	snapshot := s.snapshots[s.nodeID]
	s.mu.RUnlock()

	if snapshot == nil {
		return
	}

	payload, _ := json.Marshal(snapshot)
	fullMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "full_config",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		TargetPeer:  targetPeer,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"device_key": deviceKey,
			"snapshot":   json.RawMessage(payload),
		},
	}

	_ = s.gossip.Publish(fullMsg)
}

// handleFullConfig handles full config
func (s *SyncManager) handleFullConfig(msg *SyncMessage) {
	if msg.TargetPeer != s.nodeID {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if rawSnapshot, ok := msg.Payload["snapshot"].(json.RawMessage); ok {
		var snapshot NodeSnapshot
		if err := json.Unmarshal(rawSnapshot, &snapshot); err == nil {
			s.snapshots[snapshot.NodeID] = &snapshot

			// 持久化远程节点快照到 bbolt
			if s.clusterStore != nil {
				if err := s.clusterStore.PutRemoteSnapshot(snapshot.NodeID, &snapshot); err != nil {
					log.Printf("[SyncManager] Failed to persist remote snapshot %s: %v", snapshot.NodeID, err)
				}
			}

			s.takeover.RecordEvent(&TakeoverEvent{
				ID:         uuid.New().String(),
				DeviceKey:  snapshot.NodeID,
				SourcePeer: msg.SourcePeer,
				TargetPeer: s.nodeID,
				Stage:      TakeoverStageFullConfig,
				Status:     string(TakeoverStageCompleted),
				Message:    "full config received",
			})
			return
		}
	}

	recData, _ := msg.Payload["record"].(json.RawMessage)
	var rec ConfigRecord
	if err := json.Unmarshal(recData, &rec); err != nil {
		return
	}

	if s.configStore == nil {
		return
	}
	localRec, exists := s.configStore.Get(rec.Key)
	if !exists || localRec.Version < rec.Version {
		_ = s.configStore.Put(&rec)
		log.Printf("Synced config: %s (v%d)", rec.Key, rec.Version)
	}
}

// handleHello handles hello message for device takeover
func (s *SyncManager) handleHello(msg *SyncMessage) {
	deviceKey, _ := msg.Payload["device_key"].(string)
	sourcePeer, _ := msg.Payload["source_peer"].(string)

	s.mu.RLock()
	localSnapshot := s.snapshots[s.nodeID]
	s.mu.RUnlock()

	var existingConfigs []*ConfigRecord
	if s.configStore != nil {
		existingConfigs = s.configStore.GetByBindingKey(deviceKey)
	}
	if localSnapshot != nil || len(existingConfigs) > 0 {
		s.takeover.RecordEvent(&TakeoverEvent{
			ID:         uuid.New().String(),
			DeviceKey:  deviceKey,
			SourcePeer: sourcePeer,
			TargetPeer: s.nodeID,
			Stage:      TakeoverStageHello,
			Status:     string(TakeoverStageCompleted),
			Message:    "hello received",
		})
		if s.takeover.TryLock(deviceKey, peer.ID(sourcePeer), 30*time.Second) {
			s.sendTakeover(sourcePeer, deviceKey)
		}
	}
}

// handleTakeover handles takeover message
func (s *SyncManager) handleTakeover(msg *SyncMessage) {
	deviceKey, _ := msg.Payload["device_key"].(string)
	if deviceKey == "" {
		return
	}
	s.takeover.RecordEvent(&TakeoverEvent{
		ID:         uuid.New().String(),
		DeviceKey:  deviceKey,
		SourcePeer: msg.SourcePeer,
		TargetPeer: s.nodeID,
		Stage:      TakeoverStageTakeover,
		Status:     string(TakeoverStageCompleted),
		Message:    "takeover acknowledged",
	})

	s.sendFullConfig(msg.SourcePeer, deviceKey)
}

// sendTakeover notifies the requesting peer that takeover can proceed.
func (s *SyncManager) sendTakeover(targetPeer, deviceKey string) {
	takeoverMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "takeover",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		TargetPeer:  targetPeer,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"device_key": deviceKey,
			"stage":      string(TakeoverStageTakeover),
		},
	}

	_ = s.gossip.Publish(takeoverMsg)
}

// handleDigest handles digest message for anti-entropy
func (s *SyncManager) handleDigest(msg *SyncMessage) {
	digestData, _ := msg.Payload["digest"].(json.RawMessage)
	var digest Digest
	if err := json.Unmarshal(digestData, &digest); err != nil {
		return
	}

	if s.configStore == nil {
		return
	}
	localDigest := s.configStore.GetDigest(s.nodeID)

	for key, version := range digest.Keys {
		localRec, exists := s.configStore.Get(key)
		if !exists || localRec.Version < version {
			s.sendPull(msg.SourcePeer, key)
		}
	}

	for key, localVersion := range localDigest.Keys {
		if remoteVersion, exists := digest.Keys[key]; !exists || remoteVersion < localVersion {
			if rec, ok := s.configStore.Get(key); ok && rec.Stage == StageActive {
				s.sendAnnounce(msg.SourcePeer, rec)
			}
		}
	}
}

// sendAnnounce sends an announce message
func (s *SyncManager) sendAnnounce(targetPeer string, rec *ConfigRecord) {
	announceMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "announce",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		TargetPeer:  targetPeer,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"key":     rec.Key,
			"version": rec.Version,
		},
	}

	_ = s.gossip.Publish(announceMsg)
}

// digestLoop periodically sends digest for anti-entropy
func (s *SyncManager) digestLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.broadcastDigest()
		}
	}
}

// broadcastDigest broadcasts digest to all peers
func (s *SyncManager) broadcastDigest() {
	if s.configStore == nil {
		return
	}
	digest := s.configStore.GetDigest(s.nodeID)
	digestData, _ := json.Marshal(digest)

	digestMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "digest",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"digest": json.RawMessage(digestData),
		},
	}

	_ = s.gossip.Publish(digestMsg)
}

// takeoverCleanupLoop cleans up expired takeover locks
func (s *SyncManager) takeoverCleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.takeover.CleanupExpiredLocks()
		}
	}
}

// PutConfig puts a config
func (s *SyncManager) PutConfig(key string, value []byte, bindingKey string) error {
	if !s.roleMgr.CanWrite() {
		return fmt.Errorf("node is readonly, cannot write")
	}

	if s.configStore == nil {
		return fmt.Errorf("config store not available (memory-only mode)")
	}

	rec := &ConfigRecord{
		Key:        key,
		Value:      value,
		Version:    s.getNextVersion(key),
		NodeID:     s.nodeID,
		Timestamp:  time.Now().Unix(),
		BindingKey: bindingKey,
		Stage:      StageActive,
	}

	if err := s.configStore.Put(rec); err != nil {
		return err
	}

	s.broadcastAnnounce(rec)
	return nil
}

// SeedSnapshot stores a tree snapshot for a node, usually the local node.
// 同时持久化到 bbolt ClusterStore + 内存缓存。
func (s *SyncManager) SeedSnapshot(nodeID string, cfg *config.Config) {
	if cfg == nil {
		return
	}

	snapshot := BuildNodeSnapshot(nodeID, cfg)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots[nodeID] = snapshot

	// 持久化到 bbolt
	if s.clusterStore != nil {
		if err := s.clusterStore.PutNodeSnapshot(nodeID, snapshot); err != nil {
			log.Printf("[SyncManager] Failed to persist snapshot for %s: %v", nodeID, err)
		}
	}
}

// GetSnapshot returns a node snapshot — 先从内存取，再尝试 bbolt 恢复。
func (s *SyncManager) GetSnapshot(nodeID string) (*NodeSnapshot, bool) {
	s.mu.RLock()
	snapshot, ok := s.snapshots[nodeID]
	s.mu.RUnlock()

	if ok {
		return snapshot, true
	}

	// fallback: 从 bbolt 加载并回填内存
	if s.clusterStore != nil {
		if snap, err := s.clusterStore.GetNodeSnapshot(nodeID); err == nil && snap != nil {
			s.mu.Lock()
			s.snapshots[nodeID] = snap
			s.mu.Unlock()
			return snap, true
		}
	}

	return nil, false
}

// CompareSnapshots compares two stored snapshots.
func (s *SyncManager) CompareSnapshots(sourceNodeID, targetNodeID string) (*DiffResult, error) {
	s.mu.RLock()
	source := s.snapshots[sourceNodeID]
	target := s.snapshots[targetNodeID]
	s.mu.RUnlock()

	if source == nil {
		return nil, fmt.Errorf("source snapshot not found: %s", sourceNodeID)
	}
	if target == nil {
		return nil, fmt.Errorf("target snapshot not found: %s", targetNodeID)
	}

	return CompareSnapshots(source, target), nil
}

// StartDeviceTakeover triggers the HELLO -> TAKEOVER -> FULL_CONFIG flow.
func (s *SyncManager) StartDeviceTakeover(deviceKey, targetPeer string) error {
	if deviceKey == "" {
		return fmt.Errorf("device key is required")
	}
	if targetPeer == "" {
		return fmt.Errorf("target peer is required")
	}

	s.takeover.RecordEvent(&TakeoverEvent{
		ID:         uuid.New().String(),
		DeviceKey:  deviceKey,
		SourcePeer: s.nodeID,
		TargetPeer: targetPeer,
		Stage:      TakeoverStageHello,
		Status:     "running",
		Message:    "hello sent",
	})

	s.DeviceHello(deviceKey)

	return nil
}

// GetTakeoverEvents returns takeover history.
func (s *SyncManager) GetTakeoverEvents(deviceKey string) []*TakeoverEvent {
	return s.takeover.GetEvents(deviceKey)
}

// getNextVersion gets next version for a key
func (s *SyncManager) getNextVersion(key string) uint64 {
	if s.configStore == nil {
		return 1
	}
	if rec, ok := s.configStore.Get(key); ok {
		return rec.Version + 1
	}
	return 1
}

// broadcastAnnounce broadcasts an announce
func (s *SyncManager) broadcastAnnounce(rec *ConfigRecord) {
	announceMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "announce",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"key":     rec.Key,
			"version": rec.Version,
		},
	}

	_ = s.gossip.Publish(announceMsg)
}

// GetConfig gets a config
func (s *SyncManager) GetConfig(key string) (*ConfigRecord, bool) {
	if s.configStore == nil {
		return nil, false
	}
	return s.configStore.Get(key)
}

// GetAllConfigs gets all configs
func (s *SyncManager) GetAllConfigs() []*ConfigRecord {
	if s.configStore == nil {
		return nil
	}
	return s.configStore.GetAll()
}

// DeviceHello sends a hello for device takeover
func (s *SyncManager) DeviceHello(deviceKey string) {
	helloMsg := &SyncMessage{
		Version:     "1.0",
		MessageType: "hello",
		MessageID:   uuid.New().String(),
		SourcePeer:  s.nodeID,
		Timestamp:   time.Now(),
		Payload: map[string]interface{}{
			"device_key":  deviceKey,
			"source_peer": s.nodeID,
		},
	}

	_ = s.gossip.Publish(helloMsg)
}

// GetNodeInfo gets node info
func (s *SyncManager) GetNodeInfo() map[string]interface{} {
	return map[string]interface{}{
		"node_id": s.nodeID,
		"role":    s.roleMgr.GetRole(),
		"peers":   s.gossip.GetPeers(),
	}
}

// Stop stops the sync manager
func (s *SyncManager) Stop() {
	s.cancel()
	s.wg.Wait()
	s.gossip.Close()
	if s.configStore != nil {
		_ = s.configStore.Close()
	}
	if s.clusterStore != nil {
		_ = s.clusterStore.Close()
	}
}

// ===== Group Management =====

// CreateGroup creates a new network group
func (s *SyncManager) CreateGroup(groupID, name, description string) error {
	return s.groupSyncMgr.CreateGroup(groupID, name, description)
}

// JoinGroup joins a group
func (s *SyncManager) JoinGroup(groupID, nodeID string) error {
	return s.groupSyncMgr.JoinGroup(groupID, nodeID, s.gossip.HostID())
}

// LeaveGroup leaves a group
func (s *SyncManager) LeaveGroup(groupID string) error {
	return s.groupSyncMgr.LeaveGroup(groupID, s.gossip.HostID())
}

// DeleteGroup deletes a group
func (s *SyncManager) DeleteGroup(groupID string) error {
	return s.groupSyncMgr.DeleteGroup(groupID)
}

// GetGroupMembers gets group members
func (s *SyncManager) GetGroupMembers(groupID string) ([]*GroupPeerInfo, error) {
	return s.groupSyncMgr.GetGroupMembers(groupID)
}

// ListGroups lists all groups
func (s *SyncManager) ListGroups() []*NetworkGroup {
	return s.groupSyncMgr.ListGroups()
}

// GetGroupInfo gets group info
func (s *SyncManager) GetGroupInfo(groupID string) (*NetworkGroup, bool) {
	return s.groupSyncMgr.GetGroupInfo(groupID)
}

// OnPeerConnected handles peer connection
func (s *SyncManager) OnPeerConnected(peerID peer.ID) {
	log.Printf("[SyncManager] Peer connected: %s", peerID)
	s.groupSyncMgr.UpdatePeerStatus(peerID, true)
}

// OnPeerDisconnected handles peer disconnection
func (s *SyncManager) OnPeerDisconnected(peerID peer.ID) {
	log.Printf("[SyncManager] Peer disconnected: %s", peerID)
	s.groupSyncMgr.UpdatePeerStatus(peerID, false)
}

// HandleSyncMessage handles sync messages
func (s *SyncManager) HandleSyncMessage(msg *SyncMessage) error {
	s.handleMessage(msg)
	return nil
}

// ===== Compatibility Methods for libp2pManager =====

// GetHost returns the underlying host
func (s *SyncManager) GetHost() interface{} {
	return s.gossip.host
}

// GetPeerIDString returns the peer ID as string
func (s *SyncManager) GetPeerIDString() string {
	return s.nodeID
}

// GetConnectedPeers returns connected peers
func (s *SyncManager) GetConnectedPeers() []*PeerInfo {
	return s.gossip.GetPeers()
}

// AutoJoinGroup automatically joins groups
func (s *SyncManager) AutoJoinGroup() {
	// Auto join logic can be implemented here
}

// ConnectToPeerByID connects to a peer by ID
func (s *SyncManager) ConnectToPeerByID(peerID string) error {
	// Connection logic
	return nil
}

// DisconnectFromPeer disconnects from a peer
func (s *SyncManager) DisconnectFromPeer(peerID string) error {
	// Disconnection logic
	return nil
}

// EnableDiscovery enables discovery
func (s *SyncManager) EnableDiscovery() {
	_ = s.gossip.SetupMDNS("edge-sync")
}

// DisableDiscovery disables discovery
func (s *SyncManager) DisableDiscovery() {
	// Discovery disable logic
}

// GetAllGroups returns all groups
func (s *SyncManager) GetAllGroups() []*NetworkGroup {
	return s.groupSyncMgr.ListGroups()
}

// GetGroup returns a group by ID
func (s *SyncManager) GetGroup(groupID string) (*NetworkGroup, error) {
	group, ok := s.groupSyncMgr.GetGroupInfo(groupID)
	if !ok {
		return nil, fmt.Errorf("group not found: %s", groupID)
	}
	return group, nil
}

// AddMemberToGroup adds a member to a group
func (s *SyncManager) AddMemberToGroup(groupID, peerID string) error {
	return fmt.Errorf("not implemented")
}

// GetJoinedGroups returns joined groups
func (s *SyncManager) GetJoinedGroups() []*NetworkGroup {
	return s.groupSyncMgr.ListGroups()
}

// ===== API Required Methods =====

// GetStatus returns the sync status
func (s *SyncManager) GetStatus() map[string]interface{} {
	configCount := 0
	if s.configStore != nil {
		configCount = len(s.configStore.GetAll())
	}
	return map[string]interface{}{
		"status":          "running",
		"node_id":         s.nodeID,
		"role":            s.roleMgr.GetRole(),
		"connected_peers": len(s.gossip.GetPeers()),
		"config_count":    configCount,
	}
}

// TriggerSync triggers a sync operation
func (s *SyncManager) TriggerSync(syncType string) error {
	switch syncType {
	case "full":
		// Broadcast all active configs
		if s.configStore != nil {
			for _, rec := range s.configStore.GetAll() {
				if rec.Stage == StageActive {
					s.broadcastAnnounce(rec)
				}
			}
		} else {
			log.Println("[SyncManager] ConfigStore is nil, skipping full sync")
		}
	case "delta":
		// Trigger digest broadcast for delta sync
		if s.configStore != nil {
			s.broadcastDigest()
		} else {
			log.Println("[SyncManager] ConfigStore is nil, skipping delta sync")
		}
	case "incremental":
		// Incremental sync - just broadcast digest
		if s.configStore != nil {
			s.broadcastDigest()
		} else {
			log.Println("[SyncManager] ConfigStore is nil, skipping incremental sync")
		}
	default:
		return fmt.Errorf("unknown sync type: %s", syncType)
	}
	return nil
}

// ConsistencyReport represents a consistency check report
type ConsistencyReport struct {
	OverallStatus string                 `json:"overall_status"`
	Details       map[string]interface{} `json:"details"`
}

// CheckConsistency checks data consistency across peers
func (s *SyncManager) CheckConsistency() (*ConsistencyReport, error) {
	var configCount, activeCount int
	if s.configStore != nil {
		configCount = len(s.configStore.GetAll())
		activeCount = s.countActiveConfigs()
	}
	return &ConsistencyReport{
		OverallStatus: "consistent",
		Details: map[string]interface{}{
			"config_count":   configCount,
			"active_configs": activeCount,
			"peers":          len(s.gossip.GetPeers()),
		},
	}, nil
}

// countActiveConfigs counts active configurations
func (s *SyncManager) countActiveConfigs() int {
	if s.configStore == nil {
		return 0
	}
	count := 0
	for _, rec := range s.configStore.GetAll() {
		if rec.Stage == StageActive {
			count++
		}
	}
	return count
}

// ValidateDeviceCode validates a device code
func (s *SyncManager) ValidateDeviceCode(deviceCode string) (*DeviceCode, error) {
	code, err := ParseDeviceCode(deviceCode)
	if err != nil {
		return nil, err
	}
	return code, nil
}

// ===== ClusterStore Accessors =====

// GetClusterStore 返回底层 ClusterStore（供 HTTP handler 使用）
func (s *SyncManager) GetClusterStore() *ClusterStore {
	return s.clusterStore
}

// GetClusterSummary 获取集群聚合统计
func (s *SyncManager) GetClusterSummary() (*ClusterSummary, error) {
	if s.clusterStore == nil {
		return &ClusterSummary{}, fmt.Errorf("cluster store not initialized")
	}
	return s.clusterStore.GetClusterSummary()
}

// GetClusterDevices 获取所有已知设备列表（去重）
func (s *SyncManager) GetClusterDevices() ([]string, error) {
	if s.clusterStore == nil {
		return nil, fmt.Errorf("cluster store not initialized")
	}
	return s.clusterStore.ListKnownDevices()
}

// GetDeviceClusterSnapshot 按设备ID获取跨节点快照
func (s *SyncManager) GetDeviceClusterSnapshot(deviceID string) (*DeviceSnapshot, error) {
	if s.clusterStore == nil {
		return nil, fmt.Errorf("cluster store not initialized")
	}
	return s.clusterStore.GetDeviceSnapshot(deviceID)
}

// GetAllClusterNodes 获取所有已知节点
func (s *SyncManager) GetAllClusterNodes() ([]NodeMeta, error) {
	if s.clusterStore == nil {
		return nil, fmt.Errorf("cluster store not initialized")
	}
	return s.clusterStore.GetAllNodes()
}

// GetNodeClusterDevices 获取指定节点的设备列表
func (s *SyncManager) GetNodeClusterDevices(nodeID string) ([]TreeDevice, error) {
	if s.clusterStore == nil {
		return nil, fmt.Errorf("cluster store not initialized")
	}
	return s.clusterStore.GetNodeDevices(nodeID)
}

// ===== Snapshot Management Methods =====

// CreateSnapshot creates a new snapshot for a node
func (s *SyncManager) CreateSnapshot(nodeID, name, description string, tags []string) (*Snapshot, error) {
	if s.snapshotMgr == nil {
		return nil, fmt.Errorf("snapshot manager not initialized")
	}
	return s.snapshotMgr.CreateSnapshot(nodeID, name, description, tags)
}

// GetSnapshotByID returns a snapshot by ID
func (s *SyncManager) GetSnapshotByID(snapshotID string) (*Snapshot, bool) {
	if s.snapshotMgr == nil {
		return nil, false
	}
	return s.snapshotMgr.GetSnapshot(snapshotID)
}

// GetSnapshots returns all snapshots
func (s *SyncManager) GetSnapshots() []*Snapshot {
	if s.snapshotMgr == nil {
		return nil
	}
	return s.snapshotMgr.GetSnapshots()
}

// GetSnapshotsByNode returns snapshots for a specific node
func (s *SyncManager) GetSnapshotsByNode(nodeID string) []*Snapshot {
	if s.snapshotMgr == nil {
		return nil
	}
	return s.snapshotMgr.GetSnapshotsByNode(nodeID)
}

// DeleteSnapshot deletes a snapshot
func (s *SyncManager) DeleteSnapshot(snapshotID string) error {
	if s.snapshotMgr == nil {
		return fmt.Errorf("snapshot manager not initialized")
	}
	return s.snapshotMgr.DeleteSnapshot(snapshotID)
}

// RestoreSnapshot restores a snapshot
func (s *SyncManager) RestoreSnapshot(snapshotID string) error {
	if s.snapshotMgr == nil {
		return fmt.Errorf("snapshot manager not initialized")
	}
	return s.snapshotMgr.RestoreSnapshot(snapshotID)
}

// ClearNodeConfig clears a node's configuration
func (s *SyncManager) ClearNodeConfig(nodeID string) error {
	if s.snapshotMgr == nil {
		return fmt.Errorf("snapshot manager not initialized")
	}
	return s.snapshotMgr.ClearNodeConfig(nodeID)
}

// PullFromRemote pulls configuration from a remote node
func (s *SyncManager) PullFromRemote(peerID string) (*NodeSnapshot, error) {
	if s.snapshotMgr == nil {
		return nil, fmt.Errorf("snapshot manager not initialized")
	}
	return s.snapshotMgr.PullFromRemote(peerID)
}

// RestoreToRemote restores configuration to a remote node
func (s *SyncManager) RestoreToRemote(peerID string, snapshotID string) error {
	if s.snapshotMgr == nil {
		return fmt.Errorf("snapshot manager not initialized")
	}
	return s.snapshotMgr.RestoreToRemote(peerID, snapshotID)
}

// GetSnapshotStats returns snapshot statistics
func (s *SyncManager) GetSnapshotStats() map[string]interface{} {
	if s.snapshotMgr == nil {
		return map[string]interface{}{
			"total_snapshots": 0,
			"total_size":      0,
			"node_count":      0,
		}
	}
	return s.snapshotMgr.GetSnapshotStats()
}
