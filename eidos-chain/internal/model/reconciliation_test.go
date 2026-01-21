package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestReconciliationStatus_Constants(t *testing.T) {
	// ReconciliationStatus 是字符串类型
	assert.Equal(t, ReconciliationStatus("OK"), ReconciliationStatusOK)
	assert.Equal(t, ReconciliationStatus("DISCREPANCY"), ReconciliationStatusDiscrepancy)
	assert.Equal(t, ReconciliationStatus("RESOLVED"), ReconciliationStatusResolved)
	assert.Equal(t, ReconciliationStatus("IGNORED"), ReconciliationStatusIgnored)
}

func TestReconciliationRecord_TableName(t *testing.T) {
	record := ReconciliationRecord{}
	assert.Equal(t, "chain_reconciliation_records", record.TableName())
}

func TestReconciliationRecord_HasDiscrepancy(t *testing.T) {
	tests := []struct {
		name       string
		difference decimal.Decimal
		expected   bool
	}{
		{"zero difference", decimal.Zero, false},
		{"positive difference", decimal.NewFromFloat(100.0), true},
		{"negative difference", decimal.NewFromFloat(-100.0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := &ReconciliationRecord{
				Difference: tt.difference,
			}
			assert.Equal(t, tt.expected, record.HasDiscrepancy())
		})
	}
}
