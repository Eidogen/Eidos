package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

var (
	// ErrClientClosed 客户端已关闭
	ErrClientClosed = errors.New("redis client is closed")
	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("invalid redis configuration")
	// ErrConnectionFailed 连接失败
	ErrConnectionFailed = errors.New("redis connection failed")
	// ErrHealthCheckFailed 健康检查失败
	ErrHealthCheckFailed = errors.New("redis health check failed")
)

// Mode Redis 模式
type Mode string

const (
	// ModeSingle 单机模式
	ModeSingle Mode = "single"
	// ModeSentinel 哨兵模式
	ModeSentinel Mode = "sentinel"
	// ModeCluster 集群模式
	ModeCluster Mode = "cluster"
)

// Config Redis 客户端配置
type Config struct {
	// Mode 运行模式: single, sentinel, cluster
	Mode Mode `yaml:"mode" json:"mode"`

	// Addresses Redis 地址列表
	// 单机模式: ["127.0.0.1:6379"]
	// 哨兵模式: ["sentinel1:26379", "sentinel2:26379"]
	// 集群模式: ["node1:6379", "node2:6379", "node3:6379"]
	Addresses []string `yaml:"addresses" json:"addresses"`

	// Password 密码
	Password string `yaml:"password" json:"password"`

	// DB 数据库编号 (仅单机和哨兵模式有效)
	DB int `yaml:"db" json:"db"`

	// MasterName 主节点名称 (仅哨兵模式有效)
	MasterName string `yaml:"master_name" json:"master_name"`

	// PoolSize 连接池大小
	PoolSize int `yaml:"pool_size" json:"pool_size"`

	// MinIdleConns 最小空闲连接数
	MinIdleConns int `yaml:"min_idle_conns" json:"min_idle_conns"`

	// MaxIdleConns 最大空闲连接数
	MaxIdleConns int `yaml:"max_idle_conns" json:"max_idle_conns"`

	// ConnMaxLifetime 连接最大生命周期
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`

	// ConnMaxIdleTime 连接最大空闲时间
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`

	// DialTimeout 连接超时
	DialTimeout time.Duration `yaml:"dial_timeout" json:"dial_timeout"`

	// ReadTimeout 读取超时
	ReadTimeout time.Duration `yaml:"read_timeout" json:"read_timeout"`

	// WriteTimeout 写入超时
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`

	// MaxRetries 最大重试次数
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// MinRetryBackoff 最小重试间隔
	MinRetryBackoff time.Duration `yaml:"min_retry_backoff" json:"min_retry_backoff"`

	// MaxRetryBackoff 最大重试间隔
	MaxRetryBackoff time.Duration `yaml:"max_retry_backoff" json:"max_retry_backoff"`

	// TLS 配置
	TLSConfig *tls.Config `yaml:"-" json:"-"`

	// EnableTLS 是否启用 TLS
	EnableTLS bool `yaml:"enable_tls" json:"enable_tls"`

	// HealthCheckInterval 健康检查间隔
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"`

	// RouteByLatency 集群模式下按延迟路由
	RouteByLatency bool `yaml:"route_by_latency" json:"route_by_latency"`

	// RouteRandomly 集群模式下随机路由
	RouteRandomly bool `yaml:"route_randomly" json:"route_randomly"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Mode:                ModeSingle,
		Addresses:          []string{"127.0.0.1:6379"},
		DB:                 0,
		PoolSize:           100,
		MinIdleConns:       10,
		MaxIdleConns:       50,
		ConnMaxLifetime:    30 * time.Minute,
		ConnMaxIdleTime:    5 * time.Minute,
		DialTimeout:        5 * time.Second,
		ReadTimeout:        3 * time.Second,
		WriteTimeout:       3 * time.Second,
		MaxRetries:         3,
		MinRetryBackoff:    8 * time.Millisecond,
		MaxRetryBackoff:    512 * time.Millisecond,
		HealthCheckInterval: 30 * time.Second,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if len(c.Addresses) == 0 {
		return fmt.Errorf("%w: addresses is empty", ErrInvalidConfig)
	}

	switch c.Mode {
	case ModeSingle, ModeSentinel, ModeCluster:
		// 有效模式
	default:
		return fmt.Errorf("%w: invalid mode %s", ErrInvalidConfig, c.Mode)
	}

	if c.Mode == ModeSentinel && c.MasterName == "" {
		return fmt.Errorf("%w: master_name is required for sentinel mode", ErrInvalidConfig)
	}

	if c.PoolSize <= 0 {
		c.PoolSize = 100
	}

	return nil
}

// Client Redis 客户端封装
type Client struct {
	config     *Config
	client     redis.UniversalClient
	scripts    *ScriptManager
	pubsub     *PubSubManager
	mu         sync.RWMutex
	closed     int32
	closeChan  chan struct{}
	healthOnce sync.Once
	metrics    *Metrics
}

// Metrics Redis 指标
type Metrics struct {
	// 连接池统计
	PoolHits     uint64
	PoolMisses   uint64
	PoolTimeouts uint64
	PoolSize     uint64
	IdleConns    uint64

	// 操作统计
	TotalCommands  uint64
	FailedCommands uint64

	// 健康状态
	LastHealthCheck time.Time
	IsHealthy       bool
}

// NewClient 创建 Redis 客户端
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	c := &Client{
		config:    cfg,
		closeChan: make(chan struct{}),
		metrics:   &Metrics{},
	}

	// 创建底层客户端
	if err := c.createClient(); err != nil {
		return nil, err
	}

	// 初始化脚本管理器
	c.scripts = NewScriptManager(c.client)

	// 初始化 PubSub 管理器
	c.pubsub = NewPubSubManager(c.client)

	// 启动健康检查
	if cfg.HealthCheckInterval > 0 {
		go c.startHealthCheck()
	}

	logger.Info("redis client initialized",
		zap.String("mode", string(cfg.Mode)),
		zap.Strings("addresses", cfg.Addresses),
		zap.Int("pool_size", cfg.PoolSize),
	)

	return c, nil
}

// createClient 创建底层 Redis 客户端
func (c *Client) createClient() error {
	var client redis.UniversalClient

	switch c.config.Mode {
	case ModeSingle:
		client = c.createSingleClient()
	case ModeSentinel:
		client = c.createSentinelClient()
	case ModeCluster:
		client = c.createClusterClient()
	default:
		return fmt.Errorf("%w: unsupported mode %s", ErrInvalidConfig, c.config.Mode)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), c.config.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	c.client = client
	return nil
}

// createSingleClient 创建单机客户端
func (c *Client) createSingleClient() redis.UniversalClient {
	opts := &redis.Options{
		Addr:            c.config.Addresses[0],
		Password:        c.config.Password,
		DB:              c.config.DB,
		PoolSize:        c.config.PoolSize,
		MinIdleConns:    c.config.MinIdleConns,
		MaxIdleConns:    c.config.MaxIdleConns,
		ConnMaxLifetime: c.config.ConnMaxLifetime,
		ConnMaxIdleTime: c.config.ConnMaxIdleTime,
		DialTimeout:     c.config.DialTimeout,
		ReadTimeout:     c.config.ReadTimeout,
		WriteTimeout:    c.config.WriteTimeout,
		MaxRetries:      c.config.MaxRetries,
		MinRetryBackoff: c.config.MinRetryBackoff,
		MaxRetryBackoff: c.config.MaxRetryBackoff,
	}

	if c.config.EnableTLS {
		opts.TLSConfig = c.config.TLSConfig
		if opts.TLSConfig == nil {
			opts.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}
	}

	return redis.NewClient(opts)
}

// createSentinelClient 创建哨兵客户端
func (c *Client) createSentinelClient() redis.UniversalClient {
	opts := &redis.FailoverOptions{
		MasterName:       c.config.MasterName,
		SentinelAddrs:    c.config.Addresses,
		SentinelPassword: c.config.Password,
		Password:         c.config.Password,
		DB:               c.config.DB,
		PoolSize:         c.config.PoolSize,
		MinIdleConns:     c.config.MinIdleConns,
		MaxIdleConns:     c.config.MaxIdleConns,
		ConnMaxLifetime:  c.config.ConnMaxLifetime,
		ConnMaxIdleTime:  c.config.ConnMaxIdleTime,
		DialTimeout:      c.config.DialTimeout,
		ReadTimeout:      c.config.ReadTimeout,
		WriteTimeout:     c.config.WriteTimeout,
		MaxRetries:       c.config.MaxRetries,
		MinRetryBackoff:  c.config.MinRetryBackoff,
		MaxRetryBackoff:  c.config.MaxRetryBackoff,
	}

	if c.config.EnableTLS {
		opts.TLSConfig = c.config.TLSConfig
		if opts.TLSConfig == nil {
			opts.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}
	}

	return redis.NewFailoverClient(opts)
}

// createClusterClient 创建集群客户端
func (c *Client) createClusterClient() redis.UniversalClient {
	opts := &redis.ClusterOptions{
		Addrs:           c.config.Addresses,
		Password:        c.config.Password,
		PoolSize:        c.config.PoolSize,
		MinIdleConns:    c.config.MinIdleConns,
		MaxIdleConns:    c.config.MaxIdleConns,
		ConnMaxLifetime: c.config.ConnMaxLifetime,
		ConnMaxIdleTime: c.config.ConnMaxIdleTime,
		DialTimeout:     c.config.DialTimeout,
		ReadTimeout:     c.config.ReadTimeout,
		WriteTimeout:    c.config.WriteTimeout,
		MaxRetries:      c.config.MaxRetries,
		MinRetryBackoff: c.config.MinRetryBackoff,
		MaxRetryBackoff: c.config.MaxRetryBackoff,
		RouteByLatency:  c.config.RouteByLatency,
		RouteRandomly:   c.config.RouteRandomly,
	}

	if c.config.EnableTLS {
		opts.TLSConfig = c.config.TLSConfig
		if opts.TLSConfig == nil {
			opts.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}
	}

	return redis.NewClusterClient(opts)
}

// startHealthCheck 启动健康检查
func (c *Client) startHealthCheck() {
	ticker := time.NewTicker(c.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeChan:
			return
		case <-ticker.C:
			c.doHealthCheck()
		}
	}
}

// doHealthCheck 执行健康检查
func (c *Client) doHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.client.Ping(ctx).Err()
	c.metrics.LastHealthCheck = time.Now()
	c.metrics.IsHealthy = err == nil

	if err != nil {
		logger.Warn("redis health check failed", zap.Error(err))
	}

	// 更新连接池统计
	c.updatePoolStats()
}

// updatePoolStats 更新连接池统计
func (c *Client) updatePoolStats() {
	stats := c.client.PoolStats()
	if stats != nil {
		atomic.StoreUint64(&c.metrics.PoolHits, uint64(stats.Hits))
		atomic.StoreUint64(&c.metrics.PoolMisses, uint64(stats.Misses))
		atomic.StoreUint64(&c.metrics.PoolTimeouts, uint64(stats.Timeouts))
		atomic.StoreUint64(&c.metrics.PoolSize, uint64(stats.TotalConns))
		atomic.StoreUint64(&c.metrics.IdleConns, uint64(stats.IdleConns))
	}
}

// GetMetrics 获取指标
func (c *Client) GetMetrics() *Metrics {
	c.updatePoolStats()
	return c.metrics
}

// Client 获取底层客户端
func (c *Client) Client() redis.UniversalClient {
	return c.client
}

// Scripts 获取脚本管理器
func (c *Client) Scripts() *ScriptManager {
	return c.scripts
}

// PubSub 获取 PubSub 管理器
func (c *Client) PubSub() *PubSubManager {
	return c.pubsub
}

// Pipeline 创建管道
func (c *Client) Pipeline() *Pipeline {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil
	}
	return NewPipeline(c.client.Pipeline())
}

// TxPipeline 创建事务管道
func (c *Client) TxPipeline() *Pipeline {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil
	}
	return NewPipeline(c.client.TxPipeline())
}

// Ping 检查连接
func (c *Client) Ping(ctx context.Context) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrClientClosed
	}
	return c.client.Ping(ctx).Err()
}

// IsHealthy 检查是否健康
func (c *Client) IsHealthy() bool {
	return c.metrics.IsHealthy
}

// Close 关闭客户端
func (c *Client) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return ErrClientClosed
	}

	close(c.closeChan)

	// 关闭 PubSub
	if c.pubsub != nil {
		c.pubsub.Close()
	}

	// 关闭底层客户端
	if err := c.client.Close(); err != nil {
		logger.Error("failed to close redis client", zap.Error(err))
		return err
	}

	logger.Info("redis client closed")
	return nil
}

// ===== 常用命令快捷方法 =====

// Get 获取值
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return "", ErrClientClosed
	}
	return c.client.Get(ctx, key).Result()
}

// Set 设置值
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrClientClosed
	}
	return c.client.Set(ctx, key, value, expiration).Err()
}

// SetNX 设置值 (仅当不存在时)
func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return false, ErrClientClosed
	}
	return c.client.SetNX(ctx, key, value, expiration).Result()
}

// Del 删除键
func (c *Client) Del(ctx context.Context, keys ...string) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.Del(ctx, keys...).Result()
}

// Exists 检查键是否存在
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.Exists(ctx, keys...).Result()
}

// Expire 设置过期时间
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return false, ErrClientClosed
	}
	return c.client.Expire(ctx, key, expiration).Result()
}

// TTL 获取剩余过期时间
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.TTL(ctx, key).Result()
}

// Incr 递增
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.Incr(ctx, key).Result()
}

// IncrBy 递增指定值
func (c *Client) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.IncrBy(ctx, key, value).Result()
}

// Decr 递减
func (c *Client) Decr(ctx context.Context, key string) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.Decr(ctx, key).Result()
}

// DecrBy 递减指定值
func (c *Client) DecrBy(ctx context.Context, key string, value int64) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.DecrBy(ctx, key, value).Result()
}

// HGet 获取哈希字段值
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return "", ErrClientClosed
	}
	return c.client.HGet(ctx, key, field).Result()
}

// HSet 设置哈希字段值
func (c *Client) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.HSet(ctx, key, values...).Result()
}

// HGetAll 获取所有哈希字段值
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrClientClosed
	}
	return c.client.HGetAll(ctx, key).Result()
}

// HDel 删除哈希字段
func (c *Client) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.HDel(ctx, key, fields...).Result()
}

// LPush 左侧推入列表
func (c *Client) LPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.LPush(ctx, key, values...).Result()
}

// RPush 右侧推入列表
func (c *Client) RPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.RPush(ctx, key, values...).Result()
}

// LPop 左侧弹出列表
func (c *Client) LPop(ctx context.Context, key string) (string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return "", ErrClientClosed
	}
	return c.client.LPop(ctx, key).Result()
}

// RPop 右侧弹出列表
func (c *Client) RPop(ctx context.Context, key string) (string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return "", ErrClientClosed
	}
	return c.client.RPop(ctx, key).Result()
}

// LRange 获取列表范围
func (c *Client) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrClientClosed
	}
	return c.client.LRange(ctx, key, start, stop).Result()
}

// LLen 获取列表长度
func (c *Client) LLen(ctx context.Context, key string) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.LLen(ctx, key).Result()
}

// SAdd 添加集合成员
func (c *Client) SAdd(ctx context.Context, key string, members ...interface{}) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.SAdd(ctx, key, members...).Result()
}

// SMembers 获取集合所有成员
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrClientClosed
	}
	return c.client.SMembers(ctx, key).Result()
}

// SIsMember 检查是否是集合成员
func (c *Client) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return false, ErrClientClosed
	}
	return c.client.SIsMember(ctx, key, member).Result()
}

// SRem 移除集合成员
func (c *Client) SRem(ctx context.Context, key string, members ...interface{}) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.SRem(ctx, key, members...).Result()
}

// ZAdd 添加有序集合成员
func (c *Client) ZAdd(ctx context.Context, key string, members ...redis.Z) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.ZAdd(ctx, key, members...).Result()
}

// ZRange 获取有序集合范围
func (c *Client) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrClientClosed
	}
	return c.client.ZRange(ctx, key, start, stop).Result()
}

// ZRangeWithScores 获取有序集合范围(带分数)
func (c *Client) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrClientClosed
	}
	return c.client.ZRangeWithScores(ctx, key, start, stop).Result()
}

// ZRem 移除有序集合成员
func (c *Client) ZRem(ctx context.Context, key string, members ...interface{}) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.ZRem(ctx, key, members...).Result()
}

// ZScore 获取有序集合成员分数
func (c *Client) ZScore(ctx context.Context, key, member string) (float64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.ZScore(ctx, key, member).Result()
}

// ZCard 获取有序集合成员数量
func (c *Client) ZCard(ctx context.Context, key string) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrClientClosed
	}
	return c.client.ZCard(ctx, key).Result()
}

// Eval 执行 Lua 脚本
func (c *Client) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrClientClosed
	}
	return c.client.Eval(ctx, script, keys, args...).Result()
}

// EvalSha 执行已加载的 Lua 脚本
func (c *Client) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrClientClosed
	}
	return c.client.EvalSha(ctx, sha1, keys, args...).Result()
}

// ScriptLoad 加载 Lua 脚本
func (c *Client) ScriptLoad(ctx context.Context, script string) (string, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return "", ErrClientClosed
	}
	return c.client.ScriptLoad(ctx, script).Result()
}

// Watch 监视键
func (c *Client) Watch(ctx context.Context, fn func(*redis.Tx) error, keys ...string) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrClientClosed
	}
	return c.client.Watch(ctx, fn, keys...)
}
