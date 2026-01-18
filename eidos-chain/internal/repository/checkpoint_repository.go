package repository

import (
	"context"
	"errors"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCheckpointNotFound = errors.New("checkpoint not found")
	ErrEventNotFound      = errors.New("event not found")
)

// CheckpointRepository 区块检查点仓储接口
type CheckpointRepository interface {
	// 检查点
	GetByChainID(ctx context.Context, chainID int64) (*model.BlockCheckpoint, error)
	Upsert(ctx context.Context, checkpoint *model.BlockCheckpoint) error
	UpdateBlockNumber(ctx context.Context, chainID int64, blockNumber int64, blockHash string) error

	// 链上事件
	CreateEvent(ctx context.Context, event *model.ChainEvent) error
	GetEventByTxHashAndLogIndex(ctx context.Context, txHash string, logIndex int) (*model.ChainEvent, error)
	ListUnprocessedEvents(ctx context.Context, chainID int64, limit int) ([]*model.ChainEvent, error)
	MarkEventProcessed(ctx context.Context, id int64) error
	ListEventsByBlockRange(ctx context.Context, chainID int64, startBlock, endBlock int64) ([]*model.ChainEvent, error)
}

// checkpointRepository 区块检查点仓储实现
type checkpointRepository struct {
	*Repository
}

// NewCheckpointRepository 创建区块检查点仓储
func NewCheckpointRepository(db *gorm.DB) CheckpointRepository {
	return &checkpointRepository{
		Repository: NewRepository(db),
	}
}

func (r *checkpointRepository) GetByChainID(ctx context.Context, chainID int64) (*model.BlockCheckpoint, error) {
	var checkpoint model.BlockCheckpoint
	err := r.DB(ctx).Where("chain_id = ?", chainID).First(&checkpoint).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCheckpointNotFound
	}
	if err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

func (r *checkpointRepository) Upsert(ctx context.Context, checkpoint *model.BlockCheckpoint) error {
	now := time.Now().UnixMilli()
	checkpoint.ProcessedAt = now
	checkpoint.UpdatedAt = now
	if checkpoint.CreatedAt == 0 {
		checkpoint.CreatedAt = now
	}

	return r.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chain_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"block_number", "block_hash", "processed_at", "updated_at"}),
	}).Create(checkpoint).Error
}

func (r *checkpointRepository) UpdateBlockNumber(ctx context.Context, chainID int64, blockNumber int64, blockHash string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.BlockCheckpoint{}).
		Where("chain_id = ?", chainID).
		Updates(map[string]interface{}{
			"block_number": blockNumber,
			"block_hash":   blockHash,
			"processed_at": now,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// 不存在则创建
		checkpoint := &model.BlockCheckpoint{
			ChainID:     chainID,
			BlockNumber: blockNumber,
			BlockHash:   blockHash,
			ProcessedAt: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		return r.DB(ctx).Create(checkpoint).Error
	}
	return nil
}

func (r *checkpointRepository) CreateEvent(ctx context.Context, event *model.ChainEvent) error {
	now := time.Now().UnixMilli()
	event.CreatedAt = now
	event.UpdatedAt = now
	return r.DB(ctx).Create(event).Error
}

func (r *checkpointRepository) GetEventByTxHashAndLogIndex(ctx context.Context, txHash string, logIndex int) (*model.ChainEvent, error) {
	var event model.ChainEvent
	err := r.DB(ctx).
		Where("tx_hash = ? AND log_index = ?", txHash, logIndex).
		First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *checkpointRepository) ListUnprocessedEvents(ctx context.Context, chainID int64, limit int) ([]*model.ChainEvent, error) {
	var events []*model.ChainEvent
	err := r.DB(ctx).
		Where("chain_id = ? AND processed = ?", chainID, false).
		Order("block_number ASC, log_index ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *checkpointRepository) MarkEventProcessed(ctx context.Context, id int64) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.ChainEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"processed":  true,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrEventNotFound
	}
	return nil
}

func (r *checkpointRepository) ListEventsByBlockRange(ctx context.Context, chainID int64, startBlock, endBlock int64) ([]*model.ChainEvent, error) {
	var events []*model.ChainEvent
	err := r.DB(ctx).
		Where("chain_id = ? AND block_number >= ? AND block_number <= ?", chainID, startBlock, endBlock).
		Order("block_number ASC, log_index ASC").
		Find(&events).Error
	return events, err
}
