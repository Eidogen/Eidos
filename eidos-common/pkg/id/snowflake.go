// Package id 提供分布式唯一 ID 生成器
// 基于 Snowflake 算法，支持时钟回拨处理
package id

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Snowflake ID 格式:
// 1 bit 符号位 (始终为0) + 41 bit 时间戳 + 10 bit 机器 ID + 12 bit 序列号
// 可用约 69 年，每毫秒可生成 4096 个 ID，支持 1024 个节点

const (
	epoch          = int64(1704067200000) // 2024-01-01 00:00:00 UTC (毫秒)
	workerIDBits   = 10                   // 机器 ID 位数
	sequenceBits   = 12                   // 序列号位数
	maxWorkerID    = -1 ^ (-1 << workerIDBits)
	maxSequence    = -1 ^ (-1 << sequenceBits)
	workerIDShift  = sequenceBits
	timestampShift = sequenceBits + workerIDBits

	// 时钟回拨容忍阈值 (毫秒)
	// 小于等于此值时等待追上，大于此值时返回错误
	maxClockBackward = 5
)

var (
	// ErrClockMovedBackwards 时钟回拨超过阈值错误
	ErrClockMovedBackwards = errors.New("clock moved backwards, refusing to generate ID")
	// ErrInvalidWorkerID 无效的机器 ID
	ErrInvalidWorkerID = errors.New("invalid worker ID")
)

// Generator Snowflake ID 生成器
type Generator struct {
	mu        sync.Mutex
	workerID  int64 // 机器 ID (0-1023)
	sequence  int64 // 当前毫秒内的序列号
	lastStamp int64 // 上次生成 ID 的时间戳
}

// NewGenerator 创建 ID 生成器
// workerID: 机器 ID，范围 0-1023
func NewGenerator(workerID int64) (*Generator, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("%w: must be between 0 and %d", ErrInvalidWorkerID, maxWorkerID)
	}
	return &Generator{
		workerID:  workerID,
		sequence:  0,
		lastStamp: -1,
	}, nil
}

// Generate 生成唯一 ID
// 返回值: (id, error)
// 当时钟回拨超过 5ms 时返回 ErrClockMovedBackwards
func (g *Generator) Generate() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli()

	// ⚠️ 时钟回拨处理
	if now < g.lastStamp {
		backward := g.lastStamp - now
		if backward > maxClockBackward {
			// 回拨超过阈值，拒绝生成 (防止 ID 重复)
			return 0, ErrClockMovedBackwards
		}
		// 回拨在阈值内，等待时钟追上
		time.Sleep(time.Duration(backward) * time.Millisecond)
		now = time.Now().UnixMilli()
	}

	if now == g.lastStamp {
		// 同一毫秒内，序列号递增
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// 序列号用完，等待下一毫秒
			for now <= g.lastStamp {
				time.Sleep(time.Microsecond * 100)
				now = time.Now().UnixMilli()
			}
		}
	} else {
		// 新的毫秒，序列号重置
		g.sequence = 0
	}

	g.lastStamp = now

	// 组装 ID: 时间戳 | 机器ID | 序列号
	id := ((now - epoch) << timestampShift) |
		(g.workerID << workerIDShift) |
		g.sequence

	return id, nil
}

// MustGenerate 生成 ID，时钟回拨时 panic
// 用于不允许失败的场景
func (g *Generator) MustGenerate() int64 {
	id, err := g.Generate()
	if err != nil {
		panic(err)
	}
	return id
}

// GenerateString 生成字符串格式的 ID
func (g *Generator) GenerateString() (string, error) {
	id, err := g.Generate()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}

// ParseID 解析 ID，提取时间戳、机器ID、序列号
func ParseID(id int64) (timestamp int64, workerID int64, sequence int64) {
	timestamp = (id >> timestampShift) + epoch
	workerID = (id >> workerIDShift) & maxWorkerID
	sequence = id & maxSequence
	return
}

// ParseTime 从 ID 中解析时间
func ParseTime(id int64) time.Time {
	ms := (id >> timestampShift) + epoch
	return time.UnixMilli(ms)
}

// ============================================================
// 全局默认生成器 (便捷方法)
// ============================================================

var (
	defaultGenerator *Generator
	once             sync.Once
	initErr          error
)

// InitDefault 初始化默认生成器
// 应在服务启动时调用一次
func InitDefault(workerID int64) error {
	once.Do(func() {
		defaultGenerator, initErr = NewGenerator(workerID)
	})
	return initErr
}

// Next 使用默认生成器生成 ID
// 如果未初始化，使用 workerID=0
func Next() (int64, error) {
	if defaultGenerator == nil {
		if err := InitDefault(0); err != nil {
			return 0, err
		}
	}
	return defaultGenerator.Generate()
}

// MustNext 使用默认生成器生成 ID，失败时 panic
func MustNext() int64 {
	id, err := Next()
	if err != nil {
		panic(err)
	}
	return id
}

// NextString 使用默认生成器生成字符串 ID
func NextString() (string, error) {
	id, err := Next()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}
