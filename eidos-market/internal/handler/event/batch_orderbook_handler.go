// Package event provides Kafka event handlers.
package event

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"

	"github.com/IBM/sarama"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// BatchOrderBookProcessor batch orderbook processing interface
type BatchOrderBookProcessor interface {
	ProcessOrderBookUpdate(ctx context.Context, update *model.DepthUpdate) error
}

// BatchOrderBookHandler handles batches of orderbook update events
type BatchOrderBookHandler struct {
	processor   BatchOrderBookProcessor
	logger      *slog.Logger
	workerCount int
}

// NewBatchOrderBookHandler creates a new batch orderbook handler
func NewBatchOrderBookHandler(processor BatchOrderBookProcessor, logger *slog.Logger) *BatchOrderBookHandler {
	return &BatchOrderBookHandler{
		processor:   processor,
		logger:      logger.With("component", "batch_orderbook_handler"),
		workerCount: 8,
	}
}

// HandleBatch processes a batch of orderbook update messages
func (h *BatchOrderBookHandler) HandleBatch(ctx context.Context, messages []*sarama.ConsumerMessage) error {
	if len(messages) == 0 {
		return nil
	}

	// Group messages by market
	marketGroups := make(map[string][]*orderbookUpdateWithSeq)

	for _, msg := range messages {
		var rawMsg OrderBookUpdateMessage
		if err := json.Unmarshal(msg.Value, &rawMsg); err != nil {
			h.logger.Warn("failed to unmarshal orderbook update",
				"error", err,
				"value", string(msg.Value))
			metrics.DepthUpdatesTotal.WithLabelValues("unknown").Inc()
			continue
		}

		update, err := h.toDepthUpdate(&rawMsg)
		if err != nil {
			h.logger.Warn("failed to convert orderbook update",
				"market", rawMsg.Market,
				"error", err)
			continue
		}

		marketGroups[rawMsg.Market] = append(marketGroups[rawMsg.Market], &orderbookUpdateWithSeq{
			update:   update,
			sequence: update.Sequence,
		})
	}

	// Process each market group
	var wg sync.WaitGroup
	errCh := make(chan error, len(marketGroups))
	semaphore := make(chan struct{}, h.workerCount)

	for market, updates := range marketGroups {
		wg.Add(1)
		go func(market string, updates []*orderbookUpdateWithSeq) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Sort by sequence to ensure correct ordering
			sort.Slice(updates, func(i, j int) bool {
				return updates[i].sequence < updates[j].sequence
			})

			// Process updates sequentially to maintain order
			for _, u := range updates {
				if err := h.processor.ProcessOrderBookUpdate(ctx, u.update); err != nil {
					h.logger.Error("failed to process orderbook update",
						"market", market,
						"sequence", u.sequence,
						"error", err)
					errCh <- err
				}
			}
		}(market, updates)
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
			"total", len(messages),
			"errors", len(errs))
		return errs[0]
	}

	h.logger.Debug("batch processed successfully",
		"count", len(messages),
		"markets", len(marketGroups))

	return nil
}

// orderbookUpdateWithSeq holds an update with its sequence for sorting
type orderbookUpdateWithSeq struct {
	update   *model.DepthUpdate
	sequence uint64
}

// toDepthUpdate converts message to depth update
func (h *BatchOrderBookHandler) toDepthUpdate(msg *OrderBookUpdateMessage) (*model.DepthUpdate, error) {
	update := &model.DepthUpdate{
		Market:   msg.Market,
		Sequence: msg.Sequence,
		Bids:     make([]*model.PriceLevel, 0, len(msg.Bids)),
		Asks:     make([]*model.PriceLevel, 0, len(msg.Asks)),
	}

	for _, bid := range msg.Bids {
		price, err := decimal.NewFromString(bid.Price)
		if err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(bid.Amount)
		if err != nil {
			return nil, err
		}
		update.Bids = append(update.Bids, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	for _, ask := range msg.Asks {
		price, err := decimal.NewFromString(ask.Price)
		if err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(ask.Amount)
		if err != nil {
			return nil, err
		}
		update.Asks = append(update.Asks, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	return update, nil
}

// Topic returns the topic this handler processes
func (h *BatchOrderBookHandler) Topic() string {
	return "orderbook-updates"
}
