// Package ha Standby 模式管理
package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// StandbyMode Standby 模式
type StandbyMode int32

const (
	// ModeActive 主动模式 (处理请求)
	ModeActive StandbyMode = iota
	// ModeStandby 备用模式 (只同步状态)
	ModeStandby
	// ModeReadOnly 只读模式 (可查询，不处理订单)
	ModeReadOnly
)

func (m StandbyMode) String() string {
	switch m {
	case ModeActive:
		return "active"
	case ModeStandby:
		return "standby"
	case ModeReadOnly:
		return "readonly"
	default:
		return "unknown"
	}
}

// StateChangeCallback 状态变更回调
type StateChangeCallback func(oldMode, newMode StandbyMode)

// StandbyManagerConfig 配置
type StandbyManagerConfig struct {
	// Redis 客户端
	RedisClient redis.UniversalClient
	// 节点 ID
	NodeID string
	// Key 前缀
	KeyPrefix string
	// 状态同步间隔
	SyncInterval time.Duration
	// 心跳间隔
	HeartbeatInterval time.Duration
	// 故障转移超时
	FailoverTimeout time.Duration
	// 状态变更回调
	OnStateChange StateChangeCallback
}

// NodeState 节点状态
type NodeState struct {
	NodeID       string      `json:"node_id"`
	Mode         StandbyMode `json:"mode"`
	LastHeartbeat time.Time   `json:"last_heartbeat"`
	InputSequence int64       `json:"input_sequence"`
	Markets      []string    `json:"markets"`
	Version      int64       `json:"version"`
}

// StandbyManager Standby 管理器
type StandbyManager struct {
	config         *StandbyManagerConfig
	redis          redis.UniversalClient
	mode           atomic.Int32
	leaderElection *LeaderElection
	nodeState      *NodeState

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	wg     sync.WaitGroup

	// 状态同步
	localSequences  map[string]int64 // market -> sequence
	remoteSequences map[string]int64 // market -> remote leader sequence

	// 统计
	syncCount      int64
	heartbeatCount int64
	failoverCount  int64
}

// NewStandbyManager 创建 Standby 管理器
func NewStandbyManager(cfg *StandbyManagerConfig, le *LeaderElection) (*StandbyManager, error) {
	if cfg.RedisClient == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "eidos:matching:ha"
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 100 * time.Millisecond
	}
	if cfg.FailoverTimeout <= 0 {
		cfg.FailoverTimeout = 500 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(context.Background())

	sm := &StandbyManager{
		config:          cfg,
		redis:           cfg.RedisClient,
		leaderElection:  le,
		ctx:             ctx,
		cancel:          cancel,
		localSequences:  make(map[string]int64),
		remoteSequences: make(map[string]int64),
		nodeState: &NodeState{
			NodeID:  cfg.NodeID,
			Mode:    ModeStandby,
			Markets: make([]string, 0),
		},
	}
	sm.mode.Store(int32(ModeStandby))

	return sm, nil
}

// Start 启动 Standby 管理器
func (sm *StandbyManager) Start(markets []string) error {
	sm.mu.Lock()
	sm.nodeState.Markets = markets
	sm.mu.Unlock()

	// 启动心跳
	sm.wg.Add(1)
	go sm.heartbeatLoop()

	// 启动状态同步
	sm.wg.Add(1)
	go sm.syncLoop()

	// 监听 Leader 变更
	sm.leaderElection.config.OnLeaderChange = sm.handleLeaderChange

	slog.Info("standby manager started",
		"node_id", sm.config.NodeID,
		"markets", len(markets))

	return nil
}

// Stop 停止 Standby 管理器
func (sm *StandbyManager) Stop() {
	sm.cancel()
	sm.wg.Wait()

	slog.Info("standby manager stopped",
		"node_id", sm.config.NodeID)
}

// handleLeaderChange 处理 Leader 变更
func (sm *StandbyManager) handleLeaderChange(isLeader bool, oldState, newState LeaderState) {
	var newMode StandbyMode
	if isLeader {
		newMode = ModeActive
	} else {
		newMode = ModeStandby
	}

	oldMode := StandbyMode(sm.mode.Swap(int32(newMode)))

	// 更新节点状态
	sm.mu.Lock()
	sm.nodeState.Mode = newMode
	sm.mu.Unlock()

	if oldMode != newMode {
		if sm.config.OnStateChange != nil {
			sm.config.OnStateChange(oldMode, newMode)
		}

		if newMode == ModeActive {
			sm.failoverCount++
			slog.Info("failover completed, now active",
				"node_id", sm.config.NodeID)
		} else {
			slog.Info("switched to standby mode",
				"node_id", sm.config.NodeID)
		}
	}
}

// heartbeatLoop 心跳循环
func (sm *StandbyManager) heartbeatLoop() {
	defer sm.wg.Done()

	ticker := time.NewTicker(sm.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.sendHeartbeat()
		}
	}
}

// sendHeartbeat 发送心跳
func (sm *StandbyManager) sendHeartbeat() {
	sm.mu.RLock()
	state := *sm.nodeState
	state.LastHeartbeat = time.Now()
	state.Version++
	sm.mu.RUnlock()

	data, err := json.Marshal(state)
	if err != nil {
		slog.Warn("marshal node state failed", "error", err)
		return
	}

	key := sm.nodeKey()
	expiration := sm.config.FailoverTimeout * 3

	if err := sm.redis.Set(sm.ctx, key, data, expiration).Err(); err != nil {
		slog.Warn("send heartbeat failed", "error", err)
		return
	}

	sm.heartbeatCount++
}

// syncLoop 状态同步循环
func (sm *StandbyManager) syncLoop() {
	defer sm.wg.Done()

	ticker := time.NewTicker(sm.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.syncState()
		}
	}
}

// syncState 同步状态
func (sm *StandbyManager) syncState() {
	// Standby 模式下同步 Leader 的序列号
	if sm.Mode() == ModeStandby {
		leaderID := sm.leaderElection.LeaderID()
		if leaderID == "" || leaderID == sm.config.NodeID {
			return
		}

		// 获取 Leader 的状态
		leaderState, err := sm.getNodeState(leaderID)
		if err != nil {
			slog.Debug("get leader state failed",
				"leader_id", leaderID,
				"error", err)
			return
		}

		// 更新远程序列号
		sm.mu.Lock()
		sm.remoteSequences[leaderID] = leaderState.InputSequence
		sm.mu.Unlock()

		sm.syncCount++
	}
}

// getNodeState 获取节点状态
func (sm *StandbyManager) getNodeState(nodeID string) (*NodeState, error) {
	key := sm.config.KeyPrefix + ":node:" + nodeID

	data, err := sm.redis.Get(sm.ctx, key).Bytes()
	if err != nil {
		return nil, fmt.Errorf("get node state: %w", err)
	}

	var state NodeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal node state: %w", err)
	}

	return &state, nil
}

// UpdateSequence 更新本地序列号
func (sm *StandbyManager) UpdateSequence(market string, sequence int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.localSequences[market] = sequence
	if sequence > sm.nodeState.InputSequence {
		sm.nodeState.InputSequence = sequence
	}
}

// GetSequenceLag 获取序列号延迟
func (sm *StandbyManager) GetSequenceLag(market string) int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	local := sm.localSequences[market]
	remote := sm.remoteSequences[sm.leaderElection.LeaderID()]

	if remote > local {
		return remote - local
	}
	return 0
}

// Mode 获取当前模式
func (sm *StandbyManager) Mode() StandbyMode {
	return StandbyMode(sm.mode.Load())
}

// IsActive 是否处于活跃模式
func (sm *StandbyManager) IsActive() bool {
	return sm.Mode() == ModeActive
}

// CanProcessOrders 是否可以处理订单
func (sm *StandbyManager) CanProcessOrders() bool {
	return sm.Mode() == ModeActive
}

// CanReadOrderBook 是否可以读取订单簿
func (sm *StandbyManager) CanReadOrderBook() bool {
	mode := sm.Mode()
	return mode == ModeActive || mode == ModeReadOnly
}

// nodeKey 获取节点 key
func (sm *StandbyManager) nodeKey() string {
	return sm.config.KeyPrefix + ":node:" + sm.config.NodeID
}

// GetStats 获取统计信息
func (sm *StandbyManager) GetStats() StandbyManagerStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return StandbyManagerStats{
		NodeID:          sm.config.NodeID,
		Mode:            sm.Mode().String(),
		LeaderID:        sm.leaderElection.LeaderID(),
		IsLeader:        sm.leaderElection.IsLeader(),
		SyncCount:       sm.syncCount,
		HeartbeatCount:  sm.heartbeatCount,
		FailoverCount:   sm.failoverCount,
		LocalSequences:  sm.localSequences,
		RemoteSequences: sm.remoteSequences,
	}
}

// StandbyManagerStats 统计信息
type StandbyManagerStats struct {
	NodeID          string           `json:"node_id"`
	Mode            string           `json:"mode"`
	LeaderID        string           `json:"leader_id"`
	IsLeader        bool             `json:"is_leader"`
	SyncCount       int64            `json:"sync_count"`
	HeartbeatCount  int64            `json:"heartbeat_count"`
	FailoverCount   int64            `json:"failover_count"`
	LocalSequences  map[string]int64 `json:"local_sequences"`
	RemoteSequences map[string]int64 `json:"remote_sequences"`
}
