package model

import (
	"github.com/shopspring/decimal"
)

// KlineInterval K线周期
type KlineInterval string

const (
	Interval1m  KlineInterval = "1m"
	Interval5m  KlineInterval = "5m"
	Interval15m KlineInterval = "15m"
	Interval30m KlineInterval = "30m"
	Interval1h  KlineInterval = "1h"
	Interval4h  KlineInterval = "4h"
	Interval1d  KlineInterval = "1d"
	Interval1w  KlineInterval = "1w"
)

// IntervalDurations 周期对应毫秒数
var IntervalDurations = map[KlineInterval]int64{
	Interval1m:  60_000,
	Interval5m:  300_000,
	Interval15m: 900_000,
	Interval30m: 1_800_000,
	Interval1h:  3_600_000,
	Interval4h:  14_400_000,
	Interval1d:  86_400_000,
	Interval1w:  604_800_000,
}

// AllIntervals 所有支持的K线周期（按时间从短到长排序）
var AllIntervals = []KlineInterval{
	Interval1m,
	Interval5m,
	Interval15m,
	Interval30m,
	Interval1h,
	Interval4h,
	Interval1d,
	Interval1w,
}

// IsValidInterval 检查是否为有效的K线周期
func IsValidInterval(interval string) bool {
	_, ok := IntervalDurations[KlineInterval(interval)]
	return ok
}

// Kline K线数据
type Kline struct {
	Market      string          `json:"market" gorm:"type:varchar(20);not null;primaryKey"`
	Interval    KlineInterval   `json:"interval" gorm:"type:varchar(5);not null;primaryKey"`
	OpenTime    int64           `json:"open_time" gorm:"type:bigint;not null;primaryKey"`
	Open        decimal.Decimal `json:"open" gorm:"type:decimal(36,18);not null"`
	High        decimal.Decimal `json:"high" gorm:"type:decimal(36,18);not null"`
	Low         decimal.Decimal `json:"low" gorm:"type:decimal(36,18);not null"`
	Close       decimal.Decimal `json:"close" gorm:"type:decimal(36,18);not null"`
	Volume      decimal.Decimal `json:"volume" gorm:"type:decimal(36,18);not null"`
	QuoteVolume decimal.Decimal `json:"quote_volume" gorm:"type:decimal(36,18);not null"`
	TradeCount  int32           `json:"trade_count" gorm:"type:int;not null;default:0"`
	CloseTime   int64           `json:"close_time" gorm:"type:bigint;not null"`
}

// TableName GORM表名
func (Kline) TableName() string {
	return "market_klines"
}

// IsEmpty 检查K线是否为空（无成交）
func (k *Kline) IsEmpty() bool {
	return k.TradeCount == 0
}

// Clone 克隆K线数据
func (k *Kline) Clone() *Kline {
	return &Kline{
		Market:      k.Market,
		Interval:    k.Interval,
		OpenTime:    k.OpenTime,
		Open:        k.Open,
		High:        k.High,
		Low:         k.Low,
		Close:       k.Close,
		Volume:      k.Volume,
		QuoteVolume: k.QuoteVolume,
		TradeCount:  k.TradeCount,
		CloseTime:   k.CloseTime,
	}
}

// CalculateOpenTime 根据时间戳和周期计算K线开盘时间
func CalculateOpenTime(timestamp int64, interval KlineInterval) int64 {
	duration := IntervalDurations[interval]
	return (timestamp / duration) * duration
}

// CalculateCloseTime 根据开盘时间和周期计算K线收盘时间
func CalculateCloseTime(openTime int64, interval KlineInterval) int64 {
	duration := IntervalDurations[interval]
	return openTime + duration - 1
}
