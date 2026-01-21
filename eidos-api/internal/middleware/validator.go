// Package middleware provides HTTP middleware
package middleware

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// Validation patterns
var (
	// walletPattern matches Ethereum wallet addresses (0x + 40 hex chars)
	walletPattern = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)

	// marketPattern matches market symbols (BASE-QUOTE format)
	marketPattern = regexp.MustCompile(`^[A-Z0-9]{2,10}-[A-Z0-9]{2,10}$`)

	// orderIDPattern matches order IDs (UUID or custom format)
	orderIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{8,64}$`)

	// tokenPattern matches token symbols
	tokenPattern = regexp.MustCompile(`^[A-Z0-9]{2,10}$`)

	// pricePattern matches decimal prices
	pricePattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

	// amountPattern matches decimal amounts
	amountPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

	// intervalPattern matches K-line intervals
	validIntervals = map[string]bool{
		"1m": true, "3m": true, "5m": true, "15m": true, "30m": true,
		"1h": true, "2h": true, "4h": true, "6h": true, "12h": true,
		"1d": true, "3d": true, "1w": true, "1M": true,
	}
)

// ValidatorConfig validation middleware configuration
type ValidatorConfig struct {
	MaxPageSize  int
	MaxLimit     int
	MaxTimestamp int64
}

// DefaultValidatorConfig default validation configuration
func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		MaxPageSize:  100,
		MaxLimit:     1000,
		MaxTimestamp: 253402300799000, // 9999-12-31 in milliseconds
	}
}

// ValidateMarketParam validates and normalizes market parameter
func ValidateMarketParam() gin.HandlerFunc {
	return func(c *gin.Context) {
		market := c.Param("market")
		if market == "" {
			market = c.Query("market")
		}

		if market == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidParams.WithMessage("market is required"),
			))
			return
		}

		// Normalize to uppercase
		market = strings.ToUpper(market)

		if !marketPattern.MatchString(market) {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidMarket.WithMessage("invalid market format, expected BASE-QUOTE"),
			))
			return
		}

		// Set normalized market
		c.Set("market", market)
		c.Next()
	}
}

// ValidateOrderID validates order ID parameter
func ValidateOrderID() gin.HandlerFunc {
	return func(c *gin.Context) {
		orderID := c.Param("id")
		if orderID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidParams.WithMessage("order id is required"),
			))
			return
		}

		if !orderIDPattern.MatchString(orderID) {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidParams.WithMessage("invalid order id format"),
			))
			return
		}

		c.Next()
	}
}

// ValidatePagination validates pagination parameters
func ValidatePagination(cfg *ValidatorConfig) gin.HandlerFunc {
	if cfg == nil {
		cfg = DefaultValidatorConfig()
	}

	return func(c *gin.Context) {
		// Page
		if pageStr := c.Query("page"); pageStr != "" {
			page, err := strconv.Atoi(pageStr)
			if err != nil || page < 1 {
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
					dto.ErrInvalidParams.WithMessage("page must be a positive integer"),
				))
				return
			}
		}

		// Page size
		if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
			pageSize, err := strconv.Atoi(pageSizeStr)
			if err != nil || pageSize < 1 || pageSize > cfg.MaxPageSize {
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
					dto.ErrInvalidParams.WithMessage("page_size must be between 1 and " + strconv.Itoa(cfg.MaxPageSize)),
				))
				return
			}
		}

		// Limit
		if limitStr := c.Query("limit"); limitStr != "" {
			limit, err := strconv.Atoi(limitStr)
			if err != nil || limit < 1 || limit > cfg.MaxLimit {
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
					dto.ErrInvalidParams.WithMessage("limit must be between 1 and " + strconv.Itoa(cfg.MaxLimit)),
				))
				return
			}
		}

		c.Next()
	}
}

// ValidateTimeRange validates time range parameters
func ValidateTimeRange(cfg *ValidatorConfig) gin.HandlerFunc {
	if cfg == nil {
		cfg = DefaultValidatorConfig()
	}

	return func(c *gin.Context) {
		var startTime, endTime int64
		var err error

		if startTimeStr := c.Query("start_time"); startTimeStr != "" {
			startTime, err = strconv.ParseInt(startTimeStr, 10, 64)
			if err != nil || startTime < 0 || startTime > cfg.MaxTimestamp {
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
					dto.ErrInvalidTimestamp.WithMessage("invalid start_time"),
				))
				return
			}
		}

		if endTimeStr := c.Query("end_time"); endTimeStr != "" {
			endTime, err = strconv.ParseInt(endTimeStr, 10, 64)
			if err != nil || endTime < 0 || endTime > cfg.MaxTimestamp {
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
					dto.ErrInvalidTimestamp.WithMessage("invalid end_time"),
				))
				return
			}
		}

		// Validate start < end
		if startTime > 0 && endTime > 0 && startTime >= endTime {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidParams.WithMessage("start_time must be less than end_time"),
			))
			return
		}

		c.Next()
	}
}

// ValidateInterval validates K-line interval parameter
func ValidateInterval() gin.HandlerFunc {
	return func(c *gin.Context) {
		interval := c.Query("interval")
		if interval == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidParams.WithMessage("interval is required"),
			))
			return
		}

		if !validIntervals[interval] {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidParams.WithMessage("invalid interval, valid values: 1m,3m,5m,15m,30m,1h,2h,4h,6h,12h,1d,3d,1w,1M"),
			))
			return
		}

		c.Next()
	}
}

// ValidateToken validates token parameter
func ValidateToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		if token == "" {
			token = c.Query("token")
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrInvalidParams.WithMessage("token is required"),
			))
			return
		}

		// Normalize to uppercase
		token = strings.ToUpper(token)

		if !tokenPattern.MatchString(token) {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
				dto.ErrTokenNotSupported.WithMessage("invalid token format"),
			))
			return
		}

		c.Set("token", token)
		c.Next()
	}
}

// ValidateWalletAddress validates wallet address format
func ValidateWalletAddress(address string) bool {
	return walletPattern.MatchString(address)
}

// ValidatePrice validates price format
func ValidatePrice(price string) bool {
	if price == "" {
		return true // Optional for market orders
	}
	return pricePattern.MatchString(price)
}

// ValidateAmount validates amount format
func ValidateAmount(amount string) bool {
	if amount == "" {
		return false
	}
	return amountPattern.MatchString(amount)
}

// ValidateOrderSide validates order side
func ValidateOrderSide(side string) bool {
	return side == "buy" || side == "sell"
}

// ValidateOrderType validates order type
func ValidateOrderType(orderType string) bool {
	return orderType == "limit" || orderType == "market"
}

// ValidateTimeInForce validates time in force
func ValidateTimeInForce(tif string) bool {
	switch tif {
	case "", "GTC", "IOC", "FOK", "GTT":
		return true
	default:
		return false
	}
}

// SanitizeInput sanitizes user input to prevent injection
func SanitizeInput(input string) string {
	// Remove control characters
	var result strings.Builder
	for _, r := range input {
		if r >= 32 && r != 127 {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ValidateDepthLimit validates depth limit parameter
func ValidateDepthLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if limitStr := c.Query("limit"); limitStr != "" {
			limit, err := strconv.Atoi(limitStr)
			if err != nil || limit < 1 || limit > 100 {
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
					dto.ErrInvalidParams.WithMessage("depth limit must be between 1 and 100"),
				))
				return
			}
		}
		c.Next()
	}
}

// ValidateRecentTradesLimit validates recent trades limit parameter
func ValidateRecentTradesLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if limitStr := c.Query("limit"); limitStr != "" {
			limit, err := strconv.Atoi(limitStr)
			if err != nil || limit < 1 || limit > 500 {
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewErrorResponse(
					dto.ErrInvalidParams.WithMessage("limit must be between 1 and 500"),
				))
				return
			}
		}
		c.Next()
	}
}
