// Package service 提供业务逻辑服务
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/contract"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ReconciliationTask 对账任务
type ReconciliationTask struct {
	TaskID        string `json:"task_id"`
	WalletAddress string `json:"wallet_address"` // 空表示全量
	Token         string `json:"token"`          // 空表示所有 token
	Status        string `json:"status"`         // running/completed/failed
	TotalChecked  int64  `json:"total_checked"`
	Discrepancies int64  `json:"discrepancies"`
	StartedAt     int64  `json:"started_at"`
	CompletedAt   int64  `json:"completed_at"`
	ErrorMessage  string `json:"error_message,omitempty"`
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

// ReconciliationReport 对账报告
type ReconciliationReport struct {
	TaskID          string                       `json:"task_id"`
	GeneratedAt     int64                        `json:"generated_at"`
	TotalChecked    int64                        `json:"total_checked"`
	TotalDiscrepant int64                        `json:"total_discrepant"`
	TotalAmount     decimal.Decimal              `json:"total_amount"`
	Discrepancies   []*ReconciliationDiscrepancy `json:"discrepancies"`
	Summary         *ReconciliationSummary       `json:"summary"`
}

// ReconciliationDiscrepancy 对账差异
type ReconciliationDiscrepancy struct {
	WalletAddress   string          `json:"wallet_address"`
	Token           string          `json:"token"`
	OnChainBalance  decimal.Decimal `json:"on_chain_balance"`
	OffChainBalance decimal.Decimal `json:"off_chain_balance"`
	Difference      decimal.Decimal `json:"difference"`
	CheckedAt       int64           `json:"checked_at"`
}

// ReconciliationSummary 对账摘要
type ReconciliationSummary struct {
	ByToken  map[string]*TokenSummary  `json:"by_token"`
	ByStatus map[string]int64          `json:"by_status"`
}

// TokenSummary Token 级别摘要
type TokenSummary struct {
	TotalChecked    int64           `json:"total_checked"`
	TotalDiscrepant int64           `json:"total_discrepant"`
	TotalDifference decimal.Decimal `json:"total_difference"`
	OnChainTotal    decimal.Decimal `json:"on_chain_total"`
	OffChainTotal   decimal.Decimal `json:"off_chain_total"`
}

// ReconciliationService 对账服务
type ReconciliationService struct {
	blockchainClient   *blockchain.Client
	reconciliationRepo repository.ReconciliationRepository
	balanceProvider    BalanceProvider

	// 合约绑定
	vaultContract *contract.VaultContract
	tokenRegistry *contract.TokenRegistry

	// 任务管理
	tasks   map[string]*ReconciliationTask
	tasksMu sync.RWMutex

	// 配置
	batchSize int // 批量对账大小
	chainID   int64
}

// ReconciliationServiceConfig 对账服务配置
type ReconciliationServiceConfig struct {
	BatchSize int
	ChainID   int64
}

// NewReconciliationService 创建对账服务
func NewReconciliationService(
	blockchainClient *blockchain.Client,
	reconciliationRepo repository.ReconciliationRepository,
	balanceProvider BalanceProvider,
	cfg *ReconciliationServiceConfig,
) *ReconciliationService {
	batchSize := 100
	if cfg != nil && cfg.BatchSize > 0 {
		batchSize = cfg.BatchSize
	}

	return &ReconciliationService{
		blockchainClient:   blockchainClient,
		reconciliationRepo: reconciliationRepo,
		balanceProvider:    balanceProvider,
		tasks:              make(map[string]*ReconciliationTask),
		batchSize:          batchSize,
		chainID:            cfg.ChainID,
	}
}

// SetVaultContract 设置 Vault 合约绑定
func (s *ReconciliationService) SetVaultContract(vault *contract.VaultContract) {
	s.vaultContract = vault
}

// SetTokenRegistry 设置 Token 注册表
func (s *ReconciliationService) SetTokenRegistry(reg *contract.TokenRegistry) {
	s.tokenRegistry = reg
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
		"task_id", taskID,
		"wallet", walletAddress,
		"token", token)

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
				"task_id", task.TaskID,
				"panic", r)
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
	} else if s.tokenRegistry != nil {
		tokens = s.tokenRegistry.GetAllSymbols()
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
				"task_id", task.TaskID,
				"batch_start", i,
				"error", err)
			// 继续处理下一批
		}
	}

	task.Status = "completed"
	task.CompletedAt = time.Now().UnixMilli()

	logger.Info("reconciliation completed",
		"task_id", task.TaskID,
		"total_checked", task.TotalChecked,
		"discrepancies", task.Discrepancies)
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
			onChainBalance, err := s.getOnChainBalance(ctx, wallet, token)
			if err != nil {
				logger.Warn("get on-chain balance failed",
					"wallet", wallet,
					"token", token,
					"error", err)
				continue
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
					"wallet", wallet,
					"token", token,
					"error", err)
			}

			task.TotalChecked++
		}
	}

	return nil
}

// getOnChainBalance 获取链上余额
func (s *ReconciliationService) getOnChainBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	walletAddr := common.HexToAddress(wallet)

	// 获取 token 信息
	var tokenInfo *contract.TokenInfo
	var decimals uint8 = 18 // 默认 18 位小数

	if s.tokenRegistry != nil {
		info, err := s.tokenRegistry.GetBySymbol(token)
		if err == nil {
			tokenInfo = info
			decimals = info.Decimals
		}
	}

	// 检查是否是原生 token (ETH)
	isNative := token == "ETH" || (tokenInfo != nil && tokenInfo.IsNative)

	var balanceBig *big.Int
	var err error

	if isNative {
		// 原生 ETH 余额
		if s.blockchainClient != nil {
			balanceBig, err = s.blockchainClient.BalanceAt(ctx, walletAddr, nil)
			if err != nil {
				return decimal.Zero, fmt.Errorf("get native balance: %w", err)
			}
		} else {
			return decimal.Zero, fmt.Errorf("blockchain client not configured")
		}
	} else if s.vaultContract != nil {
		// 从 Vault 合约查询余额
		var tokenAddr common.Address
		if tokenInfo != nil {
			tokenAddr = tokenInfo.Address
		} else if s.tokenRegistry != nil {
			addr, err := s.tokenRegistry.GetAddress(token)
			if err != nil {
				return decimal.Zero, fmt.Errorf("unknown token: %s", token)
			}
			tokenAddr = addr
		} else {
			return decimal.Zero, fmt.Errorf("token registry not configured")
		}

		balanceBig, err = s.vaultContract.GetBalance(ctx, walletAddr, tokenAddr)
		if err != nil {
			return decimal.Zero, fmt.Errorf("get vault balance: %w", err)
		}
	} else if s.tokenRegistry != nil {
		// 使用 Token Registry 查询 ERC20 余额
		var tokenAddr common.Address
		if tokenInfo != nil {
			tokenAddr = tokenInfo.Address
		} else {
			addr, err := s.tokenRegistry.GetAddress(token)
			if err != nil {
				return decimal.Zero, fmt.Errorf("unknown token: %s", token)
			}
			tokenAddr = addr
		}

		balanceBig, err = s.tokenRegistry.BalanceOf(ctx, tokenAddr, walletAddr)
		if err != nil {
			return decimal.Zero, fmt.Errorf("get erc20 balance: %w", err)
		}
	} else {
		return decimal.Zero, fmt.Errorf("no balance source configured for token: %s", token)
	}

	// 将 big.Int 转换为 decimal
	balance := decimal.NewFromBigInt(balanceBig, -int32(decimals))
	return balance, nil
}

// GenerateReport 生成对账报告
func (s *ReconciliationService) GenerateReport(ctx context.Context, taskID string) (*ReconciliationReport, error) {
	task, err := s.GetTaskStatus(taskID)
	if err != nil {
		return nil, err
	}

	if task.Status != "completed" {
		return nil, fmt.Errorf("task not completed: %s", task.Status)
	}

	// 查询所有差异记录
	pagination := &repository.Pagination{Page: 1, PageSize: 1000}
	records, err := s.reconciliationRepo.ListDiscrepancies(ctx, pagination)
	if err != nil {
		return nil, fmt.Errorf("list discrepancies: %w", err)
	}

	// 构建报告
	report := &ReconciliationReport{
		TaskID:          taskID,
		GeneratedAt:     time.Now().UnixMilli(),
		TotalChecked:    task.TotalChecked,
		TotalDiscrepant: task.Discrepancies,
		TotalAmount:     decimal.Zero,
		Discrepancies:   make([]*ReconciliationDiscrepancy, 0),
		Summary: &ReconciliationSummary{
			ByToken:  make(map[string]*TokenSummary),
			ByStatus: make(map[string]int64),
		},
	}

	for _, record := range records {
		if record.Status == model.ReconciliationStatusDiscrepancy {
			discrepancy := &ReconciliationDiscrepancy{
				WalletAddress:   record.WalletAddress,
				Token:           record.Token,
				OnChainBalance:  record.OnChainBalance,
				OffChainBalance: record.OffChainSettled.Add(record.PendingSettle),
				Difference:      record.Difference,
				CheckedAt:       record.CheckedAt,
			}
			report.Discrepancies = append(report.Discrepancies, discrepancy)
			report.TotalAmount = report.TotalAmount.Add(record.Difference.Abs())

			// 更新 Token 摘要
			if _, ok := report.Summary.ByToken[record.Token]; !ok {
				report.Summary.ByToken[record.Token] = &TokenSummary{
					TotalDifference: decimal.Zero,
					OnChainTotal:    decimal.Zero,
					OffChainTotal:   decimal.Zero,
				}
			}
			summary := report.Summary.ByToken[record.Token]
			summary.TotalDiscrepant++
			summary.TotalDifference = summary.TotalDifference.Add(record.Difference.Abs())
			summary.OnChainTotal = summary.OnChainTotal.Add(record.OnChainBalance)
			summary.OffChainTotal = summary.OffChainTotal.Add(record.OffChainSettled.Add(record.PendingSettle))
		}

		// 更新状态摘要
		report.Summary.ByStatus[string(record.Status)]++
	}

	return report, nil
}

// GenerateReportJSON 生成 JSON 格式的对账报告
func (s *ReconciliationService) GenerateReportJSON(ctx context.Context, taskID string) (string, error) {
	report, err := s.GenerateReport(ctx, taskID)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal report: %w", err)
	}

	return string(data), nil
}

// QueryBalance 查询单个钱包的链上余额
func (s *ReconciliationService) QueryBalance(ctx context.Context, wallet, token string) (*OnChainBalanceInfo, error) {
	balance, err := s.getOnChainBalance(ctx, wallet, token)
	if err != nil {
		return nil, err
	}

	info := &OnChainBalanceInfo{
		WalletAddress: wallet,
		Token:         token,
		Balance:       balance,
		QueriedAt:     time.Now().UnixMilli(),
	}

	// 获取 token 信息
	if s.tokenRegistry != nil {
		tokenInfo, err := s.tokenRegistry.GetBySymbol(token)
		if err == nil {
			info.TokenAddress = tokenInfo.Address.Hex()
			info.Decimals = tokenInfo.Decimals
		}
	}

	return info, nil
}

// OnChainBalanceInfo 链上余额信息
type OnChainBalanceInfo struct {
	WalletAddress string          `json:"wallet_address"`
	Token         string          `json:"token"`
	TokenAddress  string          `json:"token_address,omitempty"`
	Balance       decimal.Decimal `json:"balance"`
	Decimals      uint8           `json:"decimals"`
	QueriedAt     int64           `json:"queried_at"`
}

// QueryVaultBalance 查询 Vault 合约中的余额
func (s *ReconciliationService) QueryVaultBalance(ctx context.Context, user, token common.Address) (*big.Int, error) {
	if s.vaultContract == nil {
		return nil, fmt.Errorf("vault contract not configured")
	}
	return s.vaultContract.GetBalance(ctx, user, token)
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

// ResolveDiscrepancy 解决差异
func (s *ReconciliationService) ResolveDiscrepancy(ctx context.Context, recordID int64, resolution, resolvedBy string) error {
	return s.reconciliationRepo.UpdateResolution(ctx, recordID, model.ReconciliationStatusResolved, resolution, resolvedBy)
}

// IgnoreDiscrepancy 忽略差异
func (s *ReconciliationService) IgnoreDiscrepancy(ctx context.Context, recordID int64, reason, ignoredBy string) error {
	return s.reconciliationRepo.UpdateResolution(ctx, recordID, model.ReconciliationStatusIgnored, reason, ignoredBy)
}
