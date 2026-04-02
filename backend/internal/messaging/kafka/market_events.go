package kafka

import (
	"context"
	"encoding/json"
	"errors"
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

	return p.writer.WriteMessages(ctx, messages...)
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
	reader *kafkago.Reader
	sink   CandleSink
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
		sink: sink,
	}
}

func (c *MarketEventConsumer) Run(ctx context.Context) error {
	if c == nil || c.reader == nil {
		return nil
	}

	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		var item candle.Candle
		if err := json.Unmarshal(message.Value, &item); err != nil {
			return err
		}
		if err := c.sink.IngestRealtimeCandles(ctx, []candle.Candle{item}); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return err
		}
	}
}

func (c *MarketEventConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
