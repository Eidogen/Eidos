// Package event 处理来自 Kafka 的事件
package event

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
	"go.uber.org/zap"
)

// TradeEventHandler 处理成交事件
type TradeEventHandler struct {
	clearingService service.ClearingService
}

// NewTradeEventHandler 创建成交事件处理器
func NewTradeEventHandler(clearingService service.ClearingService) *TradeEventHandler {
	return &TradeEventHandler{
		clearingService: clearingService,
	}
}

// HandleEvent 实现 EventHandler 接口
func (h *TradeEventHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	msg, err := worker.ParseTradeResult(payload)
	if err != nil {
		return fmt.Errorf("parse trade result: %w", err)
	}

	logger.Info("processing trade result",
		zap.String("trade_id", msg.TradeID),
		zap.String("market", msg.Market),
		zap.String("maker_order_id", msg.MakerOrderID),
		zap.String("taker_order_id", msg.TakerOrderID),
	)

	// 调用清算服务处理成交
	if err := h.clearingService.ProcessTradeResult(ctx, msg); err != nil {
		return fmt.Errorf("process trade result: %w", err)
	}

	return nil
}

// Topic 返回处理的 topic
func (h *TradeEventHandler) Topic() string {
	return kafka.TopicTradeResults
}

// OrderCancelledHandler 处理订单取消事件
type OrderCancelledHandler struct {
	orderService service.OrderService
}

// NewOrderCancelledHandler 创建订单取消处理器
func NewOrderCancelledHandler(orderService service.OrderService) *OrderCancelledHandler {
	return &OrderCancelledHandler{
		orderService: orderService,
	}
}

// HandleEvent 实现 EventHandler 接口
func (h *OrderCancelledHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	msg, err := worker.ParseOrderCancelled(payload)
	if err != nil {
		return fmt.Errorf("parse order cancelled: %w", err)
	}

	logger.Info("processing order cancelled",
		zap.String("order_id", msg.OrderID),
		zap.String("market", msg.Market),
		zap.String("result", msg.Result),
	)

	// 转换为服务层消息类型
	svcMsg := &service.OrderCancelledMessage{
		OrderID:       msg.OrderID,
		Market:        msg.Market,
		Result:        msg.Result,
		RemainingSize: msg.RemainingSize,
		FilledSize:    msg.FilledSize,
		Timestamp:     msg.Timestamp,
		Sequence:      msg.Sequence,
	}

	// 调用订单服务处理取消确认
	if err := h.orderService.HandleCancelConfirm(ctx, svcMsg); err != nil {
		return fmt.Errorf("handle cancel confirm: %w", err)
	}

	return nil
}

// Topic 返回处理的 topic
func (h *OrderCancelledHandler) Topic() string {
	return kafka.TopicOrderCancelled
}

// OrderAcceptedHandler 处理订单接受事件
type OrderAcceptedHandler struct {
	orderService service.OrderService
}

// NewOrderAcceptedHandler 创建订单接受处理器
func NewOrderAcceptedHandler(orderService service.OrderService) *OrderAcceptedHandler {
	return &OrderAcceptedHandler{
		orderService: orderService,
	}
}

// HandleEvent 实现 EventHandler 接口
func (h *OrderAcceptedHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	msg, err := worker.ParseOrderAccepted(payload)
	if err != nil {
		return fmt.Errorf("parse order accepted: %w", err)
	}

	logger.Debug("processing order accepted",
		zap.String("order_id", msg.OrderID),
		zap.String("market", msg.Market),
	)

	// 转换为服务层消息类型
	svcMsg := &service.OrderAcceptedMessage{
		OrderID:   msg.OrderID,
		Market:    msg.Market,
		Timestamp: msg.Timestamp,
		Sequence:  msg.Sequence,
	}

	// 更新订单状态为 OPEN
	if err := h.orderService.HandleOrderAccepted(ctx, svcMsg); err != nil {
		return fmt.Errorf("handle order accepted: %w", err)
	}

	return nil
}

// Topic 返回处理的 topic
func (h *OrderAcceptedHandler) Topic() string {
	return "order-accepted" // 新增 topic
}
