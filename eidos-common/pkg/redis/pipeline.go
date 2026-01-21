package redis

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

var (
	// ErrPipelineEmpty 管道为空
	ErrPipelineEmpty = errors.New("pipeline is empty")
	// ErrPipelineExecuted 管道已执行
	ErrPipelineExecuted = errors.New("pipeline already executed")
)

// Pipeline Redis 管道封装
type Pipeline struct {
	pipe     redis.Pipeliner
	mu       sync.Mutex
	commands int
	executed bool
}

// NewPipeline 创建管道
func NewPipeline(pipe redis.Pipeliner) *Pipeline {
	return &Pipeline{
		pipe: pipe,
	}
}

// Get 获取值
func (p *Pipeline) Get(ctx context.Context, key string) *redis.StringCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.Get(ctx, key)
}

// Set 设置值
func (p *Pipeline) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.Set(ctx, key, value, expiration)
}

// SetNX 设置值 (仅当不存在时)
func (p *Pipeline) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.SetNX(ctx, key, value, expiration)
}

// Del 删除键
func (p *Pipeline) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.Del(ctx, keys...)
}

// Exists 检查键是否存在
func (p *Pipeline) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.Exists(ctx, keys...)
}

// Expire 设置过期时间
func (p *Pipeline) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.Expire(ctx, key, expiration)
}

// TTL 获取剩余过期时间
func (p *Pipeline) TTL(ctx context.Context, key string) *redis.DurationCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.TTL(ctx, key)
}

// Incr 递增
func (p *Pipeline) Incr(ctx context.Context, key string) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.Incr(ctx, key)
}

// IncrBy 递增指定值
func (p *Pipeline) IncrBy(ctx context.Context, key string, value int64) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.IncrBy(ctx, key, value)
}

// IncrByFloat 递增浮点值
func (p *Pipeline) IncrByFloat(ctx context.Context, key string, value float64) *redis.FloatCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.IncrByFloat(ctx, key, value)
}

// Decr 递减
func (p *Pipeline) Decr(ctx context.Context, key string) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.Decr(ctx, key)
}

// DecrBy 递减指定值
func (p *Pipeline) DecrBy(ctx context.Context, key string, value int64) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.DecrBy(ctx, key, value)
}

// HGet 获取哈希字段值
func (p *Pipeline) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.HGet(ctx, key, field)
}

// HSet 设置哈希字段值
func (p *Pipeline) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.HSet(ctx, key, values...)
}

// HGetAll 获取所有哈希字段值
func (p *Pipeline) HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.HGetAll(ctx, key)
}

// HDel 删除哈希字段
func (p *Pipeline) HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.HDel(ctx, key, fields...)
}

// HIncrBy 递增哈希字段值
func (p *Pipeline) HIncrBy(ctx context.Context, key, field string, incr int64) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.HIncrBy(ctx, key, field, incr)
}

// HIncrByFloat 递增哈希字段浮点值
func (p *Pipeline) HIncrByFloat(ctx context.Context, key, field string, incr float64) *redis.FloatCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.HIncrByFloat(ctx, key, field, incr)
}

// LPush 左侧推入列表
func (p *Pipeline) LPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.LPush(ctx, key, values...)
}

// RPush 右侧推入列表
func (p *Pipeline) RPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.RPush(ctx, key, values...)
}

// LPop 左侧弹出列表
func (p *Pipeline) LPop(ctx context.Context, key string) *redis.StringCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.LPop(ctx, key)
}

// RPop 右侧弹出列表
func (p *Pipeline) RPop(ctx context.Context, key string) *redis.StringCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.RPop(ctx, key)
}

// LRange 获取列表范围
func (p *Pipeline) LRange(ctx context.Context, key string, start, stop int64) *redis.StringSliceCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.LRange(ctx, key, start, stop)
}

// LLen 获取列表长度
func (p *Pipeline) LLen(ctx context.Context, key string) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.LLen(ctx, key)
}

// SAdd 添加集合成员
func (p *Pipeline) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.SAdd(ctx, key, members...)
}

// SMembers 获取集合所有成员
func (p *Pipeline) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.SMembers(ctx, key)
}

// SIsMember 检查是否是集合成员
func (p *Pipeline) SIsMember(ctx context.Context, key string, member interface{}) *redis.BoolCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.SIsMember(ctx, key, member)
}

// SRem 移除集合成员
func (p *Pipeline) SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.SRem(ctx, key, members...)
}

// ZAdd 添加有序集合成员
func (p *Pipeline) ZAdd(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.ZAdd(ctx, key, members...)
}

// ZRange 获取有序集合范围
func (p *Pipeline) ZRange(ctx context.Context, key string, start, stop int64) *redis.StringSliceCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.ZRange(ctx, key, start, stop)
}

// ZRangeWithScores 获取有序集合范围(带分数)
func (p *Pipeline) ZRangeWithScores(ctx context.Context, key string, start, stop int64) *redis.ZSliceCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.ZRangeWithScores(ctx, key, start, stop)
}

// ZRem 移除有序集合成员
func (p *Pipeline) ZRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.ZRem(ctx, key, members...)
}

// ZScore 获取有序集合成员分数
func (p *Pipeline) ZScore(ctx context.Context, key, member string) *redis.FloatCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.ZScore(ctx, key, member)
}

// ZCard 获取有序集合成员数量
func (p *Pipeline) ZCard(ctx context.Context, key string) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.ZCard(ctx, key)
}

// ZIncrBy 递增有序集合成员分数
func (p *Pipeline) ZIncrBy(ctx context.Context, key string, increment float64, member string) *redis.FloatCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands++
	return p.pipe.ZIncrBy(ctx, key, increment, member)
}

// Exec 执行管道
func (p *Pipeline) Exec(ctx context.Context) ([]redis.Cmder, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.executed {
		return nil, ErrPipelineExecuted
	}

	if p.commands == 0 {
		return nil, ErrPipelineEmpty
	}

	p.executed = true
	cmds, err := p.pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		logger.Error("pipeline exec failed",
			zap.Int("commands", p.commands),
			zap.Error(err),
		)
	}

	return cmds, err
}

// Discard 丢弃管道
func (p *Pipeline) Discard() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pipe.Discard()
	p.executed = true
}

// Len 获取命令数量
func (p *Pipeline) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.commands
}

// BatchExecutor 批量执行器
type BatchExecutor struct {
	client    redis.UniversalClient
	batchSize int
	commands  []func(pipe redis.Pipeliner)
	mu        sync.Mutex
}

// NewBatchExecutor 创建批量执行器
func NewBatchExecutor(client redis.UniversalClient, batchSize int) *BatchExecutor {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &BatchExecutor{
		client:    client,
		batchSize: batchSize,
		commands:  make([]func(pipe redis.Pipeliner), 0, batchSize),
	}
}

// Add 添加命令
func (b *BatchExecutor) Add(cmd func(pipe redis.Pipeliner)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.commands = append(b.commands, cmd)
}

// Flush 执行所有命令
func (b *BatchExecutor) Flush(ctx context.Context) error {
	b.mu.Lock()
	commands := b.commands
	b.commands = make([]func(pipe redis.Pipeliner), 0, b.batchSize)
	b.mu.Unlock()

	if len(commands) == 0 {
		return nil
	}

	// 分批执行
	for i := 0; i < len(commands); i += b.batchSize {
		end := i + b.batchSize
		if end > len(commands) {
			end = len(commands)
		}

		batch := commands[i:end]
		_, err := b.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, cmd := range batch {
				cmd(pipe)
			}
			return nil
		})

		if err != nil && !errors.Is(err, redis.Nil) {
			logger.Error("batch execute failed",
				zap.Int("batch_start", i),
				zap.Int("batch_size", len(batch)),
				zap.Error(err),
			)
			return err
		}
	}

	return nil
}

// Len 获取待执行命令数量
func (b *BatchExecutor) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.commands)
}

// Clear 清空命令
func (b *BatchExecutor) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.commands = make([]func(pipe redis.Pipeliner), 0, b.batchSize)
}
