// Package model 订单模型单元测试
package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestOrderSide_String(t *testing.T) {
	tests := []struct {
		name string
		side OrderSide
		want string
	}{
		{"buy side", OrderSideBuy, "BUY"},
		{"sell side", OrderSideSell, "SELL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.side.String())
		})
	}
}

func TestOrderSide_Opposite(t *testing.T) {
	assert.Equal(t, OrderSideSell, OrderSideBuy.Opposite())
	assert.Equal(t, OrderSideBuy, OrderSideSell.Opposite())
}

func TestOrderType_String(t *testing.T) {
	assert.Equal(t, "LIMIT", OrderTypeLimit.String())
	assert.Equal(t, "MARKET", OrderTypeMarket.String())
}

func TestTimeInForce_String(t *testing.T) {
	tests := []struct {
		name string
		tif  TimeInForce
		want string
	}{
		{"GTC", TimeInForceGTC, "GTC"},
		{"IOC", TimeInForceIOC, "IOC"},
		{"FOK", TimeInForceFOK, "FOK"},
		{"unknown", TimeInForce(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tif.String())
		})
	}
}

func TestOrder_IsFilled(t *testing.T) {
	tests := []struct {
		name      string
		remaining decimal.Decimal
		want      bool
	}{
		{"zero remaining", decimal.Zero, true},
		{"negative remaining", decimal.NewFromFloat(-0.1), true},
		{"positive remaining", decimal.NewFromFloat(1.5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := &Order{Remaining: tt.remaining}
			assert.Equal(t, tt.want, order.IsFilled())
		})
	}
}

func TestOrder_FilledAmount(t *testing.T) {
	order := &Order{
		Amount:    decimal.NewFromFloat(10.0),
		Remaining: decimal.NewFromFloat(3.5),
	}

	expected := decimal.NewFromFloat(6.5)
	assert.True(t, expected.Equal(order.FilledAmount()))
}
