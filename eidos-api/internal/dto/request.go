package dto

// PaginationQuery 分页查询参数
type PaginationQuery struct {
	Page     int `form:"page" json:"page"`
	PageSize int `form:"page_size" json:"page_size"`
}

// Normalize 规范化分页参数
func (p *PaginationQuery) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
}

// TimeRangeQuery 时间范围查询参数
type TimeRangeQuery struct {
	StartTime int64 `form:"start_time" json:"start_time"`
	EndTime   int64 `form:"end_time" json:"end_time"`
}

// ========== 订单相关 ==========

// PrepareOrderRequest 准备订单请求（获取签名摘要）
type PrepareOrderRequest struct {
	Market      string `json:"market" binding:"required"`
	Side        string `json:"side" binding:"required,oneof=buy sell"`
	Type        string `json:"type" binding:"required,oneof=limit market"`
	Price       string `json:"price"`
	Amount      string `json:"amount" binding:"required"`
	TimeInForce string `json:"time_in_force"`
}

// PrepareOrderResponse 准备订单响应
type PrepareOrderResponse struct {
	OrderID   string      `json:"order_id"`
	TypedData interface{} `json:"typed_data"`
	Nonce     uint64      `json:"nonce"`
	ExpiresAt int64       `json:"expires_at"`
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	Wallet        string `json:"-"` // 从 context 中获取
	OrderID       string `json:"order_id" binding:"required"`
	Market        string `json:"market" binding:"required"`
	Side          string `json:"side" binding:"required,oneof=buy sell"`
	Type          string `json:"type" binding:"required,oneof=limit market"`
	Price         string `json:"price"`
	Amount        string `json:"amount" binding:"required"`
	TimeInForce   string `json:"time_in_force"`
	Nonce         uint64 `json:"nonce" binding:"required"`
	Signature     string `json:"signature" binding:"required"`
	ClientOrderID string `json:"client_order_id"`
	ExpireAt      int64  `json:"expire_at"`
}

// CreateOrderResponse 创建订单响应
type CreateOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

// CancelOrderRequest 取消订单请求
type CancelOrderRequest struct {
	Nonce     uint64 `json:"nonce"`
	Signature string `json:"signature"`
}

// BatchCancelOrdersRequest 批量取消订单请求
type BatchCancelOrdersRequest struct {
	OrderIDs []string `json:"order_ids" binding:"required,min=1,max=100"`
	Market   string   `json:"market"`
	Side     string   `json:"side"`
}

// BatchCancelRequest 批量取消请求（Handler 用）
type BatchCancelRequest struct {
	Wallet   string   `json:"-"`
	Market   string   `json:"market"`
	Side     string   `json:"side"`
	OrderIDs []string `json:"order_ids,omitempty"`
}

// BatchCancelResponse 批量取消响应
type BatchCancelResponse struct {
	Cancelled []string `json:"cancelled"`
	Failed    []string `json:"failed,omitempty"`
}

// ListOrdersQuery 查询订单列表参数
type ListOrdersQuery struct {
	PaginationQuery
	TimeRangeQuery
	Market   string   `form:"market"`
	Side     string   `form:"side"`
	Statuses []string `form:"status"`
}

// ListOrdersRequest 订单列表请求（Handler 用）
type ListOrdersRequest struct {
	Wallet    string
	Market    string
	Side      string
	Status    string
	OrderBy   string
	Order     string
	Page      int
	PageSize  int
	StartTime int64
	EndTime   int64
}

// PaginatedResponse 分页响应
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// OpenOrdersQuery 查询挂单参数
type OpenOrdersQuery struct {
	PaginationQuery
	Market string `form:"market"`
	Side   string `form:"side"`
}

// OrderResponse 订单响应
type OrderResponse struct {
	OrderID         string `json:"order_id"`
	Wallet          string `json:"wallet"`
	Market          string `json:"market"`
	Side            string `json:"side"`
	Type            string `json:"type"`
	Price           string `json:"price"`
	Amount          string `json:"amount"`
	FilledAmount    string `json:"filled_amount"`
	FilledQuote     string `json:"filled_quote"`
	RemainingAmount string `json:"remaining_amount"`
	AvgPrice        string `json:"avg_price"`
	Status          string `json:"status"`
	TimeInForce     string `json:"time_in_force"`
	Nonce           uint64 `json:"nonce"`
	ClientOrderID   string `json:"client_order_id,omitempty"`
	ExpireAt        int64  `json:"expire_at,omitempty"`
	RejectReason    string `json:"reject_reason,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// ========== 余额相关 ==========

// BalanceResponse 余额响应
type BalanceResponse struct {
	Wallet           string `json:"wallet"`
	Token            string `json:"token"`
	SettledAvailable string `json:"settled_available"`
	SettledFrozen    string `json:"settled_frozen"`
	PendingAvailable string `json:"pending_available"`
	PendingFrozen    string `json:"pending_frozen"`
	TotalAvailable   string `json:"total_available"`
	Total            string `json:"total"`
	Withdrawable     string `json:"withdrawable"`
	UpdatedAt        int64  `json:"updated_at"`
}

// ListTransactionsQuery 查询资金流水参数
type ListTransactionsQuery struct {
	PaginationQuery
	TimeRangeQuery
	Token string `form:"token"`
	Type  string `form:"type"`
}

// ListTransactionsRequest 资金流水请求（Handler 用）
type ListTransactionsRequest struct {
	Wallet    string
	Token     string
	Type      string
	Page      int
	PageSize  int
	StartTime int64
	EndTime   int64
}

// TransactionResponse 资金流水响应
type TransactionResponse struct {
	ID        string `json:"id"`
	Wallet    string `json:"wallet"`
	Token     string `json:"token"`
	Type      string `json:"type"`
	Amount    string `json:"amount"`
	Balance   string `json:"balance"`
	RefID     string `json:"ref_id,omitempty"`
	RefType   string `json:"ref_type,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// ========== 充值相关 ==========

// ListDepositsQuery 查询充值列表参数
type ListDepositsQuery struct {
	PaginationQuery
	TimeRangeQuery
	Token  string `form:"token"`
	Status string `form:"status"`
}

// ListDepositsRequest 充值列表请求（Handler 用）
type ListDepositsRequest struct {
	Wallet    string
	Token     string
	Status    string
	Page      int
	PageSize  int
	StartTime int64
	EndTime   int64
}

// DepositResponse 充值响应
type DepositResponse struct {
	DepositID   string `json:"deposit_id"`
	Wallet      string `json:"wallet"`
	Token       string `json:"token"`
	Amount      string `json:"amount"`
	TxHash      string `json:"tx_hash"`
	LogIndex    uint32 `json:"log_index"`
	BlockNum    int64  `json:"block_num"`
	Status      string `json:"status"`
	DetectedAt  int64  `json:"detected_at,omitempty"`
	ConfirmedAt int64  `json:"confirmed_at,omitempty"`
	CreditedAt  int64  `json:"credited_at,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// ========== 提现相关 ==========

// CreateWithdrawalRequest 创建提现请求
type CreateWithdrawalRequest struct {
	Wallet    string `json:"-"` // 从 context 中获取
	Token     string `json:"token" binding:"required"`
	Amount    string `json:"amount" binding:"required"`
	ToAddress string `json:"to_address" binding:"required"`
	Nonce     uint64 `json:"nonce" binding:"required"`
	Signature string `json:"signature" binding:"required"`
}

// CreateWithdrawalResponse 创建提现响应
type CreateWithdrawalResponse struct {
	WithdrawID string `json:"withdraw_id"`
	Status     string `json:"status"`
}

// ListWithdrawalsQuery 查询提现列表参数
type ListWithdrawalsQuery struct {
	PaginationQuery
	TimeRangeQuery
	Token  string `form:"token"`
	Status string `form:"status"`
}

// ListWithdrawalsRequest 提现列表请求（Handler 用）
type ListWithdrawalsRequest struct {
	Wallet    string
	Token     string
	Status    string
	Page      int
	PageSize  int
	StartTime int64
	EndTime   int64
}

// WithdrawalResponse 提现响应
type WithdrawalResponse struct {
	WithdrawID   string `json:"withdraw_id"`
	Wallet       string `json:"wallet"`
	Token        string `json:"token"`
	Amount       string `json:"amount"`
	ToAddress    string `json:"to_address"`
	Nonce        uint64 `json:"nonce"`
	Status       string `json:"status"`
	TxHash       string `json:"tx_hash,omitempty"`
	RejectReason string `json:"reject_reason,omitempty"`
	SubmittedAt  int64  `json:"submitted_at,omitempty"`
	ConfirmedAt  int64  `json:"confirmed_at,omitempty"`
	RefundedAt   int64  `json:"refunded_at,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

// ========== 成交相关 ==========

// ListTradesQuery 查询成交列表参数
type ListTradesQuery struct {
	PaginationQuery
	TimeRangeQuery
	Market string `form:"market"`
}

// ListTradesRequest 成交列表请求（Handler 用）
type ListTradesRequest struct {
	Wallet    string
	Market    string
	OrderID   string
	Page      int
	PageSize  int
	StartTime int64
	EndTime   int64
}

// TradeResponse 成交响应
type TradeResponse struct {
	TradeID          string `json:"trade_id"`
	Market           string `json:"market"`
	MakerOrderID     string `json:"maker_order_id"`
	TakerOrderID     string `json:"taker_order_id"`
	MakerWallet      string `json:"maker_wallet,omitempty"`
	TakerWallet      string `json:"taker_wallet,omitempty"`
	Price            string `json:"price"`
	Amount           string `json:"amount"`
	QuoteAmount      string `json:"quote_amount"`
	MakerFee         string `json:"maker_fee,omitempty"`
	TakerFee         string `json:"taker_fee,omitempty"`
	FeeToken         string `json:"fee_token,omitempty"`
	Side             string `json:"side"`
	SettlementStatus string `json:"settlement_status,omitempty"`
	MatchedAt        int64  `json:"matched_at"`
	SettledAt        int64  `json:"settled_at,omitempty"`
}

// ========== 行情相关 ==========

// MarketResponse 市场响应
type MarketResponse struct {
	Symbol         string `json:"symbol"`
	BaseToken      string `json:"base_token"`
	QuoteToken     string `json:"quote_token"`
	PriceDecimals  int    `json:"price_decimals"`
	AmountDecimals int    `json:"amount_decimals"`
	MinAmount      string `json:"min_amount"`
	MaxAmount      string `json:"max_amount,omitempty"`
	MinNotional    string `json:"min_notional"`
	TickSize       string `json:"tick_size,omitempty"`
	MakerFeeRate   string `json:"maker_fee_rate"`
	TakerFeeRate   string `json:"taker_fee_rate"`
	IsActive       bool   `json:"is_active"`
}

// TickerResponse Ticker 响应
type TickerResponse struct {
	Market             string `json:"market"`
	LastPrice          string `json:"last_price"`
	PriceChange        string `json:"price_change"`
	PriceChangePercent string `json:"price_change_percent"`
	High               string `json:"high"`
	Low                string `json:"low"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quote_volume"`
	Open               string `json:"open"`
	BestBid            string `json:"best_bid"`
	BestBidQty         string `json:"best_bid_qty,omitempty"`
	BestAsk            string `json:"best_ask"`
	BestAskQty         string `json:"best_ask_qty,omitempty"`
	TradeCount         int    `json:"trade_count,omitempty"`
	Timestamp          int64  `json:"timestamp"`
}

// DepthQuery 深度查询参数
type DepthQuery struct {
	Limit int `form:"limit"`
}

// DepthResponse 深度响应
type DepthResponse struct {
	Market    string     `json:"market"`
	Bids      [][]string `json:"bids"`
	Asks      [][]string `json:"asks"`
	Sequence  uint64     `json:"sequence,omitempty"`
	Timestamp int64      `json:"timestamp"`
}

// KlinesQuery K线查询参数
type KlinesQuery struct {
	Interval  string `form:"interval" binding:"required"`
	StartTime int64  `form:"start_time"`
	EndTime   int64  `form:"end_time"`
	Limit     int    `form:"limit"`
}

// KlineResponse K线响应
type KlineResponse struct {
	Market      string `json:"market,omitempty"`
	Interval    string `json:"interval,omitempty"`
	OpenTime    int64  `json:"open_time"`
	Open        string `json:"open"`
	High        string `json:"high"`
	Low         string `json:"low"`
	Close       string `json:"close"`
	Volume      string `json:"volume"`
	QuoteVolume string `json:"quote_volume"`
	CloseTime   int64  `json:"close_time"`
	TradeCount  int    `json:"trade_count"`
}

// RecentTradesQuery 最近成交查询参数
type RecentTradesQuery struct {
	Limit int `form:"limit"`
}

// RecentTradeResponse 最近成交响应
type RecentTradeResponse struct {
	TradeID     string `json:"trade_id"`
	Market      string `json:"market,omitempty"`
	Price       string `json:"price"`
	Amount      string `json:"amount"`
	QuoteAmount string `json:"quote_amount,omitempty"`
	Side        string `json:"side"`
	Timestamp   int64  `json:"timestamp"`
}
