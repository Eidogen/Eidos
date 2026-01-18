package dto

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Pagination 分页信息
type Pagination struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// PagedData 分页数据
type PagedData struct {
	Items      interface{} `json:"items"`
	Pagination *Pagination `json:"pagination"`
}

// NewSuccessResponse 创建成功响应
func NewSuccessResponse(data interface{}) *Response {
	return &Response{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// NewErrorResponse 从 BizError 创建错误响应
func NewErrorResponse(err *BizError) *Response {
	return &Response{
		Code:    err.Code,
		Message: err.Message,
		Data:    nil,
	}
}

// NewPagedResponse 创建分页响应
func NewPagedResponse(items interface{}, total int64, page, pageSize int) *Response {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &Response{
		Code:    0,
		Message: "success",
		Data: &PagedData{
			Items: items,
			Pagination: &Pagination{
				Total:      total,
				Page:       page,
				PageSize:   pageSize,
				TotalPages: totalPages,
			},
		},
	}
}

// RateLimitInfo 限流信息（用于 429 响应）
type RateLimitInfo struct {
	Limit      int   `json:"limit"`
	Window     string `json:"window"`
	RetryAfter int   `json:"retry_after"`
}

// NewRateLimitResponse 创建限流响应
func NewRateLimitResponse(limit int, window string, retryAfter int) *Response {
	return &Response{
		Code:    ErrRateLimitExceeded.Code,
		Message: ErrRateLimitExceeded.Message,
		Data: &RateLimitInfo{
			Limit:      limit,
			Window:     window,
			RetryAfter: retryAfter,
		},
	}
}

