package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
)

var (
	ErrWithdrawalReviewNotFound  = errors.New("withdrawal review not found")
	ErrWithdrawalReviewDuplicate = errors.New("withdrawal review already exists")
)

// WithdrawalReviewRepository 提现审核仓储
type WithdrawalReviewRepository struct {
	db *gorm.DB
}

// NewWithdrawalReviewRepository 创建提现审核仓储
func NewWithdrawalReviewRepository(db *gorm.DB) *WithdrawalReviewRepository {
	return &WithdrawalReviewRepository{db: db}
}

// Create 创建提现审核记录
func (r *WithdrawalReviewRepository) Create(ctx context.Context, review *model.WithdrawalReview) error {
	if review.CreatedAt == 0 {
		review.CreatedAt = time.Now().UnixMilli()
	}

	result := r.db.WithContext(ctx).Create(review)
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			return ErrWithdrawalReviewDuplicate
		}
		return result.Error
	}
	return nil
}

// GetByReviewID 根据审核ID获取
func (r *WithdrawalReviewRepository) GetByReviewID(ctx context.Context, reviewID string) (*model.WithdrawalReview, error) {
	var review model.WithdrawalReview
	err := r.db.WithContext(ctx).
		Where("review_id = ?", reviewID).
		First(&review).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWithdrawalReviewNotFound
		}
		return nil, err
	}
	return &review, nil
}

// GetByWithdrawalID 根据提现ID获取
func (r *WithdrawalReviewRepository) GetByWithdrawalID(ctx context.Context, withdrawalID string) (*model.WithdrawalReview, error) {
	var review model.WithdrawalReview
	err := r.db.WithContext(ctx).
		Where("withdrawal_id = ?", withdrawalID).
		First(&review).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWithdrawalReviewNotFound
		}
		return nil, err
	}
	return &review, nil
}

// ListPending 查询待审核列表
func (r *WithdrawalReviewRepository) ListPending(ctx context.Context, pagination *Pagination) ([]*model.WithdrawalReview, int64, error) {
	var reviews []*model.WithdrawalReview
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Where("status = ?", model.WithdrawalReviewStatusPending)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("risk_score DESC, created_at ASC"). // 高风险优先，先创建的优先
		Find(&reviews).Error

	if err != nil {
		return nil, 0, err
	}
	return reviews, total, nil
}

// ListByWallet 根据钱包地址查询
func (r *WithdrawalReviewRepository) ListByWallet(ctx context.Context, walletAddress string, pagination *Pagination) ([]*model.WithdrawalReview, int64, error) {
	var reviews []*model.WithdrawalReview
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Where("wallet_address = ?", walletAddress)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&reviews).Error

	if err != nil {
		return nil, 0, err
	}
	return reviews, total, nil
}

// ListByStatus 根据状态查询
func (r *WithdrawalReviewRepository) ListByStatus(ctx context.Context, status model.WithdrawalReviewStatus, pagination *Pagination) ([]*model.WithdrawalReview, int64, error) {
	var reviews []*model.WithdrawalReview
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Where("status = ?", status)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&reviews).Error

	if err != nil {
		return nil, 0, err
	}
	return reviews, total, nil
}

// Approve 审核通过
func (r *WithdrawalReviewRepository) Approve(ctx context.Context, reviewID string, reviewer string, comment string) error {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Where("review_id = ?", reviewID).
		Where("status = ?", model.WithdrawalReviewStatusPending).
		Updates(map[string]interface{}{
			"status":         model.WithdrawalReviewStatusApproved,
			"reviewer":       reviewer,
			"review_comment": comment,
			"reviewed_at":    now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWithdrawalReviewNotFound
	}
	return nil
}

// Reject 审核拒绝
func (r *WithdrawalReviewRepository) Reject(ctx context.Context, reviewID string, reviewer string, comment string) error {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Where("review_id = ?", reviewID).
		Where("status = ?", model.WithdrawalReviewStatusPending).
		Updates(map[string]interface{}{
			"status":         model.WithdrawalReviewStatusRejected,
			"reviewer":       reviewer,
			"review_comment": comment,
			"reviewed_at":    now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWithdrawalReviewNotFound
	}
	return nil
}

// ListExpired 查询已过期的待审核记录
func (r *WithdrawalReviewRepository) ListExpired(ctx context.Context) ([]*model.WithdrawalReview, error) {
	var reviews []*model.WithdrawalReview
	now := time.Now().UnixMilli()

	err := r.db.WithContext(ctx).
		Where("status = ?", model.WithdrawalReviewStatusPending).
		Where("expires_at < ?", now).
		Find(&reviews).Error

	if err != nil {
		return nil, err
	}
	return reviews, nil
}

// AutoProcessExpired 自动处理过期记录
func (r *WithdrawalReviewRepository) AutoProcessExpired(ctx context.Context) (approved int64, rejected int64, err error) {
	now := time.Now().UnixMilli()

	// 低风险自动通过
	result := r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Where("status = ?", model.WithdrawalReviewStatusPending).
		Where("expires_at < ?", now).
		Where("risk_score < ?", 30). // 低风险阈值
		Updates(map[string]interface{}{
			"status":         model.WithdrawalReviewStatusApproved,
			"reviewer":       "system",
			"review_comment": "auto approved due to timeout (low risk)",
			"reviewed_at":    now,
		})
	if result.Error != nil {
		return 0, 0, result.Error
	}
	approved = result.RowsAffected

	// 高风险自动拒绝
	result = r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Where("status = ?", model.WithdrawalReviewStatusPending).
		Where("expires_at < ?", now).
		Where("risk_score >= ?", 70). // 高风险阈值
		Updates(map[string]interface{}{
			"status":         model.WithdrawalReviewStatusRejected,
			"reviewer":       "system",
			"review_comment": "auto rejected due to timeout (high risk)",
			"reviewed_at":    now,
		})
	if result.Error != nil {
		return approved, 0, result.Error
	}
	rejected = result.RowsAffected

	return approved, rejected, nil
}

// CountByStatus 按状态统计
func (r *WithdrawalReviewRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	type result struct {
		Status string
		Count  int64
	}
	var results []result

	err := r.db.WithContext(ctx).
		Model(&model.WithdrawalReview{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.Status] = r.Count
	}
	return counts, nil
}
