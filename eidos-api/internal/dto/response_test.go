package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSuccessResponse(t *testing.T) {
	data := map[string]string{"key": "value"}
	resp := NewSuccessResponse(data)

	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "success", resp.Message)
	assert.Equal(t, data, resp.Data)
}

func TestNewSuccessResponse_NilData(t *testing.T) {
	resp := NewSuccessResponse(nil)

	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "success", resp.Message)
	assert.Nil(t, resp.Data)
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse(ErrInvalidParams)

	assert.Equal(t, ErrInvalidParams.Code, resp.Code)
	assert.Equal(t, ErrInvalidParams.Message, resp.Message)
	assert.Nil(t, resp.Data)
}

func TestNewPagedResponse(t *testing.T) {
	items := []string{"a", "b", "c"}
	resp := NewPagedResponse(items, 100, 1, 10)

	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "success", resp.Message)

	pagedData, ok := resp.Data.(*PagedData)
	assert.True(t, ok)
	assert.Equal(t, items, pagedData.Items)
	assert.Equal(t, int64(100), pagedData.Pagination.Total)
	assert.Equal(t, 1, pagedData.Pagination.Page)
	assert.Equal(t, 10, pagedData.Pagination.PageSize)
	assert.Equal(t, 10, pagedData.Pagination.TotalPages)
}

func TestNewPagedResponse_TotalPagesCalculation(t *testing.T) {
	tests := []struct {
		name       string
		total      int64
		pageSize   int
		wantPages  int
	}{
		{"exact_division", 100, 10, 10},
		{"with_remainder", 101, 10, 11},
		{"single_item", 1, 10, 1},
		{"zero_items", 0, 10, 0},
		{"page_size_larger", 5, 10, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewPagedResponse(nil, tt.total, 1, tt.pageSize)
			pagedData := resp.Data.(*PagedData)
			assert.Equal(t, tt.wantPages, pagedData.Pagination.TotalPages)
		})
	}
}

func TestNewRateLimitResponse(t *testing.T) {
	resp := NewRateLimitResponse(100, "1m", 30)

	assert.Equal(t, ErrRateLimitExceeded.Code, resp.Code)
	assert.Equal(t, ErrRateLimitExceeded.Message, resp.Message)

	info, ok := resp.Data.(*RateLimitInfo)
	assert.True(t, ok)
	assert.Equal(t, 100, info.Limit)
	assert.Equal(t, "1m", info.Window)
	assert.Equal(t, 30, info.RetryAfter)
}
