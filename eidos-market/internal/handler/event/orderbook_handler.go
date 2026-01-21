package event

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/service"
)

// OrderBookProcessor 订单簿处理接口 (用于依赖注入和测试)
type OrderBookProcessor interface {
	ProcessOrderBookUpdate(ctx context.Context, update *model.DepthUpdate) error
}

// OrderBookUpdateMessage Kafka 消息格式
// [eidos-matching] 发送订单簿增量更新到 Kafka
//
//	Topic: orderbook-updates
//	Key: market 字段 (用于分区，如 "BTC-USDC")
//	Value: JSON 格式的 OrderBookUpdateMessage
//
//	增量更新语义:
//	  - amount > 0: 新增或更新该价格档位
//	  - amount = 0: 删除该价格档位
//	  - sequence: 单调递增序列号，用于检测消息缺失
//
//	消息发送时机:
//	  - 每次撮合成功后发送变化的档位
//	  - 订单取消后发送变化的档位
//	  - 建议批量合并同一市场的更新，减少消息量
type OrderBookUpdateMessage struct {
	Market   string              `json:"market"`
	Bids     []PriceLevelMessage `json:"bids"`
	Asks     []PriceLevelMessage `json:"asks"`
	Sequence uint64              `json:"sequence"`
}

// PriceLevelMessage 价格档位消息
type PriceLevelMessage struct {
	Price  string `json:"price"`
	Amount string `json:"amount"`
}

// OrderBookHandler 订单簿更新事件处理器
// 消费 Kafka orderbook-updates Topic
type OrderBookHandler struct {
	svc    OrderBookProcessor
	logger *slog.Logger
}

// NewOrderBookHandler 创建订单簿事件处理器
func NewOrderBookHandler(svc *service.MarketService, logger *slog.Logger) *OrderBookHandler {
	return &OrderBookHandler{
		svc:    svc,
		logger: logger.With("component", "orderbook_handler"),
	}
}

// NewOrderBookHandlerWithProcessor 创建订单簿事件处理器 (用于测试)
func NewOrderBookHandlerWithProcessor(svc OrderBookProcessor, logger *slog.Logger) *OrderBookHandler {
	return &OrderBookHandler{
		svc:    svc,
		logger: logger.With("component", "orderbook_handler"),
	}
}

// Handle 处理订单簿更新事件
func (h *OrderBookHandler) Handle(ctx context.Context, key, value []byte) error {
	var msg OrderBookUpdateMessage
	if err := json.Unmarshal(value, &msg); err != nil {
		h.logger.Error("failed to unmarshal orderbook update",
			"error", err,
			"value", string(value))
		return err
	}

	// 转换为内部模型
	update, err := h.toDepthUpdate(&msg)
	if err != nil {
		return err
	}

	h.logger.Debug("received orderbook update",
		"market", update.Market,
		"bids", len(update.Bids),
		"asks", len(update.Asks),
		"sequence", update.Sequence)

	if err := h.svc.ProcessOrderBookUpdate(ctx, update); err != nil {
		h.logger.Error("failed to process orderbook update",
			"market", msg.Market,
			"error", err)
		return err
	}

	return nil
}

// toDepthUpdate 转换消息为深度更新
func (h *OrderBookHandler) toDepthUpdate(msg *OrderBookUpdateMessage) (*model.DepthUpdate, error) {
	update := &model.DepthUpdate{
		Market:   msg.Market,
		Sequence: msg.Sequence,
		Bids:     make([]*model.PriceLevel, 0, len(msg.Bids)),
		Asks:     make([]*model.PriceLevel, 0, len(msg.Asks)),
	}

	for _, bid := range msg.Bids {
		price, err := decimal.NewFromString(bid.Price)
		if err != nil {
			h.logger.Error("invalid bid price", "price", bid.Price, "error", err)
			return nil, err
		}
		amount, err := decimal.NewFromString(bid.Amount)
		if err != nil {
			h.logger.Error("invalid bid amount", "amount", bid.Amount, "error", err)
			return nil, err
		}
		update.Bids = append(update.Bids, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	for _, ask := range msg.Asks {
		price, err := decimal.NewFromString(ask.Price)
		if err != nil {
			h.logger.Error("invalid ask price", "price", ask.Price, "error", err)
			return nil, err
		}
		amount, err := decimal.NewFromString(ask.Amount)
		if err != nil {
			h.logger.Error("invalid ask amount", "amount", ask.Amount, "error", err)
			return nil, err
		}
		update.Asks = append(update.Asks, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	return update, nil
}

// Topic 返回订阅的 Topic
func (h *OrderBookHandler) Topic() string {
	return "orderbook-updates"
}
