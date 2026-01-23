// Package engine 延迟追踪器
package engine

import (
	"sort"
	"sync"
	"time"
)

// LatencyTracker 追踪延迟统计，使用环形缓冲区存储最近的延迟样本
type LatencyTracker struct {
	mu       sync.RWMutex
	samples  []int64 // 延迟样本 (微秒)
	index    int     // 当前写入位置
	count    int     // 已记录样本数
	capacity int     // 缓冲区容量

	// TPS 计算
	startTime     time.Time
	totalOrders   int64
	windowStart   time.Time
	windowOrders  int64
	windowSeconds int
}

// NewLatencyTracker 创建延迟追踪器
// capacity: 样本缓冲区大小 (建议 1000-10000)
// windowSeconds: TPS 计算窗口 (秒)
func NewLatencyTracker(capacity, windowSeconds int) *LatencyTracker {
	if capacity <= 0 {
		capacity = 1000
	}
	if windowSeconds <= 0 {
		windowSeconds = 1
	}
	return &LatencyTracker{
		samples:       make([]int64, capacity),
		capacity:      capacity,
		startTime:     time.Now(),
		windowStart:   time.Now(),
		windowSeconds: windowSeconds,
	}
}

// Record 记录一次延迟 (微秒)
func (t *LatencyTracker) Record(latencyUs int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.samples[t.index] = latencyUs
	t.index = (t.index + 1) % t.capacity
	if t.count < t.capacity {
		t.count++
	}

	t.totalOrders++
	t.windowOrders++

	// 检查是否需要重置窗口
	if time.Since(t.windowStart) > time.Duration(t.windowSeconds)*time.Second {
		t.windowStart = time.Now()
		t.windowOrders = 1
	}
}

// P99 返回 P99 延迟 (微秒)
func (t *LatencyTracker) P99() int64 {
	return t.percentile(99)
}

// P95 返回 P95 延迟 (微秒)
func (t *LatencyTracker) P95() int64 {
	return t.percentile(95)
}

// P50 返回 P50 延迟 (微秒)
func (t *LatencyTracker) P50() int64 {
	return t.percentile(50)
}

// percentile 计算指定百分位数
func (t *LatencyTracker) percentile(p int) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return 0
	}

	// 复制样本并排序
	samples := make([]int64, t.count)
	if t.count < t.capacity {
		copy(samples, t.samples[:t.count])
	} else {
		copy(samples, t.samples)
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})

	// 计算百分位索引
	idx := (p * t.count) / 100
	if idx >= t.count {
		idx = t.count - 1
	}

	return samples[idx]
}

// TPS 返回当前每秒订单数 (基于滑动窗口)
func (t *LatencyTracker) TPS() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	elapsed := time.Since(t.windowStart).Seconds()
	if elapsed < 0.001 {
		return 0
	}

	return float64(t.windowOrders) / elapsed
}

// AvgTPS 返回平均每秒订单数 (基于启动时间)
func (t *LatencyTracker) AvgTPS() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	elapsed := time.Since(t.startTime).Seconds()
	if elapsed < 0.001 {
		return 0
	}

	return float64(t.totalOrders) / elapsed
}

// Stats 返回统计摘要
func (t *LatencyTracker) Stats() LatencyStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return LatencyStats{}
	}

	// 复制样本并排序
	samples := make([]int64, t.count)
	if t.count < t.capacity {
		copy(samples, t.samples[:t.count])
	} else {
		copy(samples, t.samples)
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})

	// 计算统计值
	var sum int64
	for _, v := range samples {
		sum += v
	}

	p50Idx := (50 * t.count) / 100
	p95Idx := (95 * t.count) / 100
	p99Idx := (99 * t.count) / 100
	if p99Idx >= t.count {
		p99Idx = t.count - 1
	}
	if p95Idx >= t.count {
		p95Idx = t.count - 1
	}
	if p50Idx >= t.count {
		p50Idx = t.count - 1
	}

	elapsed := time.Since(t.windowStart).Seconds()
	tps := float64(0)
	if elapsed >= 0.001 {
		tps = float64(t.windowOrders) / elapsed
	}

	return LatencyStats{
		Count:       int64(t.count),
		Min:         samples[0],
		Max:         samples[t.count-1],
		Avg:         sum / int64(t.count),
		P50:         samples[p50Idx],
		P95:         samples[p95Idx],
		P99:         samples[p99Idx],
		TPS:         tps,
		TotalOrders: t.totalOrders,
	}
}

// Reset 重置追踪器
func (t *LatencyTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.index = 0
	t.count = 0
	t.totalOrders = 0
	t.windowOrders = 0
	t.startTime = time.Now()
	t.windowStart = time.Now()
}

// LatencyStats 延迟统计
type LatencyStats struct {
	Count       int64   `json:"count"`
	Min         int64   `json:"min_us"`
	Max         int64   `json:"max_us"`
	Avg         int64   `json:"avg_us"`
	P50         int64   `json:"p50_us"`
	P95         int64   `json:"p95_us"`
	P99         int64   `json:"p99_us"`
	TPS         float64 `json:"tps"`
	TotalOrders int64   `json:"total_orders"`
}
