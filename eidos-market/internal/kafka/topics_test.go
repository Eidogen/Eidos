package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopicConstants(t *testing.T) {
	// Verify topic names match the expected values
	assert.Equal(t, "trade-results", TopicTradeResults)
	assert.Equal(t, "orderbook-updates", TopicOrderBookUpdates)
	assert.Equal(t, "kline-1m", TopicKline1m)
}

func TestConsumerGroupConstants(t *testing.T) {
	assert.Equal(t, "eidos-market", GroupMarket)
}

func TestTopicNamingConvention(t *testing.T) {
	// All topics should use kebab-case
	topics := []string{TopicTradeResults, TopicOrderBookUpdates, TopicKline1m}
	for _, topic := range topics {
		assert.NotContains(t, topic, "_", "topic %s should use kebab-case, not snake_case", topic)
		assert.NotContains(t, topic, " ", "topic %s should not contain spaces", topic)
	}
}
