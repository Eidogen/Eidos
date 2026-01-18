package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestRecordHTTPRequest(t *testing.T) {
	// 记录一个 HTTP 请求
	RecordHTTPRequest("GET", "/api/v1/orders", "200", 0.05, 100, 500)

	// 验证 Counter
	count := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/orders", "200"))
	assert.Equal(t, float64(1), count)
}

func TestRecordGRPCRequest(t *testing.T) {
	// 记录一个 gRPC 请求
	RecordGRPCRequest("trading", "CreateOrder", "OK", 0.01)

	// 验证 Counter
	count := testutil.ToFloat64(GRPCRequestsTotal.WithLabelValues("trading", "CreateOrder", "OK"))
	assert.Equal(t, float64(1), count)
}

func TestRecordWSConnection(t *testing.T) {
	// 获取初始值
	initial := testutil.ToFloat64(WSConnectionsTotal)

	// 记录连接
	RecordWSConnection(true)
	assert.Equal(t, initial+1, testutil.ToFloat64(WSConnectionsTotal))

	// 记录断开
	RecordWSConnection(false)
	assert.Equal(t, initial, testutil.ToFloat64(WSConnectionsTotal))
}

func TestRecordWSSubscription(t *testing.T) {
	// 获取初始值
	initial := testutil.ToFloat64(WSSubscriptionsTotal.WithLabelValues("ticker"))

	// 记录订阅
	RecordWSSubscription("ticker", true)
	assert.Equal(t, initial+1, testutil.ToFloat64(WSSubscriptionsTotal.WithLabelValues("ticker")))

	// 记录取消订阅
	RecordWSSubscription("ticker", false)
	assert.Equal(t, initial, testutil.ToFloat64(WSSubscriptionsTotal.WithLabelValues("ticker")))
}

func TestRecordWSMessage(t *testing.T) {
	// 记录发送消息
	initialSent := testutil.ToFloat64(WSMessagesSent.WithLabelValues("ticker"))
	RecordWSMessage("ticker", true)
	assert.Equal(t, initialSent+1, testutil.ToFloat64(WSMessagesSent.WithLabelValues("ticker")))

	// 记录接收消息
	initialReceived := testutil.ToFloat64(WSMessagesReceived.WithLabelValues("subscribe"))
	RecordWSMessage("subscribe", false)
	assert.Equal(t, initialReceived+1, testutil.ToFloat64(WSMessagesReceived.WithLabelValues("subscribe")))
}

func TestRecordRedisMessage(t *testing.T) {
	// 记录成功消息
	initialReceived := testutil.ToFloat64(RedisMessagesReceived.WithLabelValues("ticker"))
	RecordRedisMessage("ticker", true, "")
	assert.Equal(t, initialReceived+1, testutil.ToFloat64(RedisMessagesReceived.WithLabelValues("ticker")))

	// 记录失败消息
	initialErrors := testutil.ToFloat64(RedisMessageErrors.WithLabelValues("depth", "unmarshal_error"))
	RecordRedisMessage("depth", false, "unmarshal_error")
	assert.Equal(t, initialErrors+1, testutil.ToFloat64(RedisMessageErrors.WithLabelValues("depth", "unmarshal_error")))
}

func TestRecordAuth(t *testing.T) {
	// 记录认证成功
	initialSuccess := testutil.ToFloat64(AuthRequestsTotal.WithLabelValues("success"))
	RecordAuth("success", 0.001)
	assert.Equal(t, initialSuccess+1, testutil.ToFloat64(AuthRequestsTotal.WithLabelValues("success")))

	// 记录认证失败
	initialFailed := testutil.ToFloat64(AuthRequestsTotal.WithLabelValues("failed"))
	RecordAuth("failed", 0.002)
	assert.Equal(t, initialFailed+1, testutil.ToFloat64(AuthRequestsTotal.WithLabelValues("failed")))
}

func TestRecordRateLimit(t *testing.T) {
	// 记录允许
	initialAllowed := testutil.ToFloat64(RateLimitRequestsTotal.WithLabelValues("ip", "allowed"))
	RecordRateLimit("ip", "allowed")
	assert.Equal(t, initialAllowed+1, testutil.ToFloat64(RateLimitRequestsTotal.WithLabelValues("ip", "allowed")))

	// 记录拒绝
	initialRejected := testutil.ToFloat64(RateLimitRequestsTotal.WithLabelValues("wallet", "rejected"))
	RecordRateLimit("wallet", "rejected")
	assert.Equal(t, initialRejected+1, testutil.ToFloat64(RateLimitRequestsTotal.WithLabelValues("wallet", "rejected")))
}
