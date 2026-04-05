package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	kafkago "github.com/segmentio/kafka-go"
)

type MarketEventPublisher struct {
	writer *kafkago.Writer
}

func NewMarketEventPublisher(brokers []string, topic string) *MarketEventPublisher {
	if len(brokers) == 0 || topic == "" {
		return nil
	}

	return &MarketEventPublisher{
		writer: &kafkago.Writer{
			Addr:         kafkago.TCP(brokers...),
			Topic:        topic,
			RequiredAcks: kafkago.RequireOne,
		},
	}
}

func (p *MarketEventPublisher) PublishCandles(ctx context.Context, candles []candle.Candle) error {
	if p == nil || p.writer == nil || len(candles) == 0 {
		return nil
	}

	messages := make([]kafkago.Message, 0, len(candles))
	for _, item := range candles {
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		messages = append(messages, kafkago.Message{
			Key:   []byte(item.Pair),
			Value: body,
			Time:  time.Unix(item.Timestamp, 0).UTC(),
		})
	}

	if err := p.writer.WriteMessages(ctx, messages...); err != nil {
		return err
	}

	first := candles[0]
	slog.Info("market events published",
		"pair", first.Pair,
		"interval", first.Interval,
		"candle_count", len(candles),
	)
	return nil
}

func (p *MarketEventPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

type CandleSink interface {
	IngestRealtimeCandles(ctx context.Context, candles []candle.Candle) error
}

const (
	marketEventBatchWindow = 50 * time.Millisecond
	marketEventMaxBatch    = 256
)

type MarketEventConsumer struct {
	reader  *kafkago.Reader
	sink    CandleSink
	topic   string
	groupID string
}

func NewMarketEventConsumer(brokers []string, topic, groupID string, sink CandleSink) *MarketEventConsumer {
	if len(brokers) == 0 || topic == "" || groupID == "" || sink == nil {
		return nil
	}

	return &MarketEventConsumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,
			MaxWait: 1 * time.Second,
		}),
		sink:    sink,
		topic:   topic,
		groupID: groupID,
	}
}

func (c *MarketEventConsumer) Run(ctx context.Context) error {
	if c == nil || c.reader == nil {
		return nil
	}

	slog.Info("market event consumer started", "topic", c.topic, "group_id", c.groupID)
	defer slog.Info("market event consumer stopped", "topic", c.topic, "group_id", c.groupID)

	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		messages, err := c.collectBatch(ctx, message)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		if err := c.handleBatch(ctx, messages); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, messages...); err != nil {
			return err
		}
	}
}

func (c *MarketEventConsumer) handleMessage(ctx context.Context, message kafkago.Message) error {
	return c.handleBatch(ctx, []kafkago.Message{message})
}

// collectBatch opportunistically drains a short burst of already-available Kafka messages so the
// consumer can ingest them in grouped chunks instead of notifying the SSE path once per candle.
func (c *MarketEventConsumer) collectBatch(ctx context.Context, first kafkago.Message) ([]kafkago.Message, error) {
	messages := []kafkago.Message{first}
	deadline := time.Now().Add(marketEventBatchWindow)

	for len(messages) < marketEventMaxBatch {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		batchCtx, cancel := context.WithTimeout(ctx, remaining)
		next, err := c.reader.FetchMessage(batchCtx)
		cancel()
		if err != nil {
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				return messages, nil
			case errors.Is(err, context.Canceled) && ctx.Err() != nil:
				return nil, ctx.Err()
			default:
				return nil, err
			}
		}

		messages = append(messages, next)
	}

	return messages, nil
}

// handleBatch groups messages by pair/hour window because RealtimeIngestService consolidates one
// hourly series at a time. This keeps live ticker SSE updates aligned with realtime poll batches
// rather than the noisier per-minute message fanout produced by the Kafka topic.
func (c *MarketEventConsumer) handleBatch(ctx context.Context, messages []kafkago.Message) error {
	if len(messages) == 0 {
		return nil
	}

	grouped := make(map[string][]candle.Candle, len(messages))
	order := make([]string, 0, len(messages))
	invalidCount := 0

	for _, message := range messages {
		item, err := decodeMarketEvent(message)
		if err != nil {
			slog.Warn("market event consumer invalid payload", "error", err)
			invalidCount++
			continue
		}

		key := marketEventGroupKey(item)
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], item)
	}

	for _, key := range order {
		items := grouped[key]
		if len(items) == 0 {
			continue
		}

		first := items[0]
		slog.Info("market event batch consumed",
			"pair", first.Pair,
			"interval", first.Interval,
			"candle_count", len(items),
		)

		if err := c.sink.IngestRealtimeCandles(ctx, items); err != nil {
			return err
		}
	}

	if invalidCount > 0 {
		slog.Warn("market event batch skipped invalid payloads", "invalid_count", invalidCount, "message_count", len(messages))
	}

	return nil
}

func decodeMarketEvent(message kafkago.Message) (candle.Candle, error) {
	var item candle.Candle
	if err := json.Unmarshal(message.Value, &item); err != nil {
		return candle.Candle{}, err
	}
	return item, nil
}

func marketEventGroupKey(item candle.Candle) string {
	windowStart := time.Unix(item.Timestamp, 0).UTC().Truncate(time.Hour).Unix()
	return fmt.Sprintf("%s:%s:%d", item.Pair, item.Interval, windowStart)
}

func (c *MarketEventConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
