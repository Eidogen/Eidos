package id

import (
	"fmt"
	"sync"
	"time"
)

// Snowflake ID 生成器
// 格式: 1 bit 符号位 + 41 bit 时间戳 + 10 bit 机器 ID + 12 bit 序列号

const (
	epoch          = int64(1704067200000) // 2024-01-01 00:00:00 UTC
	workerIDBits   = 10
	sequenceBits   = 12
	maxWorkerID    = -1 ^ (-1 << workerIDBits)
	maxSequence    = -1 ^ (-1 << sequenceBits)
	workerIDShift  = sequenceBits
	timestampShift = sequenceBits + workerIDBits
)

// Generator Snowflake ID 生成器
type Generator struct {
	mu        sync.Mutex
	workerID  int64
	sequence  int64
	lastStamp int64
}

// NewGenerator 创建 ID 生成器
func NewGenerator(workerID int64) (*Generator, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("worker ID must be between 0 and %d", maxWorkerID)
	}
	return &Generator{
		workerID:  workerID,
		sequence:  0,
		lastStamp: -1,
	}, nil
}

// Generate 生成唯一 ID
func (g *Generator) Generate() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli()

	if now < g.lastStamp {
		// 时钟回拨，等待
		for now <= g.lastStamp {
			time.Sleep(time.Millisecond)
			now = time.Now().UnixMilli()
		}
	}

	if now == g.lastStamp {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// 序列号用完，等待下一毫秒
			for now <= g.lastStamp {
				time.Sleep(time.Microsecond * 100)
				now = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastStamp = now

	id := ((now - epoch) << timestampShift) |
		(g.workerID << workerIDShift) |
		g.sequence

	return id
}

// GenerateString 生成字符串格式的 ID
func (g *Generator) GenerateString() string {
	return fmt.Sprintf("%d", g.Generate())
}

// ParseID 解析 ID
func ParseID(id int64) (timestamp int64, workerID int64, sequence int64) {
	timestamp = (id >> timestampShift) + epoch
	workerID = (id >> workerIDShift) & maxWorkerID
	sequence = id & maxSequence
	return
}

// 全局默认生成器
var defaultGenerator *Generator
var once sync.Once

// InitDefault 初始化默认生成器
func InitDefault(workerID int64) error {
	var err error
	once.Do(func() {
		defaultGenerator, err = NewGenerator(workerID)
	})
	return err
}

// Next 使用默认生成器生成 ID
func Next() int64 {
	if defaultGenerator == nil {
		InitDefault(0) // 默认使用 worker ID 0
	}
	return defaultGenerator.Generate()
}

// NextString 使用默认生成器生成字符串 ID
func NextString() string {
	return fmt.Sprintf("%d", Next())
}
