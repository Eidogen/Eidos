// Package service 提供业务逻辑服务
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ReconciliationTask 对账任务
type ReconciliationTask struct {
	TaskID        string    `json:"task_id"`
	WalletAddress string    `json:"wallet_address"` // 空表示全量
	Token         string    `json:"token"`          // 空表示所有 token
	Status        string    `json:"status"`         // running/completed/failed
	TotalChecked  int64     `json:"total_checked"`
	Discrepancies int64     `json:"discrepancies"`
	StartedAt     int64     `json:"started_at"`
	CompletedAt   int64     `json:"completed_at"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// BalanceProvider 余额提供者接口
// 由外部服务实现，用于获取链下余额
type BalanceProvider interface {
	// GetOffChainBalances 获取链下余额
	// 返回 map[wallet][token]Balance
	GetOffChainBalances(ctx context.Context, wallets []string) (map[string]map[string]*OffChainBalance, error)
	// GetAllWallets 获取所有活跃钱包
	GetAllWallets(ctx context.Context) ([]string, error)
	// GetTokens 获取所有支持的 token
	GetTokens(ctx context.Context) ([]string, error)
}

// OffChainBalance 链下余额
type OffChainBalance struct {
	Settled   decimal.Decimal `json:"settled"`
	Available decimal.Decimal `json:"available"`
	Frozen    decimal.Decimal `json:"frozen"`
	Pending   decimal.Decimal `json:"pending"`
}

// ReconciliationService 对账服务
type ReconciliationService struct {
	blockchainClient   *blockchain.Client
	reconciliationRepo repository.ReconciliationRepository
	balanceProvider    BalanceProvider

	// 任务管理
	tasks   map[string]*ReconciliationTask
	tasksMu sync.RWMutex

	// 配置
	batchSize int // 批量对账大小
}

// NewReconciliationService 创建对账服务
func NewReconciliationService(
	blockchainClient *blockchain.Client,
	reconciliationRepo repository.ReconciliationRepository,
	balanceProvider BalanceProvider,
) *ReconciliationService {
	return &ReconciliationService{
		blockchainClient:   blockchainClient,
		reconciliationRepo: reconciliationRepo,
		balanceProvider:    balanceProvider,
		tasks:              make(map[string]*ReconciliationTask),
		batchSize:          100,
	}
}

// TriggerReconciliation 触发对账任务
func (s *ReconciliationService) TriggerReconciliation(ctx context.Context, walletAddress, token string) (string, error) {
	taskID := uuid.New().String()

	task := &ReconciliationTask{
		TaskID:        taskID,
		WalletAddress: walletAddress,
		Token:         token,
		Status:        "running",
		StartedAt:     time.Now().UnixMilli(),
	}

	s.tasksMu.Lock()
	s.tasks[taskID] = task
	s.tasksMu.Unlock()

	// 异步执行对账
	go s.runReconciliation(context.Background(), task)

	logger.Info("reconciliation triggered",
		zap.String("task_id", taskID),
		zap.String("wallet", walletAddress),
		zap.String("token", token))

	return taskID, nil
}

// GetTaskStatus 获取任务状态
func (s *ReconciliationService) GetTaskStatus(taskID string) (*ReconciliationTask, error) {
	s.tasksMu.RLock()
	defer s.tasksMu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return task, nil
}

// runReconciliation 执行对账
func (s *ReconciliationService) runReconciliation(ctx context.Context, task *ReconciliationTask) {
	defer func() {
		if r := recover(); r != nil {
			task.Status = "failed"
			task.ErrorMessage = fmt.Sprintf("panic: %v", r)
			task.CompletedAt = time.Now().UnixMilli()
			logger.Error("reconciliation panic",
				zap.String("task_id", task.TaskID),
				zap.Any("panic", r))
		}
	}()

	var wallets []string
	var err error

	// 确定要检查的钱包
	if task.WalletAddress != "" {
		wallets = []string{task.WalletAddress}
	} else {
		if s.balanceProvider == nil {
			task.Status = "failed"
			task.ErrorMessage = "balance provider not configured"
			task.CompletedAt = time.Now().UnixMilli()
			return
		}
		wallets, err = s.balanceProvider.GetAllWallets(ctx)
		if err != nil {
			task.Status = "failed"
			task.ErrorMessage = fmt.Sprintf("get wallets: %v", err)
			task.CompletedAt = time.Now().UnixMilli()
			return
		}
	}

	// 获取要检查的 token 列表
	var tokens []string
	if task.Token != "" {
		tokens = []string{task.Token}
	} else if s.balanceProvider != nil {
		tokens, err = s.balanceProvider.GetTokens(ctx)
		if err != nil {
			task.Status = "failed"
			task.ErrorMessage = fmt.Sprintf("get tokens: %v", err)
			task.CompletedAt = time.Now().UnixMilli()
			return
		}
	} else {
		// 默认 token 列表
		tokens = []string{"USDC", "ETH"}
	}

	// 分批处理
	for i := 0; i < len(wallets); i += s.batchSize {
		end := i + s.batchSize
		if end > len(wallets) {
			end = len(wallets)
		}
		batch := wallets[i:end]

		if err := s.reconcileBatch(ctx, task, batch, tokens); err != nil {
			logger.Error("reconcile batch failed",
				zap.String("task_id", task.TaskID),
				zap.Int("batch_start", i),
				zap.Error(err))
			// 继续处理下一批
		}
	}

	task.Status = "completed"
	task.CompletedAt = time.Now().UnixMilli()

	logger.Info("reconciliation completed",
		zap.String("task_id", task.TaskID),
		zap.Int64("total_checked", task.TotalChecked),
		zap.Int64("discrepancies", task.Discrepancies))
}

// reconcileBatch 对账一批钱包
func (s *ReconciliationService) reconcileBatch(ctx context.Context, task *ReconciliationTask, wallets []string, tokens []string) error {
	// 获取链下余额
	var offChainBalances map[string]map[string]*OffChainBalance
	var err error

	if s.balanceProvider != nil {
		offChainBalances, err = s.balanceProvider.GetOffChainBalances(ctx, wallets)
		if err != nil {
			return fmt.Errorf("get off-chain balances: %w", err)
		}
	} else {
		// 没有配置 balance provider，使用空余额
		offChainBalances = make(map[string]map[string]*OffChainBalance)
		for _, wallet := range wallets {
			offChainBalances[wallet] = make(map[string]*OffChainBalance)
			for _, token := range tokens {
				offChainBalances[wallet][token] = &OffChainBalance{}
			}
		}
	}

	now := time.Now().UnixMilli()

	// 对比每个钱包的每个 token
	for _, wallet := range wallets {
		for _, token := range tokens {
			// 获取链上余额
			var onChainBalance decimal.Decimal
			if s.blockchainClient != nil {
				// 使用 BalanceAt 获取原生 ETH 余额
				// TODO: 对于 ERC20 token 需要调用合约的 balanceOf 方法
				walletAddr := common.HexToAddress(wallet)
				balanceBig, err := s.blockchainClient.BalanceAt(ctx, walletAddr, nil)
				if err != nil {
					logger.Warn("get on-chain balance failed",
						zap.String("wallet", wallet),
						zap.String("token", token),
						zap.Error(err))
					continue
				}
				// 将 big.Int 转换为 decimal (wei -> token, 18 decimals)
				onChainBalance = decimal.NewFromBigInt(balanceBig, -18)
			}

			// 获取链下余额
			offChain := &OffChainBalance{}
			if balances, ok := offChainBalances[wallet]; ok {
				if b, ok := balances[token]; ok {
					offChain = b
				}
			}

			// 计算差异
			// 链上余额应该 = 链下 settled + pending
			expectedOnChain := offChain.Settled.Add(offChain.Pending)
			difference := onChainBalance.Sub(expectedOnChain)

			// 确定状态
			status := model.ReconciliationStatusOK
			if !difference.IsZero() {
				status = model.ReconciliationStatusDiscrepancy
				task.Discrepancies++
			}

			// 创建对账记录
			record := &model.ReconciliationRecord{
				WalletAddress:     wallet,
				Token:             token,
				OnChainBalance:    onChainBalance,
				OffChainSettled:   offChain.Settled,
				OffChainAvailable: offChain.Available,
				OffChainFrozen:    offChain.Frozen,
				PendingSettle:     offChain.Pending,
				Difference:        difference,
				Status:            status,
				CheckedAt:         now,
			}

			if err := s.reconciliationRepo.Create(ctx, record); err != nil {
				logger.Error("create reconciliation record failed",
					zap.String("wallet", wallet),
					zap.String("token", token),
					zap.Error(err))
			}

			task.TotalChecked++
		}
	}

	return nil
}

// CleanupOldTasks 清理旧任务
func (s *ReconciliationService) CleanupOldTasks(maxAge time.Duration) {
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()

	cutoff := time.Now().Add(-maxAge).UnixMilli()
	for taskID, task := range s.tasks {
		if task.CompletedAt > 0 && task.CompletedAt < cutoff {
			delete(s.tasks, taskID)
		}
	}
}
