package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

func TestNewTradingClient(t *testing.T) {
	// NewTradingClient with nil connection
	client := NewTradingClient(nil)
	assert.NotNil(t, client)
	assert.Nil(t, client.conn)
}

func TestTradingClient_Close_NilConn(t *testing.T) {
	client := &TradingClient{conn: nil}
	err := client.Close()
	assert.NoError(t, err)
}

func TestConvertGRPCError(t *testing.T) {
	tests := []struct {
		name     string
		grpcErr  error
		expected *dto.BizError
	}{
		{
			name:     "not_found_order",
			grpcErr:  status.Error(codes.NotFound, "order not found"),
			expected: dto.ErrOrderNotFound,
		},
		{
			name:     "not_found_trade",
			grpcErr:  status.Error(codes.NotFound, "trade not found"),
			expected: dto.ErrTradeNotFound,
		},
		{
			name:     "not_found_deposit",
			grpcErr:  status.Error(codes.NotFound, "deposit not found"),
			expected: dto.ErrDepositNotFound,
		},
		{
			name:     "not_found_withdraw",
			grpcErr:  status.Error(codes.NotFound, "withdraw not found"),
			expected: dto.ErrWithdrawNotFound,
		},
		{
			name:     "not_found_default",
			grpcErr:  status.Error(codes.NotFound, "something not found"),
			expected: dto.ErrOrderNotFound,
		},
		{
			name:     "invalid_argument",
			grpcErr:  status.Error(codes.InvalidArgument, "invalid input"),
			expected: dto.ErrInvalidParams,
		},
		{
			name:     "failed_precondition_insufficient",
			grpcErr:  status.Error(codes.FailedPrecondition, "insufficient balance"),
			expected: dto.ErrInsufficientBalance,
		},
		{
			name:     "failed_precondition_cancelled",
			grpcErr:  status.Error(codes.FailedPrecondition, "order already cancelled"),
			expected: dto.ErrOrderAlreadyCancelled,
		},
		{
			name:     "failed_precondition_filled",
			grpcErr:  status.Error(codes.FailedPrecondition, "order already filled"),
			expected: dto.ErrOrderAlreadyFilled,
		},
		{
			name:     "failed_precondition_not_cancellable",
			grpcErr:  status.Error(codes.FailedPrecondition, "order not cancellable"),
			expected: dto.ErrOrderNotCancellable,
		},
		{
			name:     "failed_precondition_default",
			grpcErr:  status.Error(codes.FailedPrecondition, "some other condition"),
			expected: dto.ErrInvalidParams,
		},
		{
			name:     "already_exists",
			grpcErr:  status.Error(codes.AlreadyExists, "order already exists"),
			expected: dto.ErrDuplicateOrder,
		},
		{
			name:     "unavailable",
			grpcErr:  status.Error(codes.Unavailable, "service unavailable"),
			expected: dto.ErrServiceUnavailable,
		},
		{
			name:     "deadline_exceeded",
			grpcErr:  status.Error(codes.DeadlineExceeded, "timeout"),
			expected: dto.ErrTimeout,
		},
		{
			name:     "unknown_code",
			grpcErr:  status.Error(codes.Internal, "internal error"),
			expected: dto.ErrInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertGRPCError(tt.grpcErr)
			apiErr, ok := result.(*dto.BizError)
			if ok {
				assert.Equal(t, tt.expected.Code, apiErr.Code)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestConvertGRPCError_NonGRPCError(t *testing.T) {
	// Non-gRPC error should return internal error
	err := assert.AnError
	result := convertGRPCError(err)
	assert.Equal(t, dto.ErrInternalError, result)
}

func TestContains(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"hello world", "hello", true},
		{"hello world", "world", true},
		{"hello world", "lo wo", true},
		{"hello world", "foo", false},
		{"", "foo", false},
		{"hello", "", false},
		{"", "", false},
		{"abc", "abc", true},
		{"abc", "abcd", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsInner(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"hello world", "lo wo", true},
		{"hello world", "hello", true},
		{"hello world", "world", true},
		{"hello world", "foo", false},
		{"abc", "abc", true},
		{"abc", "d", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := containsInner(tt.s, tt.substr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewTradingClientWithTarget(t *testing.T) {
	// Test creating a client with an invalid target
	// This won't actually fail because NewClient is lazy
	client, err := NewTradingClientWithTarget("localhost:99999", false)
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Clean up
	client.Close()
}
