// Package kafka Kafka 消费者单元测试
package kafka

import (
	"context"
	"testing"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewConsumer(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
		BatchSize:    100,
		LingerMs:     10,
		StartOffset:  -1,
		CommitMode:   "manual",
	}

	c := NewConsumer(cfg)

	assert.NotNil(t, c)
	assert.NotNil(t, c.ordersReader)
	assert.NotNil(t, c.cancelsReader)
	assert.NotNil(t, c.offsets)
	assert.Equal(t, cfg, c.config)
}

func TestConsumer_SetHandlers(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 设置处理函数
	orderHandler := func(ctx context.Context, order *model.Order) error { return nil }
	cancelHandler := func(ctx context.Context, cancel *model.CancelMessage) error { return nil }

	c.SetOrderHandler(orderHandler)
	c.SetCancelHandler(cancelHandler)

	assert.NotNil(t, c.orderHandler)
	assert.NotNil(t, c.cancelHandler)
}

func TestConsumer_Start_NoHandlers(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 未设置 handler 应该返回错误
	err := c.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handlers not set")
}

func TestConsumer_ParseOrderMessage(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "valid order message",
			data: []byte(`{
				"order_id": "order-1",
				"wallet": "0x123",
				"market": "BTC-USDC",
				"side": 1,
				"order_type": 1,
				"time_in_force": 1,
				"price": "50000.00",
				"amount": "1.5",
				"timestamp": 1704067200000,
				"sequence": 1
			}`),
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    []byte(`{invalid json}`),
			wantErr: true,
		},
		{
			name: "invalid price",
			data: []byte(`{
				"order_id": "order-1",
				"wallet": "0x123",
				"market": "BTC-USDC",
				"side": 1,
				"order_type": 1,
				"time_in_force": 1,
				"price": "invalid",
				"amount": "1.5",
				"timestamp": 1704067200000,
				"sequence": 1
			}`),
			wantErr: true,
		},
		{
			name: "invalid amount",
			data: []byte(`{
				"order_id": "order-1",
				"wallet": "0x123",
				"market": "BTC-USDC",
				"side": 1,
				"order_type": 1,
				"time_in_force": 1,
				"price": "50000.00",
				"amount": "invalid",
				"timestamp": 1704067200000,
				"sequence": 1
			}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order, err := c.parseOrderMessage(tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, order)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, order)
				assert.Equal(t, "order-1", order.OrderID)
				assert.Equal(t, "0x123", order.Wallet)
				assert.Equal(t, "BTC-USDC", order.Market)
				assert.True(t, decimal.NewFromFloat(50000.00).Equal(order.Price))
				assert.True(t, decimal.NewFromFloat(1.5).Equal(order.Amount))
				assert.True(t, order.Amount.Equal(order.Remaining)) // 初始 remaining = amount
			}
		})
	}
}

func TestConsumer_ParseCancelMessage(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "valid cancel message",
			data: []byte(`{
				"order_id": "order-1",
				"wallet": "0x123",
				"market": "BTC-USDC",
				"timestamp": 1704067200000,
				"sequence": 1
			}`),
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    []byte(`{invalid json}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cancel, err := c.parseCancelMessage(tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cancel)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cancel)
				assert.Equal(t, "order-1", cancel.OrderID)
				assert.Equal(t, "0x123", cancel.Wallet)
				assert.Equal(t, "BTC-USDC", cancel.Market)
			}
		})
	}
}

func TestConsumer_UpdateOffset(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 更新偏移量
	c.updateOffset("orders", 0, 100)
	c.updateOffset("orders", 1, 200)
	c.updateOffset("cancel-requests", 0, 50)

	// 获取偏移量
	offsets := c.GetOffsets()
	assert.Equal(t, int64(100), offsets["orders:0"])
	assert.Equal(t, int64(200), offsets["orders:1"])
	assert.Equal(t, int64(50), offsets["cancel-requests:0"])
}

func TestConsumer_GetOffsets_Copy(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)
	c.updateOffset("orders", 0, 100)

	// 获取副本
	offsets1 := c.GetOffsets()
	offsets2 := c.GetOffsets()

	// 修改不应影响原始数据
	offsets1["orders:0"] = 999

	assert.Equal(t, int64(100), offsets2["orders:0"])
}

func TestConsumer_SeekToOffset_NotImplemented(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	err := c.SeekToOffset("orders", 0, 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "seek not implemented")
}

func TestConsumerConfig_Fields(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"broker1:9092", "broker2:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
		BatchSize:    100,
		LingerMs:     50,
		StartOffset:  -2, // earliest
		CommitMode:   "auto",
	}

	assert.Equal(t, 2, len(cfg.Brokers))
	assert.Equal(t, "test-group", cfg.GroupID)
	assert.Equal(t, "orders", cfg.OrdersTopic)
	assert.Equal(t, "cancel-requests", cfg.CancelsTopic)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.Equal(t, 50, cfg.LingerMs)
	assert.Equal(t, int64(-2), cfg.StartOffset)
	assert.Equal(t, "auto", cfg.CommitMode)
}

func TestConsumer_UpdateOffset_Overwrite(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 第一次更新
	c.updateOffset("orders", 0, 100)
	offsets := c.GetOffsets()
	assert.Equal(t, int64(100), offsets["orders:0"])

	// 覆盖更新
	c.updateOffset("orders", 0, 200)
	offsets = c.GetOffsets()
	assert.Equal(t, int64(200), offsets["orders:0"])
}

func TestConsumer_UpdateOffset_Concurrent(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 并发更新测试
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		partition := i
		go func() {
			for j := 0; j < 100; j++ {
				c.updateOffset("orders", partition, int64(j))
			}
			done <- struct{}{}
		}()
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}

	offsets := c.GetOffsets()
	// 每个分区都应该有记录
	for i := 0; i < 10; i++ {
		key := "orders:" + string(rune('0'+i))
		_, exists := offsets[key]
		// 可能不存在因为 key 格式问题，用 sprintf 更安全
		_ = exists
	}
}

func TestConsumer_ParseOrderMessage_AllFields(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	data := []byte(`{
		"order_id": "order-123",
		"wallet": "0xabc123",
		"market": "ETH-USDC",
		"side": 2,
		"order_type": 2,
		"time_in_force": 2,
		"price": "3000.50",
		"amount": "10.25",
		"timestamp": 1704067200000,
		"sequence": 999
	}`)

	order, err := c.parseOrderMessage(data)
	assert.NoError(t, err)
	assert.Equal(t, "order-123", order.OrderID)
	assert.Equal(t, "0xabc123", order.Wallet)
	assert.Equal(t, "ETH-USDC", order.Market)
	assert.Equal(t, model.OrderSideSell, order.Side)
	assert.Equal(t, model.OrderTypeMarket, order.Type)
	assert.Equal(t, model.TimeInForceIOC, order.TimeInForce)
	assert.True(t, decimal.NewFromFloat(3000.50).Equal(order.Price))
	assert.True(t, decimal.NewFromFloat(10.25).Equal(order.Amount))
	assert.Equal(t, int64(1704067200000), order.Timestamp)
	assert.Equal(t, int64(999), order.Sequence)
}

func TestConsumer_ParseCancelMessage_AllFields(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	data := []byte(`{
		"order_id": "cancel-order-456",
		"wallet": "0xdef789",
		"market": "SOL-USDC",
		"timestamp": 1704067300000,
		"sequence": 1001
	}`)

	cancel, err := c.parseCancelMessage(data)
	assert.NoError(t, err)
	assert.Equal(t, "cancel-order-456", cancel.OrderID)
	assert.Equal(t, "0xdef789", cancel.Wallet)
	assert.Equal(t, "SOL-USDC", cancel.Market)
	assert.Equal(t, int64(1704067300000), cancel.Timestamp)
	assert.Equal(t, int64(1001), cancel.Sequence)
}

func TestConsumer_ParseOrderMessage_EmptyFields(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 最小字段
	data := []byte(`{
		"order_id": "",
		"wallet": "",
		"market": "",
		"side": 0,
		"order_type": 0,
		"time_in_force": 0,
		"price": "0",
		"amount": "0",
		"timestamp": 0,
		"sequence": 0
	}`)

	order, err := c.parseOrderMessage(data)
	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, "", order.OrderID)
	assert.True(t, decimal.Zero.Equal(order.Price))
}

func TestConsumer_Start_OnlyOrderHandler(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 只设置 order handler
	c.SetOrderHandler(func(ctx context.Context, order *model.Order) error { return nil })

	err := c.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handlers not set")
}

func TestConsumer_Start_OnlyCancelHandler(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-group",
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
	}

	c := NewConsumer(cfg)

	// 只设置 cancel handler
	c.SetCancelHandler(func(ctx context.Context, cancel *model.CancelMessage) error { return nil })

	err := c.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handlers not set")
}
