package service

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BlacklistService 黑名单服务
type BlacklistService struct {
	repo  *repository.BlacklistRepository
	cache *cache.BlacklistCache

	// 风控告警回调
	onRiskAlert func(ctx context.Context, alert *RiskAlertMessage) error
}

// NewBlacklistService 创建黑名单服务
func NewBlacklistService(
	repo *repository.BlacklistRepository,
	cache *cache.BlacklistCache,
) *BlacklistService {
	return &BlacklistService{
		repo:  repo,
		cache: cache,
	}
}

// SetOnRiskAlert 设置风控告警回调
func (s *BlacklistService) SetOnRiskAlert(fn func(ctx context.Context, alert *RiskAlertMessage) error) {
	s.onRiskAlert = fn
}

// AddToBlacklist 添加到黑名单
func (s *BlacklistService) AddToBlacklist(ctx context.Context, req *AddToBlacklistRequest) error {
	now := time.Now().UnixMilli()

	entry := &model.BlacklistEntry{
		WalletAddress:  req.Wallet,
		ListType:       model.BlacklistType(req.ListType),
		Reason:         req.Reason,
		Source:         model.BlacklistSource(req.Source),
		EffectiveFrom:  now,
		EffectiveUntil: req.ExpireAt,
		Status:         model.BlacklistStatusActive,
		CreatedBy:      req.OperatorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// 写入数据库
	if err := s.repo.Create(ctx, entry); err != nil {
		return err
	}

	// 更新缓存
	cacheEntry := &cache.BlacklistEntry{
		WalletAddress:  req.Wallet,
		ListType:       req.ListType,
		Reason:         req.Reason,
		EffectiveUntil: req.ExpireAt,
	}
	if err := s.cache.Set(ctx, cacheEntry); err != nil {
		logger.Error("failed to update blacklist cache", zap.Error(err))
	}

	// 发送告警
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      req.Wallet,
		AlertType:   "BLACKLIST_ADDED",
		Severity:    "critical",
		Description: "账户已被加入黑名单: " + req.Reason,
		Context: map[string]string{
			"list_type":   req.ListType,
			"source":      req.Source,
			"operator_id": req.OperatorID,
		},
		CreatedAt: now,
	})

	logger.Info("wallet added to blacklist",
		zap.String("wallet", req.Wallet),
		zap.String("reason", req.Reason),
		zap.String("operator", req.OperatorID))

	return nil
}

// RemoveFromBlacklist 从黑名单移除
func (s *BlacklistService) RemoveFromBlacklist(ctx context.Context, req *RemoveFromBlacklistRequest) error {
	now := time.Now().UnixMilli()

	// 更新数据库
	if err := s.repo.Remove(ctx, req.Wallet, req.OperatorID); err != nil {
		return err
	}

	// 更新缓存
	if err := s.cache.Remove(ctx, req.Wallet); err != nil {
		logger.Error("failed to remove from blacklist cache", zap.Error(err))
	}

	// 发送告警
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      req.Wallet,
		AlertType:   "BLACKLIST_REMOVED",
		Severity:    "info",
		Description: "账户已从黑名单移除: " + req.Reason,
		Context: map[string]string{
			"operator_id": req.OperatorID,
		},
		CreatedAt: now,
	})

	logger.Info("wallet removed from blacklist",
		zap.String("wallet", req.Wallet),
		zap.String("reason", req.Reason),
		zap.String("operator", req.OperatorID))

	return nil
}

// CheckBlacklist 检查是否在黑名单中
func (s *BlacklistService) CheckBlacklist(ctx context.Context, wallet string) (*CheckBlacklistResponse, error) {
	// 先查缓存
	entry, err := s.cache.Get(ctx, wallet)
	if err == nil && entry != nil {
		now := time.Now().UnixMilli()

		// 检查是否过期
		if entry.EffectiveUntil > 0 && now > entry.EffectiveUntil {
			return &CheckBlacklistResponse{
				IsBlacklisted: false,
			}, nil
		}

		return &CheckBlacklistResponse{
			IsBlacklisted: true,
			Reason:        entry.Reason,
			ExpireAt:      entry.EffectiveUntil,
		}, nil
	}

	// 缓存未命中，查数据库
	isBlacklisted, reason, expireAt, err := s.repo.CheckBlacklisted(ctx, wallet)
	if err != nil {
		return nil, err
	}

	// 如果在黑名单中，更新缓存
	if isBlacklisted {
		cacheEntry := &cache.BlacklistEntry{
			WalletAddress:  wallet,
			ListType:       "full",
			Reason:         reason,
			EffectiveUntil: expireAt,
		}
		s.cache.Set(ctx, cacheEntry)
	}

	return &CheckBlacklistResponse{
		IsBlacklisted: isBlacklisted,
		Reason:        reason,
		ExpireAt:      expireAt,
	}, nil
}

// ListBlacklist 获取黑名单列表
func (s *BlacklistService) ListBlacklist(ctx context.Context, page, pageSize int) ([]*model.BlacklistEntry, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}

	return s.repo.ListActive(ctx, pagination)
}

// SyncCacheFromDB 从数据库同步黑名单到缓存
func (s *BlacklistService) SyncCacheFromDB(ctx context.Context) error {
	// 获取所有活跃的黑名单
	entries, err := s.repo.GetAllActiveWallets(ctx)
	if err != nil {
		return err
	}

	// 清空缓存
	if err := s.cache.Clear(ctx); err != nil {
		logger.Error("failed to clear blacklist cache", zap.Error(err))
	}

	// 批量加载到缓存
	cacheEntries := make([]*cache.BlacklistEntry, 0, len(entries))
	for _, entry := range entries {
		cacheEntries = append(cacheEntries, &cache.BlacklistEntry{
			WalletAddress:  entry.WalletAddress,
			ListType:       string(entry.ListType),
			Reason:         entry.Reason,
			EffectiveUntil: entry.EffectiveUntil,
		})
	}

	if err := s.cache.LoadFromDB(ctx, cacheEntries); err != nil {
		return err
	}

	logger.Info("blacklist cache synced from database",
		zap.Int("count", len(cacheEntries)))

	return nil
}

// CleanupExpired 清理过期的黑名单条目
func (s *BlacklistService) CleanupExpired(ctx context.Context) (int64, error) {
	count, err := s.repo.CleanupExpired(ctx)
	if err != nil {
		return 0, err
	}

	if count > 0 {
		logger.Info("cleaned up expired blacklist entries",
			zap.Int64("count", count))

		// 重新同步缓存
		s.SyncCacheFromDB(ctx)
	}

	return count, nil
}

// sendAlert 发送风控告警
func (s *BlacklistService) sendAlert(ctx context.Context, alert *RiskAlertMessage) {
	if s.onRiskAlert != nil {
		if err := s.onRiskAlert(ctx, alert); err != nil {
			logger.Error("failed to send risk alert",
				zap.String("alert_id", alert.AlertID),
				zap.Error(err))
		}
	}
}

// AddToBlacklistRequest 添加黑名单请求
type AddToBlacklistRequest struct {
	Wallet     string
	ListType   string // trade, withdraw, full
	Reason     string
	Source     string // manual, auto, external
	ExpireAt   int64  // 0 表示永久
	OperatorID string
}

// RemoveFromBlacklistRequest 移除黑名单请求
type RemoveFromBlacklistRequest struct {
	Wallet     string
	Reason     string
	OperatorID string
}

// CheckBlacklistResponse 黑名单检查响应
type CheckBlacklistResponse struct {
	IsBlacklisted bool
	Reason        string
	ExpireAt      int64
}
