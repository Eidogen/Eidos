package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// OutboxRepository outbox 消息仓库
type OutboxRepository struct {
	db *gorm.DB
}

// NewOutboxRepository 创建 outbox 仓库
func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// Create 创建 outbox 消息
func (r *OutboxRepository) Create(ctx context.Context, msg *model.OutboxMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// CreateWithTx 在事务中创建 outbox 消息
func (r *OutboxRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, msg *model.OutboxMessage) error {
	return tx.WithContext(ctx).Create(msg).Error
}

// FetchPending 获取待发送消息 (已废弃，请使用 FetchAndClaim)
// Deprecated: 多实例部署时会导致重复处理，请使用 FetchAndClaim
func (r *OutboxRepository) FetchPending(ctx context.Context, limit int) ([]*model.OutboxMessage, error) {
	return r.FetchAndClaim(ctx, limit)
}

// FetchAndClaim 原子获取并锁定待发送消息
// 使用 FOR UPDATE SKIP LOCKED 确保多实例不会处理同一消息
// 返回的消息状态会被更新为 processing，处理完成后需调用 MarkSent 或 MarkFailed
func (r *OutboxRepository) FetchAndClaim(ctx context.Context, limit int) ([]*model.OutboxMessage, error) {
	var messages []*model.OutboxMessage

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 使用 FOR UPDATE SKIP LOCKED 锁定待处理消息
		// SKIP LOCKED 会跳过已被其他事务锁定的行，避免等待
		var ids []int64
		err := tx.Raw(`
			SELECT id FROM outbox_messages
			WHERE status = ?
			ORDER BY created_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		`, model.OutboxStatusPending, limit).Scan(&ids).Error
		if err != nil {
			return fmt.Errorf("select pending messages: %w", err)
		}

		if len(ids) == 0 {
			return nil
		}

		// 2. 更新状态为 processing (标记已被当前实例认领)
		now := time.Now().UnixMilli()
		err = tx.Exec(`
			UPDATE outbox_messages
			SET status = 'processing', updated_at = ?
			WHERE id IN ?
		`, now, ids).Error
		if err != nil {
			return fmt.Errorf("update status to processing: %w", err)
		}

		// 3. 查询完整消息内容
		err = tx.Where("id IN ?", ids).Find(&messages).Error
		if err != nil {
			return fmt.Errorf("fetch claimed messages: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("fetch and claim messages failed: %w", err)
	}
	return messages, nil
}

// MarkSent 标记消息已发送
func (r *OutboxRepository) MarkSent(ctx context.Context, id int64) error {
	now := time.Now().UnixMilli()
	result := r.db.WithContext(ctx).
		Model(&model.OutboxMessage{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":  model.OutboxStatusSent,
			"sent_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("mark message sent failed: %w", result.Error)
	}
	return nil
}

// MarkFailed 标记消息发送失败
// 如果重试次数未超过上限，状态会回到 pending 等待下次重试
func (r *OutboxRepository) MarkFailed(ctx context.Context, id int64, err error) error {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
	}

	now := time.Now().UnixMilli()
	// 更新重试次数，如果超过最大重试次数则标记为 failed，否则回到 pending
	result := r.db.WithContext(ctx).Exec(`
		UPDATE outbox_messages
		SET retry_count = retry_count + 1,
		    last_error = ?,
		    updated_at = ?,
		    status = CASE WHEN retry_count + 1 >= max_retries THEN 'failed' ELSE 'pending' END
		WHERE id = ?
	`, errMsg, now, id)

	if result.Error != nil {
		return fmt.Errorf("mark message failed: %w", result.Error)
	}
	return nil
}

// RecoverStaleProcessing 恢复卡住的 processing 状态消息
// 当实例崩溃时，processing 状态的消息不会被释放，需要定期恢复
// staleThreshold: 超过此时间的 processing 消息视为卡住 (建议 5-10 分钟)
func (r *OutboxRepository) RecoverStaleProcessing(ctx context.Context, staleThreshold time.Duration) (int64, error) {
	threshold := time.Now().Add(-staleThreshold).UnixMilli()
	result := r.db.WithContext(ctx).Exec(`
		UPDATE outbox_messages
		SET status = 'pending', updated_at = ?
		WHERE status = 'processing' AND updated_at < ?
	`, time.Now().UnixMilli(), threshold)

	if result.Error != nil {
		return 0, fmt.Errorf("recover stale processing messages: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CleanSent 清理已发送的消息
func (r *OutboxRepository) CleanSent(ctx context.Context, beforeTime int64, batchSize int) (int64, error) {
	var totalDeleted int64

	for {
		result := r.db.WithContext(ctx).Exec(`
			DELETE FROM outbox_messages
			WHERE id IN (
				SELECT id FROM outbox_messages
				WHERE status = 'sent' AND sent_at < ?
				LIMIT ?
			)
		`, beforeTime, batchSize)

		if result.Error != nil {
			return totalDeleted, fmt.Errorf("clean sent messages failed: %w", result.Error)
		}

		totalDeleted += result.RowsAffected
		if result.RowsAffected < int64(batchSize) {
			break
		}
	}

	return totalDeleted, nil
}

// CountFailed 统计失败消息数量
func (r *OutboxRepository) CountFailed(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.OutboxMessage{}).
		Where("status = ?", model.OutboxStatusFailed).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count failed messages: %w", err)
	}
	return count, nil
}

// GetByAggregateID 根据聚合 ID 获取消息
func (r *OutboxRepository) GetByAggregateID(ctx context.Context, aggregateType, aggregateID string) (*model.OutboxMessage, error) {
	var msg model.OutboxMessage
	err := r.db.WithContext(ctx).
		Where("aggregate_type = ? AND aggregate_id = ?", aggregateType, aggregateID).
		First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}
