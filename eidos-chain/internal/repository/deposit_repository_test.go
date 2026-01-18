package repository

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// TestDepositRepository_Errors 测试错误类型
func TestDepositRepository_Errors(t *testing.T) {
	assert.Equal(t, "deposit record not found", ErrDepositRecordNotFound.Error())
	assert.Equal(t, "duplicate deposit", ErrDuplicateDeposit.Error())
}

// TestDepositRecordStatus_Values 测试充值状态枚举值
func TestDepositRecordStatus_Values(t *testing.T) {
	assert.Equal(t, model.DepositRecordStatus(0), model.DepositRecordStatusPending)
	assert.Equal(t, model.DepositRecordStatus(1), model.DepositRecordStatusConfirmed)
	assert.Equal(t, model.DepositRecordStatus(2), model.DepositRecordStatusCredited)
}

// TestDepositRecordStatus_String 测试状态字符串表示
func TestDepositRecordStatus_String(t *testing.T) {
	assert.Equal(t, "PENDING", model.DepositRecordStatusPending.String())
	assert.Equal(t, "CONFIRMED", model.DepositRecordStatusConfirmed.String())
	assert.Equal(t, "CREDITED", model.DepositRecordStatusCredited.String())
	assert.Equal(t, "UNKNOWN", model.DepositRecordStatus(99).String())
}

// TestDepositRecord_TableName 测试表名
func TestDepositRecord_TableName(t *testing.T) {
	record := &model.DepositRecord{}
	assert.Equal(t, "eidos_chain_deposit_records", record.TableName())
}

// TestDepositRecord_Fields 测试 DepositRecord 字段
func TestDepositRecord_Fields(t *testing.T) {
	record := &model.DepositRecord{
		ID:                    1,
		DepositID:             "dep-123",
		WalletAddress:         "0x1234567890123456789012345678901234567890",
		Token:                 "USDC",
		TokenAddress:          "0xtoken",
		Amount:                decimal.NewFromFloat(1000),
		ChainID:               31337,
		TxHash:                "0xabc123",
		BlockNumber:           12345,
		BlockHash:             "0xblockhash",
		LogIndex:              0,
		Confirmations:         6,
		RequiredConfirmations: 12,
		Status:                model.DepositRecordStatusPending,
		CreditedAt:            0,
		CreatedAt:             1234567890000,
		UpdatedAt:             1234567890000,
	}

	assert.Equal(t, int64(1), record.ID)
	assert.Equal(t, "dep-123", record.DepositID)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", record.WalletAddress)
	assert.Equal(t, "USDC", record.Token)
	assert.Equal(t, "0xtoken", record.TokenAddress)
	assert.True(t, record.Amount.Equal(decimal.NewFromFloat(1000)))
	assert.Equal(t, int64(31337), record.ChainID)
	assert.Equal(t, "0xabc123", record.TxHash)
	assert.Equal(t, int64(12345), record.BlockNumber)
	assert.Equal(t, "0xblockhash", record.BlockHash)
	assert.Equal(t, 0, record.LogIndex)
	assert.Equal(t, 6, record.Confirmations)
	assert.Equal(t, 12, record.RequiredConfirmations)
	assert.Equal(t, model.DepositRecordStatusPending, record.Status)
}

// TestIsDuplicateKeyError 测试重复键错误检测
func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"nil error string", "", false},
		{"duplicate key error", "duplicate key value violates unique constraint", true},
		{"postgres error code 23505", "ERROR 23505: unique_violation", true},
		{"other error", "connection refused", false},
		{"mixed case duplicate", "Duplicate Key error", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errStr == "" {
				assert.False(t, isDuplicateKeyError(nil))
			} else {
				// isDuplicateKeyError 检查 error 的字符串表示
				err := &testError{msg: tt.errStr}
				result := isDuplicateKeyError(err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// testError 用于测试的自定义错误类型
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestContainsHelper 测试 contains 辅助函数
func TestContainsHelper(t *testing.T) {
	assert.True(t, contains("duplicate key", "duplicate key"))
	assert.True(t, contains("error: duplicate key occurred", "duplicate key"))
	assert.True(t, contains("ERROR 23505: unique_violation", "23505"))
	assert.False(t, contains("some error", "duplicate key"))
	assert.False(t, contains("", "test"))
	assert.True(t, contains("test", ""))
}

// TestPagination 测试分页结构
func TestPagination(t *testing.T) {
	t.Run("default values - applies defaults", func(t *testing.T) {
		page := &Pagination{}
		// Offset() sets Page=1 if <= 0, so (1-1)*0 = 0
		assert.Equal(t, 0, page.Offset())
		// Limit() sets PageSize=20 if <= 0
		assert.Equal(t, 20, page.Limit())
	})

	t.Run("with values", func(t *testing.T) {
		page := &Pagination{
			Page:     2,
			PageSize: 10,
		}
		assert.Equal(t, 10, page.Offset()) // (2-1) * 10
		assert.Equal(t, 10, page.Limit())
	})

	t.Run("first page", func(t *testing.T) {
		page := &Pagination{
			Page:     1,
			PageSize: 20,
		}
		assert.Equal(t, 0, page.Offset()) // (1-1) * 20
		assert.Equal(t, 20, page.Limit())
	})

	t.Run("max limit enforced", func(t *testing.T) {
		page := &Pagination{
			Page:     1,
			PageSize: 200,
		}
		// Limit() caps PageSize at 100
		assert.Equal(t, 100, page.Limit())
	})
}
