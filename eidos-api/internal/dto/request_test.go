package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPaginationQuery_Normalize(t *testing.T) {
	tests := []struct {
		name         string
		page         int
		pageSize     int
		wantPage     int
		wantPageSize int
	}{
		{
			name:         "valid_values",
			page:         2,
			pageSize:     20,
			wantPage:     2,
			wantPageSize: 20,
		},
		{
			name:         "zero_page",
			page:         0,
			pageSize:     20,
			wantPage:     1,
			wantPageSize: 20,
		},
		{
			name:         "negative_page",
			page:         -1,
			pageSize:     20,
			wantPage:     1,
			wantPageSize: 20,
		},
		{
			name:         "zero_page_size",
			page:         1,
			pageSize:     0,
			wantPage:     1,
			wantPageSize: 20,
		},
		{
			name:         "negative_page_size",
			page:         1,
			pageSize:     -10,
			wantPage:     1,
			wantPageSize: 20,
		},
		{
			name:         "page_size_over_max",
			page:         1,
			pageSize:     200,
			wantPage:     1,
			wantPageSize: 100,
		},
		{
			name:         "page_size_at_max",
			page:         1,
			pageSize:     100,
			wantPage:     1,
			wantPageSize: 100,
		},
		{
			name:         "all_invalid",
			page:         0,
			pageSize:     0,
			wantPage:     1,
			wantPageSize: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PaginationQuery{
				Page:     tt.page,
				PageSize: tt.pageSize,
			}
			p.Normalize()
			assert.Equal(t, tt.wantPage, p.Page)
			assert.Equal(t, tt.wantPageSize, p.PageSize)
		})
	}
}
