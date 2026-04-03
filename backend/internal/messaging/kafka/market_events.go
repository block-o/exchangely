package kafka

import (
	"context"
	"encoding/json"
	"errors"
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

		if err := c.handleMessage(ctx, message); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return err
		}
	}
}

func (c *MarketEventConsumer) handleMessage(ctx context.Context, message kafkago.Message) error {
	var item candle.Candle
	if err := json.Unmarshal(message.Value, &item); err != nil {
		slog.Warn("market event consumer invalid payload", "error", err)
		return err
	}
	slog.Info("market event consumed",
		"pair", item.Pair,
		"interval", item.Interval,
		"timestamp", time.Unix(item.Timestamp, 0).UTC().Format(time.RFC3339),
	)
	return c.sink.IngestRealtimeCandles(ctx, []candle.Candle{item})
}

func (c *MarketEventConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
