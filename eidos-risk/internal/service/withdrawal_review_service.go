// Package service provides withdrawal review workflow service
package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// WithdrawalReviewStatus represents the review state
type WithdrawalReviewStatus string

const (
	ReviewStatusPending   WithdrawalReviewStatus = "pending"
	ReviewStatusApproved  WithdrawalReviewStatus = "approved"
	ReviewStatusRejected  WithdrawalReviewStatus = "rejected"
	ReviewStatusExpired   WithdrawalReviewStatus = "expired"
	ReviewStatusCancelled WithdrawalReviewStatus = "cancelled"
)

// ReviewTransition represents a valid state transition
type ReviewTransition struct {
	From   WithdrawalReviewStatus
	To     WithdrawalReviewStatus
	Action string
}

var validTransitions = []ReviewTransition{
	{From: ReviewStatusPending, To: ReviewStatusApproved, Action: "approve"},
	{From: ReviewStatusPending, To: ReviewStatusRejected, Action: "reject"},
	{From: ReviewStatusPending, To: ReviewStatusExpired, Action: "expire"},
	{From: ReviewStatusPending, To: ReviewStatusCancelled, Action: "cancel"},
}

// WithdrawalReviewService manages withdrawal review workflow
type WithdrawalReviewService struct {
	mu sync.RWMutex

	repo *repository.WithdrawalReviewRepository

	// Configuration
	autoApproveThreshold int           // Risk score threshold for auto-approve
	autoRejectThreshold  int           // Risk score threshold for auto-reject
	reviewTimeout        time.Duration // Review timeout duration

	// Callbacks
	onApproved  func(ctx context.Context, review *model.WithdrawalReview) error
	onRejected  func(ctx context.Context, review *model.WithdrawalReview) error
	onExpired   func(ctx context.Context, review *model.WithdrawalReview) error
	onRiskAlert func(ctx context.Context, alert *RiskAlertMessage) error
}

// NewWithdrawalReviewService creates a new withdrawal review service
func NewWithdrawalReviewService(repo *repository.WithdrawalReviewRepository) *WithdrawalReviewService {
	return &WithdrawalReviewService{
		repo:                 repo,
		autoApproveThreshold: 30,
		autoRejectThreshold:  80,
		reviewTimeout:        24 * time.Hour,
	}
}

// SetConfig sets the service configuration
func (s *WithdrawalReviewService) SetConfig(autoApprove, autoReject int, timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.autoApproveThreshold = autoApprove
	s.autoRejectThreshold = autoReject
	s.reviewTimeout = timeout
}

// SetOnApproved sets the callback for approved withdrawals
func (s *WithdrawalReviewService) SetOnApproved(fn func(ctx context.Context, review *model.WithdrawalReview) error) {
	s.onApproved = fn
}

// SetOnRejected sets the callback for rejected withdrawals
func (s *WithdrawalReviewService) SetOnRejected(fn func(ctx context.Context, review *model.WithdrawalReview) error) {
	s.onRejected = fn
}

// SetOnExpired sets the callback for expired withdrawals
func (s *WithdrawalReviewService) SetOnExpired(fn func(ctx context.Context, review *model.WithdrawalReview) error) {
	s.onExpired = fn
}

// SetOnRiskAlert sets the risk alert callback
func (s *WithdrawalReviewService) SetOnRiskAlert(fn func(ctx context.Context, alert *RiskAlertMessage) error) {
	s.onRiskAlert = fn
}

// CreateReviewRequest represents a request to create a review
type CreateReviewRequest struct {
	WithdrawalID  string
	WalletAddress string
	Token         string
	Amount        decimal.Decimal
	ToAddress     string
	RiskScore     int
	RiskFactors   []model.RiskFactorEntry
}

// CreateReview creates a new withdrawal review
func (s *WithdrawalReviewService) CreateReview(ctx context.Context, req *CreateReviewRequest) (*model.WithdrawalReview, error) {
	now := time.Now().UnixMilli()

	// Determine auto decision
	autoDecision := model.AutoDecisionManualReview
	if req.RiskScore < s.autoApproveThreshold {
		autoDecision = model.AutoDecisionAutoApprove
	} else if req.RiskScore >= s.autoRejectThreshold {
		autoDecision = model.AutoDecisionAutoReject
	}

	// Serialize risk factors
	riskFactorsJSON, _ := json.Marshal(req.RiskFactors)

	review := &model.WithdrawalReview{
		ReviewID:      uuid.New().String(),
		WithdrawalID:  req.WithdrawalID,
		WalletAddress: req.WalletAddress,
		Token:         req.Token,
		Amount:        req.Amount,
		ToAddress:     req.ToAddress,
		RiskScore:     req.RiskScore,
		RiskFactors:   string(riskFactorsJSON),
		AutoDecision:  autoDecision,
		Status:        model.WithdrawalReviewStatusPending,
		CreatedAt:     now,
		ExpiresAt:     now + int64(s.reviewTimeout.Milliseconds()),
	}

	if err := s.repo.Create(ctx, review); err != nil {
		return nil, err
	}

	// Send alert for high-risk withdrawals
	severity := "info"
	if req.RiskScore >= 70 {
		severity = "critical"
	} else if req.RiskScore >= 50 {
		severity = "warning"
	}

	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      req.WalletAddress,
		AlertType:   "WITHDRAWAL_REVIEW_CREATED",
		Severity:    severity,
		Description: "Withdrawal review created",
		Context: map[string]string{
			"withdrawal_id": req.WithdrawalID,
			"review_id":     review.ReviewID,
			"amount":        req.Amount.String(),
			"token":         req.Token,
			"to_address":    req.ToAddress,
			"risk_score":    string(rune(req.RiskScore)),
			"auto_decision": string(autoDecision),
		},
		CreatedAt: now,
	})

	logger.Info("withdrawal review created",
		"review_id", review.ReviewID,
		"wallet", req.WalletAddress,
		"risk_score", req.RiskScore,
		"auto_decision", string(autoDecision))

	// Handle auto-approve for low-risk withdrawals
	if autoDecision == model.AutoDecisionAutoApprove {
		go func() {
			time.Sleep(100 * time.Millisecond) // Small delay for consistency
			if err := s.Approve(context.Background(), review.ReviewID, "system", "Auto-approved (low risk)"); err != nil {
				logger.Error("auto-approve failed", "error", err)
			}
		}()
	}

	return review, nil
}

// Approve approves a withdrawal review
func (s *WithdrawalReviewService) Approve(ctx context.Context, reviewID, reviewer, comment string) error {
	// Validate state transition
	review, err := s.repo.GetByReviewID(ctx, reviewID)
	if err != nil {
		return err
	}

	if !s.isValidTransition(WithdrawalReviewStatus(review.Status), ReviewStatusApproved) {
		return errors.New("invalid state transition: cannot approve from current status")
	}

	// Perform approval
	if err := s.repo.Approve(ctx, reviewID, reviewer, comment); err != nil {
		return err
	}

	// Refresh review
	review, _ = s.repo.GetByReviewID(ctx, reviewID)

	// Trigger callback
	if s.onApproved != nil {
		if err := s.onApproved(ctx, review); err != nil {
			logger.Error("onApproved callback failed", "error", err)
		}
	}

	// Send alert
	now := time.Now().UnixMilli()
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      review.WalletAddress,
		AlertType:   "WITHDRAWAL_APPROVED",
		Severity:    "info",
		Description: "Withdrawal approved: " + comment,
		Context: map[string]string{
			"withdrawal_id": review.WithdrawalID,
			"review_id":     reviewID,
			"reviewer":      reviewer,
			"amount":        review.Amount.String(),
		},
		CreatedAt: now,
	})

	logger.Info("withdrawal review approved",
		"review_id", reviewID,
		"reviewer", reviewer)

	return nil
}

// Reject rejects a withdrawal review
func (s *WithdrawalReviewService) Reject(ctx context.Context, reviewID, reviewer, comment string) error {
	// Validate state transition
	review, err := s.repo.GetByReviewID(ctx, reviewID)
	if err != nil {
		return err
	}

	if !s.isValidTransition(WithdrawalReviewStatus(review.Status), ReviewStatusRejected) {
		return errors.New("invalid state transition: cannot reject from current status")
	}

	// Perform rejection
	if err := s.repo.Reject(ctx, reviewID, reviewer, comment); err != nil {
		return err
	}

	// Refresh review
	review, _ = s.repo.GetByReviewID(ctx, reviewID)

	// Trigger callback
	if s.onRejected != nil {
		if err := s.onRejected(ctx, review); err != nil {
			logger.Error("onRejected callback failed", "error", err)
		}
	}

	// Send alert
	now := time.Now().UnixMilli()
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     uuid.New().String(),
		Wallet:      review.WalletAddress,
		AlertType:   "WITHDRAWAL_REJECTED",
		Severity:    "warning",
		Description: "Withdrawal rejected: " + comment,
		Context: map[string]string{
			"withdrawal_id": review.WithdrawalID,
			"review_id":     reviewID,
			"reviewer":      reviewer,
			"amount":        review.Amount.String(),
		},
		CreatedAt: now,
	})

	logger.Info("withdrawal review rejected",
		"review_id", reviewID,
		"reviewer", reviewer,
		"reason", comment)

	return nil
}

// GetReview gets a withdrawal review by ID
func (s *WithdrawalReviewService) GetReview(ctx context.Context, reviewID string) (*model.WithdrawalReview, error) {
	return s.repo.GetByReviewID(ctx, reviewID)
}

// GetReviewByWithdrawalID gets a withdrawal review by withdrawal ID
func (s *WithdrawalReviewService) GetReviewByWithdrawalID(ctx context.Context, withdrawalID string) (*model.WithdrawalReview, error) {
	return s.repo.GetByWithdrawalID(ctx, withdrawalID)
}

// ListPendingReviews lists pending reviews
func (s *WithdrawalReviewService) ListPendingReviews(ctx context.Context, page, pageSize int) ([]*model.WithdrawalReview, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}
	return s.repo.ListPending(ctx, pagination)
}

// ListReviewsByWallet lists reviews for a wallet
func (s *WithdrawalReviewService) ListReviewsByWallet(ctx context.Context, wallet string, page, pageSize int) ([]*model.WithdrawalReview, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}
	return s.repo.ListByWallet(ctx, wallet, pagination)
}

// ProcessExpiredReviews processes expired reviews
func (s *WithdrawalReviewService) ProcessExpiredReviews(ctx context.Context) (approved, rejected int64, err error) {
	// Get expired reviews before processing
	expiredReviews, err := s.repo.ListExpired(ctx)
	if err != nil {
		return 0, 0, err
	}

	// Process expired
	approved, rejected, err = s.repo.AutoProcessExpired(ctx)
	if err != nil {
		return 0, 0, err
	}

	// Trigger callbacks for each expired review
	for _, review := range expiredReviews {
		if review.RiskScore < 30 && s.onApproved != nil {
			s.onApproved(ctx, review)
		} else if review.RiskScore >= 70 && s.onRejected != nil {
			s.onRejected(ctx, review)
		} else if s.onExpired != nil {
			s.onExpired(ctx, review)
		}
	}

	if approved > 0 || rejected > 0 {
		logger.Info("processed expired withdrawal reviews",
			"auto_approved", approved,
			"auto_rejected", rejected)
	}

	return approved, rejected, nil
}

// GetReviewStats gets review statistics
func (s *WithdrawalReviewService) GetReviewStats(ctx context.Context) (map[string]int64, error) {
	return s.repo.CountByStatus(ctx)
}

// isValidTransition checks if a state transition is valid
func (s *WithdrawalReviewService) isValidTransition(from, to WithdrawalReviewStatus) bool {
	for _, t := range validTransitions {
		if t.From == from && t.To == to {
			return true
		}
	}
	return false
}

// sendAlert sends a risk alert
func (s *WithdrawalReviewService) sendAlert(ctx context.Context, alert *RiskAlertMessage) {
	if s.onRiskAlert != nil {
		if err := s.onRiskAlert(ctx, alert); err != nil {
			logger.Error("failed to send risk alert",
				"alert_id", alert.AlertID,
				"error", err)
		}
	}
}

// StartBackgroundTasks starts background tasks for the service
func (s *WithdrawalReviewService) StartBackgroundTasks(ctx context.Context) {
	// Process expired reviews every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.ProcessExpiredReviews(ctx)
			}
		}
	}()

	logger.Info("withdrawal review background tasks started")
}
