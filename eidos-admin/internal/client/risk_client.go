package client

import (
	"context"

	"google.golang.org/grpc"

	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
)

// RiskClient wraps the risk service gRPC client
type RiskClient struct {
	conn     *grpc.ClientConn
	client   riskv1.RiskServiceClient
	ownsConn bool // 是否拥有连接（用于关闭时判断）
}

// NewRiskClient creates a new risk client
func NewRiskClient(ctx context.Context, target string) (*RiskClient, error) {
	conn, err := dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &RiskClient{
		conn:     conn,
		client:   riskv1.NewRiskServiceClient(conn),
		ownsConn: true,
	}, nil
}

// NewRiskClientFromConn creates a risk client from an existing connection (service discovery mode)
func NewRiskClientFromConn(conn *grpc.ClientConn) *RiskClient {
	return &RiskClient{
		conn:     conn,
		client:   riskv1.NewRiskServiceClient(conn),
		ownsConn: false,
	}
}

// Close closes the connection
func (c *RiskClient) Close() error {
	if c.ownsConn && c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// === Blacklist Management ===

// AddToBlacklist adds a wallet to the blacklist
func (c *RiskClient) AddToBlacklist(ctx context.Context, req *riskv1.AddToBlacklistRequest) (*riskv1.AddToBlacklistResponse, error) {
	return c.client.AddToBlacklist(ctx, req)
}

// RemoveFromBlacklist removes a wallet from the blacklist
func (c *RiskClient) RemoveFromBlacklist(ctx context.Context, wallet, reason, operatorID string) (*riskv1.RemoveFromBlacklistResponse, error) {
	return c.client.RemoveFromBlacklist(ctx, &riskv1.RemoveFromBlacklistRequest{
		Wallet:     wallet,
		Reason:     reason,
		OperatorId: operatorID,
	})
}

// CheckBlacklist checks if a wallet is blacklisted
func (c *RiskClient) CheckBlacklist(ctx context.Context, wallet string) (*riskv1.CheckBlacklistResponse, error) {
	return c.client.CheckBlacklist(ctx, &riskv1.CheckBlacklistRequest{
		Wallet: wallet,
	})
}

// ListBlacklist retrieves the blacklist with filters
func (c *RiskClient) ListBlacklist(ctx context.Context, blacklistType, source, search string, page, pageSize int32) (*riskv1.ListBlacklistResponse, error) {
	return c.client.ListBlacklist(ctx, &riskv1.ListBlacklistRequest{
		BlacklistType: blacklistType,
		Source:        source,
		Search:        search,
		Pagination: &commonv1.PaginationRequest{
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// === Risk Rules Management ===

// ListRiskRules retrieves all risk rules
func (c *RiskClient) ListRiskRules(ctx context.Context, category string, enabledOnly bool) (*riskv1.ListRiskRulesResponse, error) {
	return c.client.ListRiskRules(ctx, &riskv1.ListRiskRulesRequest{
		Category:    category,
		EnabledOnly: enabledOnly,
	})
}

// GetRiskRule retrieves a specific risk rule
func (c *RiskClient) GetRiskRule(ctx context.Context, ruleID string) (*riskv1.RiskRule, error) {
	resp, err := c.client.GetRiskRule(ctx, &riskv1.GetRiskRuleRequest{
		RuleId: ruleID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Rule, nil
}

// UpdateRiskRule updates a risk rule
func (c *RiskClient) UpdateRiskRule(ctx context.Context, req *riskv1.UpdateRiskRuleRequest) (*riskv1.UpdateRiskRuleResponse, error) {
	return c.client.UpdateRiskRule(ctx, req)
}

// === Rate Limiting ===

// GetRateLimitStatus retrieves rate limit status for a wallet
func (c *RiskClient) GetRateLimitStatus(ctx context.Context, wallet, counterName string) (*riskv1.GetRateLimitStatusResponse, error) {
	return c.client.GetRateLimitStatus(ctx, &riskv1.GetRateLimitStatusRequest{
		Wallet:      wallet,
		CounterName: counterName,
	})
}

// ResetRateLimit resets rate limit counters for a wallet
func (c *RiskClient) ResetRateLimit(ctx context.Context, wallet, counterName, operatorID, reason string) (*riskv1.ResetRateLimitResponse, error) {
	return c.client.ResetRateLimit(ctx, &riskv1.ResetRateLimitRequest{
		Wallet:      wallet,
		CounterName: counterName,
		OperatorId:  operatorID,
		Reason:      reason,
	})
}

// === Risk Events ===

// ListRiskEvents retrieves risk events with filters
func (c *RiskClient) ListRiskEvents(ctx context.Context, req *riskv1.ListRiskEventsRequest) (*riskv1.ListRiskEventsResponse, error) {
	return c.client.ListRiskEvents(ctx, req)
}

// GetRiskEvent retrieves a specific risk event
func (c *RiskClient) GetRiskEvent(ctx context.Context, eventID string) (*riskv1.RiskEvent, error) {
	resp, err := c.client.GetRiskEvent(ctx, &riskv1.GetRiskEventRequest{
		EventId: eventID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Event, nil
}

// AcknowledgeRiskEvent acknowledges a risk event
func (c *RiskClient) AcknowledgeRiskEvent(ctx context.Context, eventID, operatorID, note string) (*riskv1.AcknowledgeRiskEventResponse, error) {
	return c.client.AcknowledgeRiskEvent(ctx, &riskv1.AcknowledgeRiskEventRequest{
		EventId:    eventID,
		OperatorId: operatorID,
		Note:       note,
	})
}

// === User Limits ===

// GetUserLimits retrieves limits for a user
func (c *RiskClient) GetUserLimits(ctx context.Context, wallet string) (*riskv1.GetUserLimitsResponse, error) {
	return c.client.GetUserLimits(ctx, &riskv1.GetUserLimitsRequest{
		Wallet: wallet,
	})
}

// SetUserLimits sets custom limits for a user
func (c *RiskClient) SetUserLimits(ctx context.Context, req *riskv1.SetUserLimitsRequest) (*riskv1.SetUserLimitsResponse, error) {
	return c.client.SetUserLimits(ctx, req)
}

// === Account Actions ===

// FreezeAccount freezes a user account
func (c *RiskClient) FreezeAccount(ctx context.Context, wallet, freezeType, reason, operatorID string, durationSeconds int64) (*riskv1.FreezeAccountResponse, error) {
	return c.client.FreezeAccount(ctx, &riskv1.FreezeAccountRequest{
		Wallet:          wallet,
		FreezeType:      freezeType,
		Reason:          reason,
		OperatorId:      operatorID,
		DurationSeconds: durationSeconds,
	})
}

// UnfreezeAccount unfreezes a user account
func (c *RiskClient) UnfreezeAccount(ctx context.Context, wallet, reason, operatorID string) (*riskv1.UnfreezeAccountResponse, error) {
	return c.client.UnfreezeAccount(ctx, &riskv1.UnfreezeAccountRequest{
		Wallet:     wallet,
		Reason:     reason,
		OperatorId: operatorID,
	})
}

// GetAccountStatus retrieves account risk status
func (c *RiskClient) GetAccountStatus(ctx context.Context, wallet string) (*riskv1.GetAccountStatusResponse, error) {
	return c.client.GetAccountStatus(ctx, &riskv1.GetAccountStatusRequest{
		Wallet: wallet,
	})
}

// === Risk Statistics ===

// GetRiskStats retrieves risk statistics
func (c *RiskClient) GetRiskStats(ctx context.Context, periodHours int32) (*riskv1.GetRiskStatsResponse, error) {
	return c.client.GetRiskStats(ctx, &riskv1.GetRiskStatsRequest{
		PeriodHours: periodHours,
	})
}

// === Pre-Trade Checks (for reference, usually called by trading service) ===

// CheckOrder validates an order against risk rules
func (c *RiskClient) CheckOrder(ctx context.Context, req *riskv1.CheckOrderRequest) (*riskv1.CheckOrderResponse, error) {
	return c.client.CheckOrder(ctx, req)
}

// CheckWithdrawal validates a withdrawal against risk rules
func (c *RiskClient) CheckWithdrawal(ctx context.Context, req *riskv1.CheckWithdrawalRequest) (*riskv1.CheckWithdrawalResponse, error) {
	return c.client.CheckWithdrawal(ctx, req)
}

// CheckTransaction validates a generic transaction
func (c *RiskClient) CheckTransaction(ctx context.Context, req *riskv1.CheckTransactionRequest) (*riskv1.CheckTransactionResponse, error) {
	return c.client.CheckTransaction(ctx, req)
}
