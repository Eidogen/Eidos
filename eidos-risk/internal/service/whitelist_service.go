// Package service provides whitelist management service
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// WhitelistService provides whitelist management functionality
type WhitelistService struct {
	repo          *repository.WhitelistRepository
	withdrawCache *cache.WithdrawCache

	// Callbacks
	onRiskAlert func(ctx context.Context, alert *RiskAlertMessage) error
}

// NewWhitelistService creates a new whitelist service
func NewWhitelistService(
	repo *repository.WhitelistRepository,
	withdrawCache *cache.WithdrawCache,
) *WhitelistService {
	return &WhitelistService{
		repo:          repo,
		withdrawCache: withdrawCache,
	}
}

// SetOnRiskAlert sets the risk alert callback
func (s *WhitelistService) SetOnRiskAlert(fn func(ctx context.Context, alert *RiskAlertMessage) error) {
	s.onRiskAlert = fn
}

// AddAddressRequest represents a request to add an address to whitelist
type AddAddressRequest struct {
	Wallet        string
	TargetAddress string
	Label         string
	Remark        string
	OperatorID    string
	RequireApproval bool
	EffectiveUntil int64
}

// AddAddress adds an address to the whitelist
func (s *WhitelistService) AddAddress(ctx context.Context, req *AddAddressRequest) (*model.WhitelistEntry, error) {
	now := time.Now().UnixMilli()

	status := model.WhitelistStatusActive
	if req.RequireApproval {
		status = model.WhitelistStatusPending
	}

	entry := &model.WhitelistEntry{
		EntryID:        uuid.New().String(),
		WalletAddress:  req.Wallet,
		TargetAddress:  req.TargetAddress,
		ListType:       model.WhitelistTypeAddress,
		Status:         status,
		Label:          req.Label,
		Remark:         req.Remark,
		EffectiveFrom:  now,
		EffectiveUntil: req.EffectiveUntil,
		CreatedBy:      req.OperatorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		return nil, err
	}

	// Update cache if active
	if status == model.WhitelistStatusActive {
		if err := s.withdrawCache.AddToWhitelist(ctx, req.Wallet, req.TargetAddress); err != nil {
			logger.Error("failed to update whitelist cache", zap.Error(err))
		}
	}

	// Send alert
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      req.Wallet,
		AlertType:   "WHITELIST_ADDRESS_ADDED",
		Severity:    "info",
		Description: "Address added to whitelist: " + req.TargetAddress,
		Context: map[string]string{
			"target_address": req.TargetAddress,
			"operator":       req.OperatorID,
			"status":         string(status),
		},
		CreatedAt: now,
	})

	logger.Info("address added to whitelist",
		zap.String("wallet", req.Wallet),
		zap.String("target_address", req.TargetAddress),
		zap.String("status", string(status)))

	return entry, nil
}

// RemoveAddressRequest represents a request to remove an address from whitelist
type RemoveAddressRequest struct {
	EntryID    string
	Reason     string
	OperatorID string
}

// RemoveAddress removes an address from the whitelist
func (s *WhitelistService) RemoveAddress(ctx context.Context, req *RemoveAddressRequest) error {
	// Get the entry first
	entry, err := s.repo.GetByEntryID(ctx, req.EntryID)
	if err != nil {
		return err
	}

	// Deactivate the entry
	if err := s.repo.Deactivate(ctx, req.EntryID, req.OperatorID, req.Reason); err != nil {
		return err
	}

	// Update cache
	if err := s.withdrawCache.RemoveFromWhitelist(ctx, entry.WalletAddress, entry.TargetAddress); err != nil {
		logger.Error("failed to update whitelist cache", zap.Error(err))
	}

	// Send alert
	now := time.Now().UnixMilli()
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      entry.WalletAddress,
		AlertType:   "WHITELIST_ADDRESS_REMOVED",
		Severity:    "info",
		Description: "Address removed from whitelist: " + entry.TargetAddress,
		Context: map[string]string{
			"target_address": entry.TargetAddress,
			"operator":       req.OperatorID,
			"reason":         req.Reason,
		},
		CreatedAt: now,
	})

	logger.Info("address removed from whitelist",
		zap.String("wallet", entry.WalletAddress),
		zap.String("target_address", entry.TargetAddress))

	return nil
}

// AddVIPRequest represents a request to add a VIP user
type AddVIPRequest struct {
	Wallet         string
	Label          string
	Remark         string
	Metadata       *model.WhitelistMetadata
	OperatorID     string
	EffectiveUntil int64
}

// AddVIP adds a wallet to VIP whitelist
func (s *WhitelistService) AddVIP(ctx context.Context, req *AddVIPRequest) (*model.WhitelistEntry, error) {
	now := time.Now().UnixMilli()

	metadataJSON := ""
	if req.Metadata != nil {
		data, _ := json.Marshal(req.Metadata)
		metadataJSON = string(data)
	}

	entry := &model.WhitelistEntry{
		EntryID:        uuid.New().String(),
		WalletAddress:  req.Wallet,
		ListType:       model.WhitelistTypeVIP,
		Status:         model.WhitelistStatusActive,
		Label:          req.Label,
		Remark:         req.Remark,
		Metadata:       metadataJSON,
		EffectiveFrom:  now,
		EffectiveUntil: req.EffectiveUntil,
		CreatedBy:      req.OperatorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		return nil, err
	}

	// Send alert
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      req.Wallet,
		AlertType:   "VIP_ADDED",
		Severity:    "info",
		Description: "VIP status granted: " + req.Label,
		Context: map[string]string{
			"operator": req.OperatorID,
		},
		CreatedAt: now,
	})

	logger.Info("VIP status granted",
		zap.String("wallet", req.Wallet),
		zap.String("label", req.Label))

	return entry, nil
}

// AddMarketMakerRequest represents a request to add a market maker
type AddMarketMakerRequest struct {
	Wallet         string
	Label          string
	Remark         string
	Metadata       *model.WhitelistMetadata
	OperatorID     string
	EffectiveUntil int64
}

// AddMarketMaker adds a wallet to market maker whitelist
func (s *WhitelistService) AddMarketMaker(ctx context.Context, req *AddMarketMakerRequest) (*model.WhitelistEntry, error) {
	now := time.Now().UnixMilli()

	metadataJSON := ""
	if req.Metadata != nil {
		data, _ := json.Marshal(req.Metadata)
		metadataJSON = string(data)
	}

	entry := &model.WhitelistEntry{
		EntryID:        uuid.New().String(),
		WalletAddress:  req.Wallet,
		ListType:       model.WhitelistTypeMarketMaker,
		Status:         model.WhitelistStatusActive,
		Label:          req.Label,
		Remark:         req.Remark,
		Metadata:       metadataJSON,
		EffectiveFrom:  now,
		EffectiveUntil: req.EffectiveUntil,
		CreatedBy:      req.OperatorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		return nil, err
	}

	// Send alert
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      req.Wallet,
		AlertType:   "MARKET_MAKER_ADDED",
		Severity:    "info",
		Description: "Market maker status granted: " + req.Label,
		Context: map[string]string{
			"operator": req.OperatorID,
		},
		CreatedAt: now,
	})

	logger.Info("Market maker status granted",
		zap.String("wallet", req.Wallet),
		zap.String("label", req.Label))

	return entry, nil
}

// CheckAddressWhitelist checks if an address is whitelisted for a wallet
func (s *WhitelistService) CheckAddressWhitelist(ctx context.Context, wallet, targetAddress string) (bool, error) {
	// Check cache first
	inCache, err := s.withdrawCache.IsWhitelistedAddress(ctx, wallet, targetAddress)
	if err == nil && inCache {
		return true, nil
	}

	// Check database
	return s.repo.IsAddressWhitelisted(ctx, wallet, targetAddress)
}

// CheckVIP checks if a wallet is a VIP
func (s *WhitelistService) CheckVIP(ctx context.Context, wallet string) (bool, *model.WhitelistMetadata, error) {
	isVIP, err := s.repo.IsVIP(ctx, wallet)
	if err != nil || !isVIP {
		return false, nil, err
	}

	// Get VIP metadata
	entries, err := s.repo.GetByWalletAndType(ctx, wallet, model.WhitelistTypeVIP)
	if err != nil || len(entries) == 0 {
		return true, nil, nil
	}

	var metadata model.WhitelistMetadata
	if entries[0].Metadata != "" {
		json.Unmarshal([]byte(entries[0].Metadata), &metadata)
	}

	return true, &metadata, nil
}

// CheckMarketMaker checks if a wallet is a market maker
func (s *WhitelistService) CheckMarketMaker(ctx context.Context, wallet string) (bool, *model.WhitelistMetadata, error) {
	isMM, err := s.repo.IsMarketMaker(ctx, wallet)
	if err != nil || !isMM {
		return false, nil, err
	}

	// Get market maker metadata
	entries, err := s.repo.GetByWalletAndType(ctx, wallet, model.WhitelistTypeMarketMaker)
	if err != nil || len(entries) == 0 {
		return true, nil, nil
	}

	var metadata model.WhitelistMetadata
	if entries[0].Metadata != "" {
		json.Unmarshal([]byte(entries[0].Metadata), &metadata)
	}

	return true, &metadata, nil
}

// GetAddressWhitelist gets the address whitelist for a wallet
func (s *WhitelistService) GetAddressWhitelist(ctx context.Context, wallet string) ([]*model.WhitelistEntry, error) {
	return s.repo.GetAddressWhitelist(ctx, wallet)
}

// ListWhitelist lists whitelist entries
func (s *WhitelistService) ListWhitelist(ctx context.Context, listType model.WhitelistType, page, pageSize int) ([]*model.WhitelistEntry, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}

	if listType == "" {
		return s.repo.ListActive(ctx, pagination)
	}

	return s.repo.ListByType(ctx, listType, pagination)
}

// ListPending lists pending whitelist entries
func (s *WhitelistService) ListPending(ctx context.Context, page, pageSize int) ([]*model.WhitelistEntry, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}
	return s.repo.ListPending(ctx, pagination)
}

// ApproveEntry approves a pending whitelist entry
func (s *WhitelistService) ApproveEntry(ctx context.Context, entryID, approver string) error {
	// Get the entry first
	entry, err := s.repo.GetByEntryID(ctx, entryID)
	if err != nil {
		return err
	}

	// Approve the entry
	if err := s.repo.Approve(ctx, entryID, approver); err != nil {
		return err
	}

	// Update cache if it's an address whitelist
	if entry.ListType == model.WhitelistTypeAddress && entry.TargetAddress != "" {
		if err := s.withdrawCache.AddToWhitelist(ctx, entry.WalletAddress, entry.TargetAddress); err != nil {
			logger.Error("failed to update whitelist cache", zap.Error(err))
		}
	}

	// Send alert
	now := time.Now().UnixMilli()
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      entry.WalletAddress,
		AlertType:   "WHITELIST_APPROVED",
		Severity:    "info",
		Description: "Whitelist entry approved: " + string(entry.ListType),
		Context: map[string]string{
			"entry_id": entryID,
			"approver": approver,
		},
		CreatedAt: now,
	})

	logger.Info("whitelist entry approved",
		zap.String("entry_id", entryID),
		zap.String("approver", approver))

	return nil
}

// RejectEntry rejects a pending whitelist entry
func (s *WhitelistService) RejectEntry(ctx context.Context, entryID, rejector, reason string) error {
	// Get the entry first
	entry, err := s.repo.GetByEntryID(ctx, entryID)
	if err != nil {
		return err
	}

	// Reject the entry
	if err := s.repo.Reject(ctx, entryID, rejector, reason); err != nil {
		return err
	}

	// Send alert
	now := time.Now().UnixMilli()
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      entry.WalletAddress,
		AlertType:   "WHITELIST_REJECTED",
		Severity:    "info",
		Description: "Whitelist entry rejected: " + reason,
		Context: map[string]string{
			"entry_id": entryID,
			"rejector": rejector,
			"reason":   reason,
		},
		CreatedAt: now,
	})

	logger.Info("whitelist entry rejected",
		zap.String("entry_id", entryID),
		zap.String("rejector", rejector),
		zap.String("reason", reason))

	return nil
}

// SyncCacheFromDB syncs whitelist cache from database
func (s *WhitelistService) SyncCacheFromDB(ctx context.Context) error {
	// Get all active address whitelists
	entries, _, err := s.repo.ListByType(ctx, model.WhitelistTypeAddress, &repository.Pagination{Page: 1, PageSize: 10000})
	if err != nil {
		return err
	}

	// Update cache
	for _, entry := range entries {
		if entry.IsValid(time.Now().UnixMilli()) && entry.TargetAddress != "" {
			if err := s.withdrawCache.AddToWhitelist(ctx, entry.WalletAddress, entry.TargetAddress); err != nil {
				logger.Error("failed to sync whitelist entry to cache",
					zap.String("wallet", entry.WalletAddress),
					zap.Error(err))
			}
		}
	}

	logger.Info("whitelist cache synced from database",
		zap.Int("count", len(entries)))

	return nil
}

// CleanupExpired cleans up expired whitelist entries
func (s *WhitelistService) CleanupExpired(ctx context.Context) (int64, error) {
	count, err := s.repo.CleanupExpired(ctx)
	if err != nil {
		return 0, err
	}

	if count > 0 {
		logger.Info("cleaned up expired whitelist entries",
			zap.Int64("count", count))

		// Sync cache
		s.SyncCacheFromDB(ctx)
	}

	return count, nil
}

// sendAlert sends a risk alert
func (s *WhitelistService) sendAlert(ctx context.Context, alert *RiskAlertMessage) {
	if s.onRiskAlert != nil {
		if err := s.onRiskAlert(ctx, alert); err != nil {
			logger.Error("failed to send risk alert",
				zap.String("alert_id", alert.AlertID),
				zap.Error(err))
		}
	}
}
