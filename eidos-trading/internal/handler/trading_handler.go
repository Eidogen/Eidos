package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// TradingHandler gRPC 交易服务处理器
// 实现 TradingServiceServer 接口
type TradingHandler struct {
	pb.UnimplementedTradingServiceServer
	orderService      service.OrderService
	balanceService    service.BalanceService
	tradeService      service.TradeService
	clearingService   service.ClearingService
	depositService    service.DepositService
	withdrawalService service.WithdrawalService
	nonceRepo         repository.NonceRepository
	eip712Config      *config.EIP712Config
}

// NewTradingHandler 创建交易处理器
func NewTradingHandler(
	orderService service.OrderService,
	balanceService service.BalanceService,
	tradeService service.TradeService,
	clearingService service.ClearingService,
	depositService service.DepositService,
	withdrawalService service.WithdrawalService,
	nonceRepo repository.NonceRepository,
	eip712Config *config.EIP712Config,
) *TradingHandler {
	return &TradingHandler{
		orderService:      orderService,
		balanceService:    balanceService,
		tradeService:      tradeService,
		clearingService:   clearingService,
		depositService:    depositService,
		withdrawalService: withdrawalService,
		nonceRepo:         nonceRepo,
		eip712Config:      eip712Config,
	}
}

// ========== 订单接口实现 ==========

// PrepareOrder 准备订单签名数据
// 返回 EIP-712 类型数据供客户端签名
func (h *TradingHandler) PrepareOrder(ctx context.Context, req *pb.PrepareOrderRequest) (*pb.PrepareOrderResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}
	if req.Side == commonv1.OrderSide_ORDER_SIDE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "side is required")
	}
	if req.Type == commonv1.OrderType_ORDER_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "type is required")
	}
	if req.Amount == "" {
		return nil, status.Error(codes.InvalidArgument, "amount is required")
	}

	// 验证价格和数量格式
	if req.Type == commonv1.OrderType_ORDER_TYPE_LIMIT && req.Price == "" {
		return nil, status.Error(codes.InvalidArgument, "price is required for limit orders")
	}

	// 生成订单 ID
	orderID := uuid.New().String()

	// 获取下一个可用 nonce
	latestNonce, err := h.nonceRepo.GetLatestNonce(ctx, req.Wallet, model.NonceUsageOrder)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get nonce: %v", err)
	}
	nonce := latestNonce + 1

	// 设置过期时间 (默认 1 小时)
	expiresAt := time.Now().Add(time.Hour).UnixMilli()

	// 构建 EIP-712 类型数据
	typedData := h.buildOrderTypedData(req, orderID, nonce, expiresAt)

	return &pb.PrepareOrderResponse{
		OrderId:   orderID,
		Nonce:     nonce,
		ExpiresAt: expiresAt,
		TypedData: typedData,
	}, nil
}

// buildOrderTypedData 构建订单的 EIP-712 类型数据
func (h *TradingHandler) buildOrderTypedData(req *pb.PrepareOrderRequest, orderID string, nonce uint64, expiresAt int64) *pb.EIP712TypedData {
	// EIP712Domain 类型定义
	eip712DomainTypes := &pb.EIP712TypeList{
		Types: []*pb.EIP712Type{
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
	}

	// Order 类型定义
	orderTypes := &pb.EIP712TypeList{
		Types: []*pb.EIP712Type{
			{Name: "orderId", Type: "string"},
			{Name: "wallet", Type: "address"},
			{Name: "market", Type: "string"},
			{Name: "side", Type: "uint8"},
			{Name: "orderType", Type: "uint8"},
			{Name: "price", Type: "string"},
			{Name: "amount", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAt", Type: "uint256"},
		},
	}

	// 获取 side 和 type 的数值
	sideValue := uint8(req.Side)
	typeValue := uint8(req.Type)

	price := req.Price
	if price == "" {
		price = "0"
	}

	return &pb.EIP712TypedData{
		Types: map[string]*pb.EIP712TypeList{
			"EIP712Domain": eip712DomainTypes,
			"Order":        orderTypes,
		},
		PrimaryType: "Order",
		Domain: &pb.EIP712Domain{
			Name:              h.eip712Config.Domain.Name,
			Version:           h.eip712Config.Domain.Version,
			ChainId:           h.eip712Config.Domain.ChainID,
			VerifyingContract: h.eip712Config.Domain.VerifyingContract,
		},
		Message: map[string]string{
			"orderId":   orderID,
			"wallet":    req.Wallet,
			"market":    req.Market,
			"side":      strconv.FormatUint(uint64(sideValue), 10),
			"orderType": strconv.FormatUint(uint64(typeValue), 10),
			"price":     price,
			"amount":    req.Amount,
			"nonce":     strconv.FormatUint(nonce, 10),
			"expiresAt": strconv.FormatInt(expiresAt, 10),
		},
	}
}

// CreateOrder 创建订单
func (h *TradingHandler) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	// 解析价格和数量
	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid price format")
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	// 转换订单类型和方向
	side := protoToModelOrderSide(req.Side)
	orderType := protoToModelOrderType(req.Type)

	// 创建订单
	order, err := h.orderService.CreateOrder(ctx, &service.CreateOrderRequest{
		Wallet:        req.Wallet,
		Market:        req.Market,
		Side:          side,
		Type:          orderType,
		Price:         price,
		Amount:        amount,
		Nonce:         req.Nonce,
		Signature:     req.Signature,
		ClientOrderID: req.ClientOrderId,
		ExpireAt:      req.ExpireAt,
	})

	if err != nil {
		return nil, handleServiceError(err)
	}

	return &pb.CreateOrderResponse{
		Order: modelToProtoOrder(order),
	}, nil
}

// CancelOrder 取消订单
func (h *TradingHandler) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*emptypb.Empty, error) {
	if req.Wallet == "" || req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet and order_id are required")
	}

	if err := h.orderService.CancelOrder(ctx, req.Wallet, req.OrderId); err != nil {
		return nil, handleServiceError(err)
	}

	return &emptypb.Empty{}, nil
}

// BatchCancelOrders 批量取消订单
func (h *TradingHandler) BatchCancelOrders(ctx context.Context, req *pb.BatchCancelOrdersRequest) (*pb.BatchCancelOrdersResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	var orderIDs []string

	// 如果指定了订单ID列表，使用它们
	if len(req.OrderIds) > 0 {
		orderIDs = req.OrderIds
	} else {
		// 否则，查询符合条件的活跃订单
		orders, err := h.orderService.ListOpenOrders(ctx, req.Wallet, req.Market)
		if err != nil {
			return nil, handleServiceError(err)
		}

		// 根据方向筛选
		for _, order := range orders {
			if req.Side != commonv1.OrderSide_ORDER_SIDE_UNSPECIFIED {
				side := protoToModelOrderSide(req.Side)
				if order.Side != side {
					continue
				}
			}
			orderIDs = append(orderIDs, order.OrderID)
		}
	}

	// 批量取消
	var cancelledCount, failedCount int32
	var failures []*pb.CancelFailure

	for _, orderID := range orderIDs {
		err := h.orderService.CancelOrder(ctx, req.Wallet, orderID)
		if err != nil {
			failedCount++
			failures = append(failures, &pb.CancelFailure{
				OrderId:      orderID,
				ErrorCode:    "CANCEL_FAILED",
				ErrorMessage: err.Error(),
			})
		} else {
			cancelledCount++
		}
	}

	return &pb.BatchCancelOrdersResponse{
		CancelledCount: cancelledCount,
		FailedCount:    failedCount,
		Failures:       failures,
	}, nil
}

// GetOrder 获取订单详情
func (h *TradingHandler) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.Order, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	order, err := h.orderService.GetOrder(ctx, req.OrderId)
	if err != nil {
		return nil, handleServiceError(err)
	}

	return modelToProtoOrder(order), nil
}

// ListOrders 获取订单列表
func (h *TradingHandler) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	// 构建过滤条件
	filter := &repository.OrderFilter{
		Market: req.Market,
	}
	if req.Side != commonv1.OrderSide_ORDER_SIDE_UNSPECIFIED {
		side := protoToModelOrderSide(req.Side)
		filter.Side = &side
	}
	if req.Type != commonv1.OrderType_ORDER_TYPE_UNSPECIFIED {
		orderType := protoToModelOrderType(req.Type)
		filter.Type = &orderType
	}
	if len(req.Statuses) > 0 {
		filter.Statuses = make([]model.OrderStatus, len(req.Statuses))
		for i, s := range req.Statuses {
			filter.Statuses[i] = protoToModelOrderStatus(s)
		}
	}
	if req.StartTime > 0 && req.EndTime > 0 {
		filter.TimeRange = &repository.TimeRange{
			Start: req.StartTime,
			End:   req.EndTime,
		}
	}

	// 分页
	page := &repository.Pagination{
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}

	orders, err := h.orderService.ListOrders(ctx, req.Wallet, filter, page)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoOrders := make([]*pb.Order, len(orders))
	for i, order := range orders {
		protoOrders[i] = modelToProtoOrder(order)
	}

	return &pb.ListOrdersResponse{
		Orders:   protoOrders,
		Total:    page.Total,
		Page:     int32(page.Page),
		PageSize: int32(page.PageSize),
	}, nil
}

// ListOpenOrders 获取活跃订单
func (h *TradingHandler) ListOpenOrders(ctx context.Context, req *pb.ListOpenOrdersRequest) (*pb.ListOrdersResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	orders, err := h.orderService.ListOpenOrders(ctx, req.Wallet, req.Market)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoOrders := make([]*pb.Order, len(orders))
	for i, order := range orders {
		protoOrders[i] = modelToProtoOrder(order)
	}

	return &pb.ListOrdersResponse{
		Orders:   protoOrders,
		Total:    int64(len(orders)),
		Page:     1,
		PageSize: int32(len(orders)),
	}, nil
}

// ========== 余额接口实现 ==========

// GetBalance 获取单个代币余额
func (h *TradingHandler) GetBalance(ctx context.Context, req *pb.GetBalanceRequest) (*pb.Balance, error) {
	if req.Wallet == "" || req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet and token are required")
	}

	balance, err := h.balanceService.GetBalance(ctx, req.Wallet, req.Token)
	if err != nil {
		return nil, handleServiceError(err)
	}

	return modelToProtoBalance(balance), nil
}

// GetBalances 获取所有余额
func (h *TradingHandler) GetBalances(ctx context.Context, req *pb.GetBalancesRequest) (*pb.GetBalancesResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	balances, err := h.balanceService.GetBalances(ctx, req.Wallet)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoBalances := make([]*pb.Balance, len(balances))
	for i, balance := range balances {
		protoBalances[i] = modelToProtoBalance(balance)
	}

	return &pb.GetBalancesResponse{
		Balances: protoBalances,
	}, nil
}

// GetBalanceLogs 获取余额流水
func (h *TradingHandler) GetBalanceLogs(ctx context.Context, req *pb.GetBalanceLogsRequest) (*pb.GetBalanceLogsResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	filter := &repository.BalanceLogFilter{
		Token:   req.Token,
		OrderID: req.OrderId,
	}
	if req.Type != commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNSPECIFIED {
		logType := protoToModelBalanceChangeType(req.Type)
		filter.Type = &logType
	}
	if req.StartTime > 0 && req.EndTime > 0 {
		filter.TimeRange = &repository.TimeRange{
			Start: req.StartTime,
			End:   req.EndTime,
		}
	}

	page := &repository.Pagination{
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}

	logs, err := h.balanceService.GetBalanceLogs(ctx, req.Wallet, filter, page)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoLogs := make([]*pb.BalanceLog, len(logs))
	for i, log := range logs {
		protoLogs[i] = modelToProtoBalanceLog(log)
	}

	return &pb.GetBalanceLogsResponse{
		Logs:     protoLogs,
		Total:    page.Total,
		Page:     int32(page.Page),
		PageSize: int32(page.PageSize),
	}, nil
}

// ========== 成交接口实现 ==========

// GetTrade 获取成交详情
func (h *TradingHandler) GetTrade(ctx context.Context, req *pb.GetTradeRequest) (*pb.Trade, error) {
	if req.TradeId == "" {
		return nil, status.Error(codes.InvalidArgument, "trade_id is required")
	}

	trade, err := h.tradeService.GetTrade(ctx, req.TradeId)
	if err != nil {
		return nil, handleServiceError(err)
	}

	return modelToProtoTrade(trade), nil
}

// ListTrades 获取成交列表
func (h *TradingHandler) ListTrades(ctx context.Context, req *pb.ListTradesRequest) (*pb.ListTradesResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	filter := &repository.TradeFilter{
		Market: req.Market,
	}
	if req.SettlementStatus != commonv1.SettlementStatus_SETTLEMENT_STATUS_UNSPECIFIED {
		settlementStatus := protoToModelSettlementStatus(req.SettlementStatus)
		filter.SettlementStatus = &settlementStatus
	}
	if req.StartTime > 0 && req.EndTime > 0 {
		filter.TimeRange = &repository.TimeRange{
			Start: req.StartTime,
			End:   req.EndTime,
		}
	}

	page := &repository.Pagination{
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}

	trades, err := h.tradeService.ListTrades(ctx, req.Wallet, filter, page)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoTrades := make([]*pb.Trade, len(trades))
	for i, trade := range trades {
		protoTrades[i] = modelToProtoTrade(trade)
	}

	return &pb.ListTradesResponse{
		Trades:   protoTrades,
		Total:    page.Total,
		Page:     int32(page.Page),
		PageSize: int32(page.PageSize),
	}, nil
}

// ListTradesByOrder 获取订单相关成交
func (h *TradingHandler) ListTradesByOrder(ctx context.Context, req *pb.ListTradesByOrderRequest) (*pb.ListTradesResponse, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	trades, err := h.tradeService.ListTradesByOrder(ctx, req.OrderId)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoTrades := make([]*pb.Trade, len(trades))
	for i, trade := range trades {
		protoTrades[i] = modelToProtoTrade(trade)
	}

	return &pb.ListTradesResponse{
		Trades:   protoTrades,
		Total:    int64(len(trades)),
		Page:     1,
		PageSize: int32(len(trades)),
	}, nil
}

// ========== 充值接口实现 ==========

// GetDeposit 获取充值详情
func (h *TradingHandler) GetDeposit(ctx context.Context, req *pb.GetDepositRequest) (*pb.Deposit, error) {
	if req.DepositId == "" {
		return nil, status.Error(codes.InvalidArgument, "deposit_id is required")
	}

	deposit, err := h.depositService.GetDeposit(ctx, req.DepositId)
	if err != nil {
		return nil, handleServiceError(err)
	}

	return modelToProtoDeposit(deposit), nil
}

// ListDeposits 获取充值列表
func (h *TradingHandler) ListDeposits(ctx context.Context, req *pb.ListDepositsRequest) (*pb.ListDepositsResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	filter := &repository.DepositFilter{
		Token: req.Token,
	}
	if req.Status != commonv1.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED {
		depositStatus := protoToModelDepositStatus(req.Status)
		filter.Status = &depositStatus
	}
	if req.StartTime > 0 && req.EndTime > 0 {
		filter.TimeRange = &repository.TimeRange{
			Start: req.StartTime,
			End:   req.EndTime,
		}
	}

	page := &repository.Pagination{
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}

	deposits, err := h.depositService.ListDeposits(ctx, req.Wallet, filter, page)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoDeposits := make([]*pb.Deposit, len(deposits))
	for i, deposit := range deposits {
		protoDeposits[i] = modelToProtoDeposit(deposit)
	}

	return &pb.ListDepositsResponse{
		Deposits: protoDeposits,
		Total:    page.Total,
		Page:     int32(page.Page),
		PageSize: int32(page.PageSize),
	}, nil
}

// ========== 提现接口实现 ==========

// CreateWithdrawal 创建提现
func (h *TradingHandler) CreateWithdrawal(ctx context.Context, req *pb.CreateWithdrawalRequest) (*pb.CreateWithdrawalResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	withdrawal, err := h.withdrawalService.CreateWithdrawal(ctx, &service.CreateWithdrawalRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    amount,
		ToAddress: req.ToAddress,
		Nonce:     req.Nonce,
		Signature: req.Signature,
	})

	if err != nil {
		return nil, handleServiceError(err)
	}

	return &pb.CreateWithdrawalResponse{
		Withdrawal: modelToProtoWithdrawal(withdrawal),
	}, nil
}

// CancelWithdrawal 取消提现
func (h *TradingHandler) CancelWithdrawal(ctx context.Context, req *pb.CancelWithdrawalRequest) (*emptypb.Empty, error) {
	if req.Wallet == "" || req.WithdrawId == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet and withdraw_id are required")
	}

	if err := h.withdrawalService.CancelWithdrawal(ctx, req.Wallet, req.WithdrawId); err != nil {
		return nil, handleServiceError(err)
	}

	return &emptypb.Empty{}, nil
}

// GetWithdrawal 获取提现详情
func (h *TradingHandler) GetWithdrawal(ctx context.Context, req *pb.GetWithdrawalRequest) (*pb.Withdrawal, error) {
	if req.WithdrawId == "" {
		return nil, status.Error(codes.InvalidArgument, "withdraw_id is required")
	}

	withdrawal, err := h.withdrawalService.GetWithdrawal(ctx, req.WithdrawId)
	if err != nil {
		return nil, handleServiceError(err)
	}

	return modelToProtoWithdrawal(withdrawal), nil
}

// ListWithdrawals 获取提现列表
func (h *TradingHandler) ListWithdrawals(ctx context.Context, req *pb.ListWithdrawalsRequest) (*pb.ListWithdrawalsResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	filter := &repository.WithdrawalFilter{
		Token: req.Token,
	}
	if req.Status != commonv1.WithdrawStatus_WITHDRAW_STATUS_UNSPECIFIED {
		withdrawStatus := protoToModelWithdrawStatus(req.Status)
		filter.Status = &withdrawStatus
	}
	if req.StartTime > 0 && req.EndTime > 0 {
		filter.TimeRange = &repository.TimeRange{
			Start: req.StartTime,
			End:   req.EndTime,
		}
	}

	page := &repository.Pagination{
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}

	withdrawals, err := h.withdrawalService.ListWithdrawals(ctx, req.Wallet, filter, page)
	if err != nil {
		return nil, handleServiceError(err)
	}

	protoWithdrawals := make([]*pb.Withdrawal, len(withdrawals))
	for i, withdrawal := range withdrawals {
		protoWithdrawals[i] = modelToProtoWithdrawal(withdrawal)
	}

	return &pb.ListWithdrawalsResponse{
		Withdrawals: protoWithdrawals,
		Total:       page.Total,
		Page:        int32(page.Page),
		PageSize:    int32(page.PageSize),
	}, nil
}

// ========== 内部接口实现 ==========

// ProcessTradeResult 处理撮合结果
// 使用 ClearingService 通过 Redis Lua 原子操作进行清算
func (h *TradingHandler) ProcessTradeResult(ctx context.Context, req *pb.ProcessTradeResultRequest) (*emptypb.Empty, error) {
	// 转换为 worker.TradeResultMessage 格式
	msg := &worker.TradeResultMessage{
		TradeID:      req.TradeId,
		Market:       req.Market,
		MakerOrderID: req.MakerOrderId,
		TakerOrderID: req.TakerOrderId,
		Maker:        req.MakerWallet,
		Taker:        req.TakerWallet,
		Price:        req.Price,
		Size:         req.Amount,
		QuoteAmount:  req.QuoteAmount,
		MakerFee:     req.MakerFee,
		TakerFee:     req.TakerFee,
		Timestamp:    req.MatchedAt,
		MakerIsBuyer: req.MakerSide == commonv1.OrderSide_ORDER_SIDE_BUY,
	}

	if err := h.clearingService.ProcessTradeResult(ctx, msg); err != nil {
		return nil, handleServiceError(err)
	}

	return &emptypb.Empty{}, nil
}

// ProcessDepositEvent 处理充值事件
func (h *TradingHandler) ProcessDepositEvent(ctx context.Context, req *pb.ProcessDepositEventRequest) (*emptypb.Empty, error) {
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid amount: %v", err)
	}

	err = h.depositService.ProcessDepositEvent(ctx, &service.DepositEvent{
		TxHash:     req.TxHash,
		LogIndex:   req.LogIndex,
		BlockNum:   req.BlockNum,
		Wallet:     req.Wallet,
		Token:      req.Token,
		Amount:     amount,
		DetectedAt: req.DetectedAt,
	})

	if err != nil {
		return nil, handleServiceError(err)
	}

	return &emptypb.Empty{}, nil
}

// ConfirmSettlement 确认结算
func (h *TradingHandler) ConfirmSettlement(ctx context.Context, req *pb.ConfirmSettlementRequest) (*emptypb.Empty, error) {
	if req.BatchId == "" || req.TxHash == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_id and tx_hash are required")
	}

	if err := h.tradeService.ConfirmSettlement(ctx, req.BatchId, req.TxHash); err != nil {
		return nil, handleServiceError(err)
	}

	return &emptypb.Empty{}, nil
}

// RollbackSettlement 回滚结算
// 当链上结算失败时调用
func (h *TradingHandler) RollbackSettlement(ctx context.Context, req *pb.RollbackSettlementRequest) (*emptypb.Empty, error) {
	if req.BatchId == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_id is required")
	}

	reason := req.ErrorMessage
	if req.ErrorCode != "" {
		reason = fmt.Sprintf("[%s] %s", req.ErrorCode, req.ErrorMessage)
	}

	if err := h.tradeService.FailSettlement(ctx, req.BatchId, reason); err != nil {
		return nil, handleServiceError(err)
	}

	return &emptypb.Empty{}, nil
}

// ========== 辅助函数 ==========

// handleServiceError 将服务层错误转换为 gRPC 错误
func handleServiceError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidOrder):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrOrderNotCancellable):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrDuplicateOrder):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, service.ErrInvalidNonce):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrInsufficientBalance):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrInvalidWithdrawal):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrWithdrawalNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrWithdrawalNotPending):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, repository.ErrOrderNotFound):
		return status.Error(codes.NotFound, "order not found")
	case errors.Is(err, repository.ErrTradeNotFound):
		return status.Error(codes.NotFound, "trade not found")
	case errors.Is(err, repository.ErrDepositNotFound):
		return status.Error(codes.NotFound, "deposit not found")
	case errors.Is(err, repository.ErrWithdrawalNotFound):
		return status.Error(codes.NotFound, "withdrawal not found")
	case errors.Is(err, repository.ErrBalanceNotFound):
		return status.Error(codes.NotFound, "balance not found")
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
