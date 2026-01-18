package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDepositRecordStatus_String(t *testing.T) {
	tests := []struct {
		status   DepositRecordStatus
		expected string
	}{
		{DepositRecordStatusPending, "PENDING"},
		{DepositRecordStatusConfirmed, "CONFIRMED"},
		{DepositRecordStatusCredited, "CREDITED"},
		{DepositRecordStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestDepositRecord_TableName(t *testing.T) {
	record := DepositRecord{}
	assert.Equal(t, "eidos_chain_deposit_records", record.TableName())
}
