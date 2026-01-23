// Package handler 子账户 gRPC 处理器
package handler

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// SubAccountHandler 子账户 gRPC 服务处理器
// 实现 SubAccountServiceServer 接口
type SubAccountHandler struct {
	pb.UnimplementedSubAccountServiceServer
	subAccountService service.SubAccountService
}

// NewSubAccountHandler 创建子账户处理器
func NewSubAccountHandler(subAccountService service.SubAccountService) *SubAccountHandler {
	return &SubAccountHandler{
		subAccountService: subAccountService,
	}
}

// ========== 子账户管理接口 ==========

// CreateSubAccount 创建子账户
func (h *SubAccountHandler) CreateSubAccount(ctx context.Context, req *pb.CreateSubAccountRequest) (*pb.CreateSubAccountResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// 转换类型，默认为 Trading
	accountType := model.SubAccountTypeTrading
	if req.Type != commonv1.SubAccountType_SUB_ACCOUNT_TYPE_UNSPECIFIED {
		accountType = protoToModelSubAccountType(req.Type)
	}

	subAccount, err := h.subAccountService.CreateSubAccount(ctx, req.Wallet, req.Name, accountType)
	if err != nil {
		return nil, handleSubAccountError(err)
	}

	return &pb.CreateSubAccountResponse{
		SubAccount: modelToProtoSubAccount(subAccount),
	}, nil
}

// GetSubAccount 获取子账户详情
func (h *SubAccountHandler) GetSubAccount(ctx context.Context, req *pb.GetSubAccountRequest) (*pb.SubAccount, error) {
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}

	subAccount, err := h.subAccountService.GetSubAccount(ctx, req.SubAccountId)
	if err != nil {
		return nil, handleSubAccountError(err)
	}

	return modelToProtoSubAccount(subAccount), nil
}

// ListSubAccounts 获取钱包下所有子账户
func (h *SubAccountHandler) ListSubAccounts(ctx context.Context, req *pb.ListSubAccountsRequest) (*pb.ListSubAccountsResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	subAccounts, err := h.subAccountService.ListSubAccounts(ctx, req.Wallet)
	if err != nil {
		return nil, handleSubAccountError(err)
	}

	protoSubAccounts := make([]*pb.SubAccount, len(subAccounts))
	for i, sa := range subAccounts {
		protoSubAccounts[i] = modelToProtoSubAccount(sa)
	}

	return &pb.ListSubAccountsResponse{
		SubAccounts: protoSubAccounts,
	}, nil
}

// UpdateSubAccount 更新子账户
func (h *SubAccountHandler) UpdateSubAccount(ctx context.Context, req *pb.UpdateSubAccountRequest) (*emptypb.Empty, error) {
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}

	if err := h.subAccountService.UpdateSubAccount(ctx, req.SubAccountId, req.Name, req.Remark); err != nil {
		return nil, handleSubAccountError(err)
	}

	return &emptypb.Empty{}, nil
}

// DeleteSubAccount 删除子账户
func (h *SubAccountHandler) DeleteSubAccount(ctx context.Context, req *pb.DeleteSubAccountRequest) (*emptypb.Empty, error) {
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}

	if err := h.subAccountService.DeleteSubAccount(ctx, req.SubAccountId); err != nil {
		return nil, handleSubAccountError(err)
	}

	return &emptypb.Empty{}, nil
}

// FreezeSubAccount 冻结子账户
func (h *SubAccountHandler) FreezeSubAccount(ctx context.Context, req *pb.FreezeSubAccountRequest) (*emptypb.Empty, error) {
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}

	if err := h.subAccountService.FreezeSubAccount(ctx, req.SubAccountId, req.Reason); err != nil {
		return nil, handleSubAccountError(err)
	}

	return &emptypb.Empty{}, nil
}

// UnfreezeSubAccount 解冻子账户
func (h *SubAccountHandler) UnfreezeSubAccount(ctx context.Context, req *pb.UnfreezeSubAccountRequest) (*emptypb.Empty, error) {
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}

	if err := h.subAccountService.UnfreezeSubAccount(ctx, req.SubAccountId); err != nil {
		return nil, handleSubAccountError(err)
	}

	return &emptypb.Empty{}, nil
}

// SetDefaultSubAccount 设置默认子账户
func (h *SubAccountHandler) SetDefaultSubAccount(ctx context.Context, req *pb.SetDefaultSubAccountRequest) (*emptypb.Empty, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}

	if err := h.subAccountService.SetDefaultSubAccount(ctx, req.Wallet, req.SubAccountId); err != nil {
		return nil, handleSubAccountError(err)
	}

	return &emptypb.Empty{}, nil
}

// ========== 余额管理接口 ==========

// GetSubAccountBalance 获取子账户单个代币余额
func (h *SubAccountHandler) GetSubAccountBalance(ctx context.Context, req *pb.GetSubAccountBalanceRequest) (*pb.SubAccountBalance, error) {
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}

	balance, err := h.subAccountService.GetSubAccountBalance(ctx, req.SubAccountId, req.Token)
	if err != nil {
		return nil, handleSubAccountError(err)
	}

	return modelToProtoSubAccountBalance(balance), nil
}

// GetSubAccountBalances 获取子账户所有余额
func (h *SubAccountHandler) GetSubAccountBalances(ctx context.Context, req *pb.GetSubAccountBalancesRequest) (*pb.GetSubAccountBalancesResponse, error) {
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}

	balances, err := h.subAccountService.GetSubAccountBalances(ctx, req.SubAccountId)
	if err != nil {
		return nil, handleSubAccountError(err)
	}

	protoBalances := make([]*pb.SubAccountBalance, 0, len(balances))
	for _, b := range balances {
		// 如果需要隐藏零余额
		if req.HideZero && b.Total().IsZero() {
			continue
		}
		protoBalances = append(protoBalances, modelToProtoSubAccountBalance(b))
	}

	return &pb.GetSubAccountBalancesResponse{
		Balances: protoBalances,
	}, nil
}

// GetAllSubAccountBalances 获取钱包下所有子账户余额
func (h *SubAccountHandler) GetAllSubAccountBalances(ctx context.Context, req *pb.GetAllSubAccountBalancesRequest) (*pb.GetSubAccountBalancesResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	balances, err := h.subAccountService.GetAllSubAccountBalances(ctx, req.Wallet)
	if err != nil {
		return nil, handleSubAccountError(err)
	}

	protoBalances := make([]*pb.SubAccountBalance, 0, len(balances))
	for _, b := range balances {
		// 如果需要隐藏零余额
		if req.HideZero && b.Total().IsZero() {
			continue
		}
		protoBalances = append(protoBalances, modelToProtoSubAccountBalance(b))
	}

	return &pb.GetSubAccountBalancesResponse{
		Balances: protoBalances,
	}, nil
}

// ========== 划转接口 ==========

// TransferIn 划入子账户 (主账户 → 子账户)
func (h *SubAccountHandler) TransferIn(ctx context.Context, req *pb.TransferRequest) (*pb.TransferResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}
	if req.Amount == "" {
		return nil, status.Error(codes.InvalidArgument, "amount is required")
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	if err := h.subAccountService.TransferIn(ctx, req.Wallet, req.SubAccountId, req.Token, amount, req.Remark); err != nil {
		return nil, handleSubAccountError(err)
	}

	// 返回空的 transfer_id (由服务层生成，但未返回)
	// 可以通过查询划转历史获取
	return &pb.TransferResponse{
		TransferId: "",
	}, nil
}

// TransferOut 划出子账户 (子账户 → 主账户)
func (h *SubAccountHandler) TransferOut(ctx context.Context, req *pb.TransferRequest) (*pb.TransferResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.SubAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "sub_account_id is required")
	}
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}
	if req.Amount == "" {
		return nil, status.Error(codes.InvalidArgument, "amount is required")
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	if err := h.subAccountService.TransferOut(ctx, req.Wallet, req.SubAccountId, req.Token, amount, req.Remark); err != nil {
		return nil, handleSubAccountError(err)
	}

	return &pb.TransferResponse{
		TransferId: "",
	}, nil
}

// GetTransferHistory 获取划转历史
func (h *SubAccountHandler) GetTransferHistory(ctx context.Context, req *pb.GetTransferHistoryRequest) (*pb.GetTransferHistoryResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	// 构建过滤条件
	filter := &repository.SubAccountTransferFilter{
		SubAccountID: req.SubAccountId,
		Token:        req.Token,
	}

	if req.Type != commonv1.SubAccountTransferType_SUB_ACCOUNT_TRANSFER_TYPE_UNSPECIFIED {
		transferType := protoToModelSubAccountTransferType(req.Type)
		filter.Type = &transferType
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
	if page.Page <= 0 {
		page.Page = 1
	}
	if page.PageSize <= 0 {
		page.PageSize = 20
	}
	if page.PageSize > 100 {
		page.PageSize = 100
	}

	transfers, err := h.subAccountService.GetTransferHistory(ctx, req.Wallet, filter, page)
	if err != nil {
		return nil, handleSubAccountError(err)
	}

	protoTransfers := make([]*pb.SubAccountTransfer, len(transfers))
	for i, t := range transfers {
		protoTransfers[i] = modelToProtoSubAccountTransfer(t)
	}

	return &pb.GetTransferHistoryResponse{
		Transfers: protoTransfers,
		Total:     page.Total,
		Page:      int32(page.Page),
		PageSize:  int32(page.PageSize),
	}, nil
}

// ========== 类型转换函数 ==========

// protoToModelSubAccountType 将 proto 子账户类型转换为 model 类型
func protoToModelSubAccountType(t commonv1.SubAccountType) model.SubAccountType {
	switch t {
	case commonv1.SubAccountType_SUB_ACCOUNT_TYPE_TRADING:
		return model.SubAccountTypeTrading
	case commonv1.SubAccountType_SUB_ACCOUNT_TYPE_MARGIN:
		return model.SubAccountTypeMargin
	case commonv1.SubAccountType_SUB_ACCOUNT_TYPE_FUTURES:
		return model.SubAccountTypeFutures
	default:
		return model.SubAccountTypeTrading
	}
}

// modelToProtoSubAccountType 将 model 子账户类型转换为 proto 类型
func modelToProtoSubAccountType(t model.SubAccountType) commonv1.SubAccountType {
	switch t {
	case model.SubAccountTypeTrading:
		return commonv1.SubAccountType_SUB_ACCOUNT_TYPE_TRADING
	case model.SubAccountTypeMargin:
		return commonv1.SubAccountType_SUB_ACCOUNT_TYPE_MARGIN
	case model.SubAccountTypeFutures:
		return commonv1.SubAccountType_SUB_ACCOUNT_TYPE_FUTURES
	default:
		return commonv1.SubAccountType_SUB_ACCOUNT_TYPE_UNSPECIFIED
	}
}

// modelToProtoSubAccountStatus 将 model 子账户状态转换为 proto 状态
func modelToProtoSubAccountStatus(s model.SubAccountStatus) commonv1.SubAccountStatus {
	switch s {
	case model.SubAccountStatusActive:
		return commonv1.SubAccountStatus_SUB_ACCOUNT_STATUS_ACTIVE
	case model.SubAccountStatusFrozen:
		return commonv1.SubAccountStatus_SUB_ACCOUNT_STATUS_FROZEN
	case model.SubAccountStatusDeleted:
		return commonv1.SubAccountStatus_SUB_ACCOUNT_STATUS_DELETED
	default:
		return commonv1.SubAccountStatus_SUB_ACCOUNT_STATUS_UNSPECIFIED
	}
}

// protoToModelSubAccountTransferType 将 proto 划转类型转换为 model 类型
func protoToModelSubAccountTransferType(t commonv1.SubAccountTransferType) model.SubAccountTransferType {
	switch t {
	case commonv1.SubAccountTransferType_SUB_ACCOUNT_TRANSFER_TYPE_IN:
		return model.SubAccountTransferTypeIn
	case commonv1.SubAccountTransferType_SUB_ACCOUNT_TRANSFER_TYPE_OUT:
		return model.SubAccountTransferTypeOut
	default:
		return model.SubAccountTransferTypeIn
	}
}

// modelToProtoSubAccountTransferType 将 model 划转类型转换为 proto 类型
func modelToProtoSubAccountTransferType(t model.SubAccountTransferType) commonv1.SubAccountTransferType {
	switch t {
	case model.SubAccountTransferTypeIn:
		return commonv1.SubAccountTransferType_SUB_ACCOUNT_TRANSFER_TYPE_IN
	case model.SubAccountTransferTypeOut:
		return commonv1.SubAccountTransferType_SUB_ACCOUNT_TRANSFER_TYPE_OUT
	default:
		return commonv1.SubAccountTransferType_SUB_ACCOUNT_TRANSFER_TYPE_UNSPECIFIED
	}
}

// modelToProtoSubAccount 将 model 子账户转换为 proto
func modelToProtoSubAccount(sa *model.SubAccount) *pb.SubAccount {
	return &pb.SubAccount{
		SubAccountId: sa.SubAccountID,
		Wallet:       sa.Wallet,
		Name:         sa.Name,
		Type:         modelToProtoSubAccountType(sa.Type),
		Status:       modelToProtoSubAccountStatus(sa.Status),
		IsDefault:    sa.IsDefault,
		Remark:       sa.Remark,
		CreatedAt:    sa.CreatedAt,
		UpdatedAt:    sa.UpdatedAt,
	}
}

// modelToProtoSubAccountBalance 将 model 子账户余额转换为 proto
func modelToProtoSubAccountBalance(b *model.SubAccountBalance) *pb.SubAccountBalance {
	return &pb.SubAccountBalance{
		SubAccountId: b.SubAccountID,
		Wallet:       b.Wallet,
		Token:        b.Token,
		Available:    b.Available.String(),
		Frozen:       b.Frozen.String(),
		Total:        b.Total().String(),
		UpdatedAt:    b.UpdatedAt,
	}
}

// modelToProtoSubAccountTransfer 将 model 划转记录转换为 proto
func modelToProtoSubAccountTransfer(t *model.SubAccountTransfer) *pb.SubAccountTransfer {
	return &pb.SubAccountTransfer{
		TransferId:   t.TransferID,
		Wallet:       t.Wallet,
		SubAccountId: t.SubAccountID,
		Type:         modelToProtoSubAccountTransferType(t.Type),
		Token:        t.Token,
		Amount:       t.Amount.String(),
		Remark:       t.Remark,
		CreatedAt:    t.CreatedAt,
	}
}

// ========== 错误处理 ==========

// handleSubAccountError 将服务层错误转换为 gRPC 错误
func handleSubAccountError(err error) error {
	switch {
	case errors.Is(err, service.ErrSubAccountNameInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrSubAccountCannotTrade):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrSubAccountCannotDelete):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrMainAccountNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrInsufficientMainBalance):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrInsufficientBalance):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrInvalidAmount):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrInvalidToken):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, repository.ErrSubAccountNotFound):
		return status.Error(codes.NotFound, "sub-account not found")
	case errors.Is(err, repository.ErrSubAccountBalanceNotFound):
		return status.Error(codes.NotFound, "sub-account balance not found")
	case errors.Is(err, repository.ErrSubAccountExists):
		return status.Error(codes.AlreadyExists, "sub-account already exists")
	case errors.Is(err, repository.ErrSubAccountLimitExceeded):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, repository.ErrInsufficientBalance):
		return status.Error(codes.FailedPrecondition, "insufficient balance")
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
