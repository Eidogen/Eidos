package repository

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// TestReconciliationRepository_Errors 测试错误类型
func TestReconciliationRepository_Errors(t *testing.T) {
	assert.Equal(t, "reconciliation record not found", ErrReconciliationNotFound.Error())
}

// TestReconciliationStatus_Values 测试对账状态值
func TestReconciliationStatus_Values(t *testing.T) {
	assert.Equal(t, model.ReconciliationStatus("OK"), model.ReconciliationStatusOK)
	assert.Equal(t, model.ReconciliationStatus("DISCREPANCY"), model.ReconciliationStatusDiscrepancy)
	assert.Equal(t, model.ReconciliationStatus("RESOLVED"), model.ReconciliationStatusResolved)
	assert.Equal(t, model.ReconciliationStatus("IGNORED"), model.ReconciliationStatusIgnored)
}

// TestReconciliationRecord_TableName 测试表名
func TestReconciliationRecord_TableName(t *testing.T) {
	record := &model.ReconciliationRecord{}
	assert.Equal(t, "chain_reconciliation_records", record.TableName())
}

// TestReconciliationRecord_Fields 测试 ReconciliationRecord 字段
func TestReconciliationRecord_Fields(t *testing.T) {
	record := &model.ReconciliationRecord{
		ID:                1,
		WalletAddress:     "0x1234567890123456789012345678901234567890",
		Token:             "USDC",
		OnChainBalance:    decimal.NewFromFloat(1000),
		OffChainSettled:   decimal.NewFromFloat(1000),
		OffChainAvailable: decimal.NewFromFloat(800),
		OffChainFrozen:    decimal.NewFromFloat(200),
		PendingSettle:     decimal.Zero,
		Difference:        decimal.Zero,
		Status:            model.ReconciliationStatusOK,
		Resolution:        "",
		ResolvedBy:        "",
		ResolvedAt:        0,
		CheckedAt:         1234567890000,
		CreatedAt:         1234567890000,
	}

	assert.Equal(t, int64(1), record.ID)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", record.WalletAddress)
	assert.Equal(t, "USDC", record.Token)
	assert.True(t, record.OnChainBalance.Equal(decimal.NewFromFloat(1000)))
	assert.True(t, record.OffChainSettled.Equal(decimal.NewFromFloat(1000)))
	assert.True(t, record.OffChainAvailable.Equal(decimal.NewFromFloat(800)))
	assert.True(t, record.OffChainFrozen.Equal(decimal.NewFromFloat(200)))
	assert.True(t, record.PendingSettle.IsZero())
	assert.True(t, record.Difference.IsZero())
	assert.Equal(t, model.ReconciliationStatusOK, record.Status)
}

// TestReconciliationRecord_Discrepancy 测试差异记录
func TestReconciliationRecord_Discrepancy(t *testing.T) {
	record := &model.ReconciliationRecord{
		ID:                2,
		WalletAddress:     "0x5678",
		Token:             "USDC",
		OnChainBalance:    decimal.NewFromFloat(1000),
		OffChainSettled:   decimal.NewFromFloat(900),
		OffChainAvailable: decimal.NewFromFloat(700),
		OffChainFrozen:    decimal.NewFromFloat(200),
		PendingSettle:     decimal.Zero,
		Difference:        decimal.NewFromFloat(100),
		Status:            model.ReconciliationStatusDiscrepancy,
	}

	assert.Equal(t, model.ReconciliationStatusDiscrepancy, record.Status)
	assert.True(t, record.Difference.Equal(decimal.NewFromFloat(100)))
}

// TestReconciliationRecord_Resolved 测试已解决记录
func TestReconciliationRecord_Resolved(t *testing.T) {
	record := &model.ReconciliationRecord{
		ID:         3,
		Status:     model.ReconciliationStatusResolved,
		Resolution: "Manual adjustment applied",
		ResolvedBy: "admin@example.com",
		ResolvedAt: 1234567890000,
	}

	assert.Equal(t, model.ReconciliationStatusResolved, record.Status)
	assert.Equal(t, "Manual adjustment applied", record.Resolution)
	assert.Equal(t, "admin@example.com", record.ResolvedBy)
	assert.NotZero(t, record.ResolvedAt)
}

// TestReconciliationRecord_Ignored 测试已忽略记录
func TestReconciliationRecord_Ignored(t *testing.T) {
	record := &model.ReconciliationRecord{
		ID:         4,
		Status:     model.ReconciliationStatusIgnored,
		Resolution: "Known timing difference",
		ResolvedBy: "system",
	}

	assert.Equal(t, model.ReconciliationStatusIgnored, record.Status)
}
