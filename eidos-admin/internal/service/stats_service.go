package service

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// StatsService 统计服务
type StatsService struct {
	statsRepo  *repository.StatsRepository
	adminRepo  *repository.AdminRepository
	marketRepo *repository.MarketConfigRepository
}

// NewStatsService 创建统计服务
func NewStatsService(
	statsRepo *repository.StatsRepository,
	adminRepo *repository.AdminRepository,
	marketRepo *repository.MarketConfigRepository,
) *StatsService {
	return &StatsService{
		statsRepo:  statsRepo,
		adminRepo:  adminRepo,
		marketRepo: marketRepo,
	}
}

// GetOverview 获取总览统计
func (s *StatsService) GetOverview(ctx context.Context) (*model.OverviewStats, error) {
	today := time.Now().Format("2006-01-02")

	// 获取今日统计
	todayStats, err := s.statsRepo.GetByDateAndMarket(ctx, today, nil)
	if err != nil {
		return nil, err
	}

	// 获取历史汇总 (最近30天)
	startDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	totalStats, err := s.statsRepo.SumByDateRange(ctx, startDate, today)
	if err != nil {
		return nil, err
	}

	overview := &model.OverviewStats{
		SystemHealthy: true,
		LastUpdatedAt: time.Now().UnixMilli(),
	}

	if todayStats != nil {
		overview.TodayOrders = todayStats.OrderCount
		overview.TodayTrades = todayStats.TradeCount
		overview.TodayVolume = todayStats.TradeVolume
		overview.TodayActiveUsers = todayStats.ActiveUsers
		overview.TodayFees = addStrings(todayStats.MakerFeeTotal, todayStats.TakerFeeTotal)
	}

	if totalStats != nil {
		overview.TotalOrders = totalStats.OrderCount
		overview.TotalTrades = totalStats.TradeCount
		overview.TotalVolume = totalStats.TradeVolume
		overview.TotalFees = addStrings(totalStats.MakerFeeTotal, totalStats.TakerFeeTotal)
	}

	return overview, nil
}

// VolumeStatsRequest 交易量统计请求
type VolumeStatsRequest struct {
	StartDate string  `form:"start_date" binding:"required"`
	EndDate   string  `form:"end_date" binding:"required"`
	Market    *string `form:"market"`
}

// GetVolumeStats 获取交易量统计
func (s *StatsService) GetVolumeStats(ctx context.Context, req *VolumeStatsRequest) ([]*model.DailyStats, error) {
	return s.statsRepo.GetByDateRange(ctx, req.StartDate, req.EndDate, req.Market)
}

// FeeStatsResponse 手续费统计响应
type FeeStatsResponse struct {
	Date          string `json:"date"`
	Market        string `json:"market,omitempty"`
	MakerFeeTotal string `json:"maker_fee_total"`
	TakerFeeTotal string `json:"taker_fee_total"`
	TotalFee      string `json:"total_fee"`
}

// GetFeeStats 获取手续费统计
func (s *StatsService) GetFeeStats(ctx context.Context, startDate, endDate string) ([]*FeeStatsResponse, error) {
	stats, err := s.statsRepo.GetByDateRange(ctx, startDate, endDate, nil)
	if err != nil {
		return nil, err
	}

	result := make([]*FeeStatsResponse, 0, len(stats))
	for _, stat := range stats {
		market := ""
		if stat.Market != nil {
			market = *stat.Market
		}
		result = append(result, &FeeStatsResponse{
			Date:          stat.StatDate,
			Market:        market,
			MakerFeeTotal: stat.MakerFeeTotal,
			TakerFeeTotal: stat.TakerFeeTotal,
			TotalFee:      addStrings(stat.MakerFeeTotal, stat.TakerFeeTotal),
		})
	}

	return result, nil
}

// UserStatsResponse 用户统计响应
type UserStatsResponse struct {
	Date        string `json:"date"`
	ActiveUsers int64  `json:"active_users"`
	NewUsers    int64  `json:"new_users"`
}

// GetUserStats 获取用户统计
func (s *StatsService) GetUserStats(ctx context.Context, startDate, endDate string) ([]*UserStatsResponse, error) {
	stats, err := s.statsRepo.GetByDateRange(ctx, startDate, endDate, nil)
	if err != nil {
		return nil, err
	}

	result := make([]*UserStatsResponse, 0, len(stats))
	for _, stat := range stats {
		// 只返回全局统计
		if stat.Market != nil {
			continue
		}
		result = append(result, &UserStatsResponse{
			Date:        stat.StatDate,
			ActiveUsers: stat.ActiveUsers,
			NewUsers:    stat.NewUsers,
		})
	}

	return result, nil
}

// GetMarketStats 获取市场统计
func (s *StatsService) GetMarketStats(ctx context.Context, date string) ([]*model.DailyStats, error) {
	return s.statsRepo.GetMarketStats(ctx, date)
}

// GetLatestStats 获取最新统计
func (s *StatsService) GetLatestStats(ctx context.Context, limit int) ([]*model.DailyStats, error) {
	return s.statsRepo.GetLatest(ctx, limit)
}

// GetDailyStats 获取指定日期的统计 (全局汇总)
func (s *StatsService) GetDailyStats(ctx context.Context, date string) (*model.DailyStats, error) {
	return s.statsRepo.GetByDateAndMarket(ctx, date, nil)
}

// GetDailyStatsList 获取每日统计列表 (分页)
func (s *StatsService) GetDailyStatsList(ctx context.Context, startDate, endDate string, page *model.Pagination) ([]*model.DailyStats, error) {
	// 获取日期范围内的全局统计
	stats, err := s.statsRepo.GetByDateRange(ctx, startDate, endDate, nil)
	if err != nil {
		return nil, err
	}

	// 过滤掉市场级别的统计，只保留全局统计
	globalStats := make([]*model.DailyStats, 0)
	for _, stat := range stats {
		if stat.Market == nil {
			globalStats = append(globalStats, stat)
		}
	}

	// 简单分页处理
	page.Total = int64(len(globalStats))
	offset := page.GetOffset()
	limit := page.GetLimit()

	if offset >= len(globalStats) {
		return []*model.DailyStats{}, nil
	}

	end := offset + limit
	if end > len(globalStats) {
		end = len(globalStats)
	}

	return globalStats[offset:end], nil
}

// TradingStatsResult 交易统计结果
type TradingStatsResult struct {
	Symbol      string `json:"symbol,omitempty"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	TotalOrders int64  `json:"total_orders"`
	TotalTrades int64  `json:"total_trades"`
	TotalVolume string `json:"total_volume"`
	TotalFees   string `json:"total_fees"`
}

// GetTradingStats 获取交易统计
func (s *StatsService) GetTradingStats(ctx context.Context, symbol, startDate, endDate string) (*TradingStatsResult, error) {
	var market *string
	if symbol != "" {
		market = &symbol
	}

	stats, err := s.statsRepo.GetByDateRange(ctx, startDate, endDate, market)
	if err != nil {
		return nil, err
	}

	result := &TradingStatsResult{
		Symbol:    symbol,
		StartDate: startDate,
		EndDate:   endDate,
	}

	// 汇总统计
	for _, stat := range stats {
		result.TotalOrders += stat.OrderCount
		result.TotalTrades += stat.TradeCount
	}

	// 使用 SumByDateRange 获取汇总数据
	sumStats, err := s.statsRepo.SumByDateRange(ctx, startDate, endDate)
	if err == nil && sumStats != nil {
		result.TotalVolume = sumStats.TradeVolume
		result.TotalFees = addStrings(sumStats.MakerFeeTotal, sumStats.TakerFeeTotal)
	}

	return result, nil
}

// SettlementStatsResult 结算统计结果
type SettlementStatsResult struct {
	StartDate          string `json:"start_date"`
	EndDate            string `json:"end_date"`
	TotalSettlements   int64  `json:"total_settlements"`
	SuccessSettlements int64  `json:"success_settlements"`
	FailedSettlements  int64  `json:"failed_settlements"`
	PendingSettlements int64  `json:"pending_settlements"`
	TotalVolume        string `json:"total_volume"`
	TotalGasFees       string `json:"total_gas_fees"`
}

// GetSettlementStats 获取结算统计
func (s *StatsService) GetSettlementStats(ctx context.Context, startDate, endDate string) (*SettlementStatsResult, error) {
	// 结算统计目前返回模拟数据，实际应从结算服务或数据库获取
	result := &SettlementStatsResult{
		StartDate:          startDate,
		EndDate:            endDate,
		TotalSettlements:   0,
		SuccessSettlements: 0,
		FailedSettlements:  0,
		PendingSettlements: 0,
		TotalVolume:        "0",
		TotalGasFees:       "0",
	}

	return result, nil
}

// addStrings 简单字符串数值相加 (实际应用中应使用 decimal 库)
func addStrings(a, b string) string {
	// 简化实现，实际应使用 shopspring/decimal
	if a == "" || a == "0" {
		return b
	}
	if b == "" || b == "0" {
		return a
	}
	return a + "+" + b // 占位符，实际需要数值运算
}
