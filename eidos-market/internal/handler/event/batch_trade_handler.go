// Package event provides Kafka event handlers.
package event

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// BatchTradeProcessor batch trade processing interface
type BatchTradeProcessor interface {
	ProcessTrade(ctx context.Context, trade *model.TradeEvent) error
}

// BatchTradeHandler handles batches of trade events
type BatchTradeHandler struct {
	processor   BatchTradeProcessor
	logger      *zap.Logger
	workerCount int
}

// NewBatchTradeHandler creates a new batch trade handler
func NewBatchTradeHandler(processor BatchTradeProcessor, logger *zap.Logger) *BatchTradeHandler {
	return &BatchTradeHandler{
		processor:   processor,
		logger:      logger.Named("batch_trade_handler"),
		workerCount: 8, // Default worker count
	}
}

// NewBatchTradeHandlerWithWorkers creates a new batch trade handler with custom worker count
func NewBatchTradeHandlerWithWorkers(processor BatchTradeProcessor, logger *zap.Logger, workers int) *BatchTradeHandler {
	if workers <= 0 {
		workers = 8
	}
	return &BatchTradeHandler{
		processor:   processor,
		logger:      logger.Named("batch_trade_handler"),
		workerCount: workers,
	}
}

// HandleBatch processes a batch of trade messages
func (h *BatchTradeHandler) HandleBatch(ctx context.Context, messages []*sarama.ConsumerMessage) error {
	if len(messages) == 0 {
		return nil
	}

	// Group messages by market for better locality
	marketGroups := make(map[string][]*model.TradeEvent)

	for _, msg := range messages {
		var trade model.TradeEvent
		if err := json.Unmarshal(msg.Value, &trade); err != nil {
			h.logger.Warn("failed to unmarshal trade event",
				zap.Error(err),
				zap.ByteString("value", msg.Value))
			metrics.TradeProcessingErrors.WithLabelValues("unknown", "unmarshal_error").Inc()
			continue
		}

		marketGroups[trade.Market] = append(marketGroups[trade.Market], &trade)
	}

	// Process each market group in parallel
	var wg sync.WaitGroup
	errCh := make(chan error, len(marketGroups))
	semaphore := make(chan struct{}, h.workerCount)

	for market, trades := range marketGroups {
		wg.Add(1)
		go func(market string, trades []*model.TradeEvent) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Process trades sequentially within the same market to maintain order
			for _, trade := range trades {
				if err := h.processor.ProcessTrade(ctx, trade); err != nil {
					h.logger.Error("failed to process trade",
						zap.String("trade_id", trade.TradeID),
						zap.String("market", market),
						zap.Error(err))
					errCh <- err
				}
			}
		}(market, trades)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		h.logger.Warn("batch processing completed with errors",
			zap.Int("total", len(messages)),
			zap.Int("errors", len(errs)))
		// Return first error but all messages are processed
		return errs[0]
	}

	h.logger.Debug("batch processed successfully",
		zap.Int("count", len(messages)),
		zap.Int("markets", len(marketGroups)))

	return nil
}

// Topic returns the topic this handler processes
func (h *BatchTradeHandler) Topic() string {
	return "trade-results"
}
