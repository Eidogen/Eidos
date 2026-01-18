// Package ws 提供 WebSocket 功能
package ws

import "encoding/json"

// MessageType 消息类型
type MessageType string

const (
	// 客户端消息类型
	MsgTypeSubscribe   MessageType = "subscribe"
	MsgTypeUnsubscribe MessageType = "unsubscribe"
	MsgTypePing        MessageType = "ping"

	// 服务端消息类型
	MsgTypePong     MessageType = "pong"
	MsgTypeError    MessageType = "error"
	MsgTypeSnapshot MessageType = "snapshot"
	MsgTypeUpdate   MessageType = "update"
	MsgTypeAck      MessageType = "ack"
)

// Channel 订阅频道
type Channel string

const (
	ChannelTicker Channel = "ticker"
	ChannelDepth  Channel = "depth"
	ChannelKline  Channel = "kline"
	ChannelTrades Channel = "trades"
	ChannelOrders Channel = "orders" // 私有频道
)

// ClientMessage 客户端消息
type ClientMessage struct {
	Type    MessageType `json:"type"`
	ID      string      `json:"id,omitempty"` // 请求 ID，用于匹配响应
	Channel Channel     `json:"channel,omitempty"`
	Market  string      `json:"market,omitempty"`
	Params  interface{} `json:"params,omitempty"`
}

// ServerMessage 服务端消息
type ServerMessage struct {
	Type      MessageType `json:"type"`
	ID        string      `json:"id,omitempty"` // 对应请求 ID
	Channel   Channel     `json:"channel,omitempty"`
	Market    string      `json:"market,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp int64       `json:"timestamp,omitempty"`
	Code      int         `json:"code,omitempty"`    // 错误码
	Message   string      `json:"message,omitempty"` // 错误消息
}

// SubscribeParams 订阅参数
type SubscribeParams struct {
	Interval string `json:"interval,omitempty"` // K线间隔
	Depth    int    `json:"depth,omitempty"`    // 深度档位
}

// ParseClientMessage 解析客户端消息
func ParseClientMessage(data []byte) (*ClientMessage, error) {
	var msg ClientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// NewPongMessage 创建 Pong 消息
func NewPongMessage() *ServerMessage {
	return &ServerMessage{
		Type: MsgTypePong,
	}
}

// NewErrorMessage 创建错误消息
func NewErrorMessage(id string, code int, message string) *ServerMessage {
	return &ServerMessage{
		Type:    MsgTypeError,
		ID:      id,
		Code:    code,
		Message: message,
	}
}

// NewAckMessage 创建确认消息
func NewAckMessage(id string, channel Channel, market string) *ServerMessage {
	return &ServerMessage{
		Type:    MsgTypeAck,
		ID:      id,
		Channel: channel,
		Market:  market,
	}
}

// NewSnapshotMessage 创建快照消息
func NewSnapshotMessage(channel Channel, market string, data interface{}, timestamp int64) *ServerMessage {
	return &ServerMessage{
		Type:      MsgTypeSnapshot,
		Channel:   channel,
		Market:    market,
		Data:      data,
		Timestamp: timestamp,
	}
}

// NewUpdateMessage 创建更新消息
func NewUpdateMessage(channel Channel, market string, data interface{}, timestamp int64) *ServerMessage {
	return &ServerMessage{
		Type:      MsgTypeUpdate,
		Channel:   channel,
		Market:    market,
		Data:      data,
		Timestamp: timestamp,
	}
}

// ToJSON 序列化为 JSON
func (m *ServerMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// TickerData Ticker 数据
type TickerData struct {
	Market     string `json:"market"`
	LastPrice  string `json:"last_price"`
	High24h    string `json:"high_24h"`
	Low24h     string `json:"low_24h"`
	Volume24h  string `json:"volume_24h"`
	Change24h  string `json:"change_24h"`
	ChangeRate string `json:"change_rate"`
	BestBid    string `json:"best_bid"`
	BestAsk    string `json:"best_ask"`
}

// DepthData 深度数据
type DepthData struct {
	Market string     `json:"market"`
	Bids   [][]string `json:"bids"` // [[price, amount], ...]
	Asks   [][]string `json:"asks"`
}

// KlineData K线数据
type KlineData struct {
	Market    string `json:"market"`
	Interval  string `json:"interval"`
	OpenTime  int64  `json:"open_time"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
	CloseTime int64  `json:"close_time"`
}

// TradeData 成交数据
type TradeData struct {
	Market    string `json:"market"`
	TradeID   string `json:"trade_id"`
	Price     string `json:"price"`
	Amount    string `json:"amount"`
	Side      string `json:"side"`
	Timestamp int64  `json:"timestamp"`
}

// OrderData 订单数据（私有频道）
type OrderData struct {
	OrderID         string `json:"order_id"`
	Market          string `json:"market"`
	Side            string `json:"side"`
	Type            string `json:"type"`
	Price           string `json:"price"`
	Amount          string `json:"amount"`
	FilledAmount    string `json:"filled_amount"`
	RemainingAmount string `json:"remaining_amount"`
	Status          string `json:"status"`
	UpdatedAt       int64  `json:"updated_at"`
}
