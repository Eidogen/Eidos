// Package ha 高可用和故障转移实现
// 基于 Redis 的 Leader 选举机制
package ha

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	// ErrNotLeader 非 Leader 错误
	ErrNotLeader = errors.New("not the leader")
	// ErrLeaderLost Leader 丢失
	ErrLeaderLost = errors.New("leader lost")
)

// LeaderState Leader 状态
type LeaderState int32

const (
	// StateFollower 跟随者状态
	StateFollower LeaderState = iota
	// StateCandidate 候选者状态
	StateCandidate
	// StateLeader Leader 状态
	StateLeader
	// StateStopped 已停止
	StateStopped
)

func (s LeaderState) String() string {
	switch s {
	case StateFollower:
		return "follower"
	case StateCandidate:
		return "candidate"
	case StateLeader:
		return "leader"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// LeaderChangeCallback Leader 变更回调
type LeaderChangeCallback func(isLeader bool, oldState, newState LeaderState)

// LeaderElectionConfig Leader 选举配置
type LeaderElectionConfig struct {
	// Redis 客户端
	RedisClient redis.UniversalClient
	// 选举 key 前缀
	KeyPrefix string
	// 节点 ID
	NodeID string
	// 租约时间
	LeaseDuration time.Duration
	// 续租间隔
	RenewInterval time.Duration
	// 重试间隔
	RetryInterval time.Duration
	// Leader 变更回调
	OnLeaderChange LeaderChangeCallback
}

// LeaderElection Leader 选举器
type LeaderElection struct {
	config   *LeaderElectionConfig
	redis    redis.UniversalClient
	state    atomic.Int32
	leaderID atomic.Value // 当前 Leader ID

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	wg     sync.WaitGroup

	// 统计
	electionCount    int64
	renewCount       int64
	failCount        int64
	lastElectionTime time.Time
	lastRenewTime    time.Time
	lastError        error
	lastErrorAt      time.Time
}

// NewLeaderElection 创建 Leader 选举器
func NewLeaderElection(cfg *LeaderElectionConfig) (*LeaderElection, error) {
	if cfg.RedisClient == nil {
		return nil, errors.New("redis client is required")
	}
	if cfg.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "eidos:matching:leader"
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 5 * time.Second
	}
	if cfg.RenewInterval <= 0 {
		cfg.RenewInterval = cfg.LeaseDuration / 3
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	le := &LeaderElection{
		config: cfg,
		redis:  cfg.RedisClient,
		ctx:    ctx,
		cancel: cancel,
	}
	le.state.Store(int32(StateFollower))
	le.leaderID.Store("")

	return le, nil
}

// Start 启动 Leader 选举
func (le *LeaderElection) Start() error {
	le.wg.Add(1)
	go le.electionLoop()

	zap.L().Info("leader election started",
		zap.String("node_id", le.config.NodeID),
		zap.Duration("lease_duration", le.config.LeaseDuration),
		zap.Duration("renew_interval", le.config.RenewInterval))

	return nil
}

// Stop 停止 Leader 选举
func (le *LeaderElection) Stop() {
	oldState := LeaderState(le.state.Swap(int32(StateStopped)))
	le.cancel()
	le.wg.Wait()

	// 如果当前是 Leader，释放锁
	if oldState == StateLeader {
		le.releaseLock()
	}

	// 触发回调
	if le.config.OnLeaderChange != nil && oldState == StateLeader {
		le.config.OnLeaderChange(false, oldState, StateStopped)
	}

	zap.L().Info("leader election stopped",
		zap.String("node_id", le.config.NodeID),
		zap.String("old_state", oldState.String()))
}

// electionLoop 选举循环
func (le *LeaderElection) electionLoop() {
	defer le.wg.Done()

	ticker := time.NewTicker(le.config.RenewInterval)
	defer ticker.Stop()

	// 首次立即尝试获取 Leader
	le.tryAcquireOrRenew()

	for {
		select {
		case <-le.ctx.Done():
			return
		case <-ticker.C:
			le.tryAcquireOrRenew()
		}
	}
}

// tryAcquireOrRenew 尝试获取或续租 Leader
func (le *LeaderElection) tryAcquireOrRenew() {
	currentState := LeaderState(le.state.Load())
	if currentState == StateStopped {
		return
	}

	var newState LeaderState
	var err error

	if currentState == StateLeader {
		// 续租
		err = le.renewLock()
		if err != nil {
			// 续租失败，降级为 Follower
			newState = StateFollower
			le.recordError(err)
			zap.L().Warn("renew leader lock failed",
				zap.String("node_id", le.config.NodeID),
				zap.Error(err))
		} else {
			newState = StateLeader
			le.renewCount++
			le.lastRenewTime = time.Now()
		}
	} else {
		// 尝试获取 Leader
		le.state.Store(int32(StateCandidate))
		acquired, currentLeader, err := le.acquireLock()
		if err != nil {
			newState = StateFollower
			le.recordError(err)
			zap.L().Warn("acquire leader lock failed",
				zap.String("node_id", le.config.NodeID),
				zap.Error(err))
		} else if acquired {
			newState = StateLeader
			le.electionCount++
			le.lastElectionTime = time.Now()
			zap.L().Info("became leader",
				zap.String("node_id", le.config.NodeID))
		} else {
			newState = StateFollower
			le.leaderID.Store(currentLeader)
		}
	}

	// 状态变更
	if newState != currentState {
		oldState := LeaderState(le.state.Swap(int32(newState)))
		if le.config.OnLeaderChange != nil {
			le.config.OnLeaderChange(newState == StateLeader, oldState, newState)
		}
		zap.L().Info("leader state changed",
			zap.String("node_id", le.config.NodeID),
			zap.String("old_state", oldState.String()),
			zap.String("new_state", newState.String()))
	}
}

// acquireLock 尝试获取 Leader 锁
// 返回: (是否获取成功, 当前 Leader ID, 错误)
func (le *LeaderElection) acquireLock() (bool, string, error) {
	key := le.lockKey()
	expiration := le.config.LeaseDuration

	// 使用 SETNX 原子操作
	success, err := le.redis.SetNX(le.ctx, key, le.config.NodeID, expiration).Result()
	if err != nil {
		return false, "", fmt.Errorf("redis setnx: %w", err)
	}

	if success {
		le.leaderID.Store(le.config.NodeID)
		return true, le.config.NodeID, nil
	}

	// 获取当前 Leader
	currentLeader, err := le.redis.Get(le.ctx, key).Result()
	if err != nil && err != redis.Nil {
		return false, "", fmt.Errorf("redis get: %w", err)
	}

	return false, currentLeader, nil
}

// renewLock 续租 Leader 锁
func (le *LeaderElection) renewLock() error {
	key := le.lockKey()
	expiration := le.config.LeaseDuration

	// 使用 Lua 脚本确保只有当前 Leader 能续租
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)

	result, err := script.Run(le.ctx, le.redis, []string{key},
		le.config.NodeID, expiration.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("renew script: %w", err)
	}

	if result == 0 {
		return ErrLeaderLost
	}

	return nil
}

// releaseLock 释放 Leader 锁
func (le *LeaderElection) releaseLock() {
	key := le.lockKey()

	// 使用 Lua 脚本确保只有当前 Leader 能释放
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := script.Run(ctx, le.redis, []string{key}, le.config.NodeID).Result(); err != nil {
		zap.L().Warn("release leader lock failed",
			zap.String("node_id", le.config.NodeID),
			zap.Error(err))
	} else {
		zap.L().Info("leader lock released",
			zap.String("node_id", le.config.NodeID))
	}
}

// lockKey 获取锁 key
func (le *LeaderElection) lockKey() string {
	return le.config.KeyPrefix + ":lock"
}

// recordError 记录错误
func (le *LeaderElection) recordError(err error) {
	le.mu.Lock()
	defer le.mu.Unlock()
	le.failCount++
	le.lastError = err
	le.lastErrorAt = time.Now()
}

// IsLeader 是否是 Leader
func (le *LeaderElection) IsLeader() bool {
	return LeaderState(le.state.Load()) == StateLeader
}

// State 获取当前状态
func (le *LeaderElection) State() LeaderState {
	return LeaderState(le.state.Load())
}

// LeaderID 获取当前 Leader ID
func (le *LeaderElection) LeaderID() string {
	id := le.leaderID.Load()
	if id == nil {
		return ""
	}
	return id.(string)
}

// GetStats 获取统计信息
func (le *LeaderElection) GetStats() LeaderElectionStats {
	le.mu.RLock()
	defer le.mu.RUnlock()

	return LeaderElectionStats{
		NodeID:           le.config.NodeID,
		State:            LeaderState(le.state.Load()).String(),
		LeaderID:         le.LeaderID(),
		ElectionCount:    le.electionCount,
		RenewCount:       le.renewCount,
		FailCount:        le.failCount,
		LastElectionTime: le.lastElectionTime,
		LastRenewTime:    le.lastRenewTime,
		LastError:        le.lastError,
		LastErrorAt:      le.lastErrorAt,
	}
}

// LeaderElectionStats 统计信息
type LeaderElectionStats struct {
	NodeID           string        `json:"node_id"`
	State            string        `json:"state"`
	LeaderID         string        `json:"leader_id"`
	ElectionCount    int64         `json:"election_count"`
	RenewCount       int64         `json:"renew_count"`
	FailCount        int64         `json:"fail_count"`
	LastElectionTime time.Time     `json:"last_election_time,omitempty"`
	LastRenewTime    time.Time     `json:"last_renew_time,omitempty"`
	LastError        error         `json:"last_error,omitempty"`
	LastErrorAt      time.Time     `json:"last_error_at,omitempty"`
}
