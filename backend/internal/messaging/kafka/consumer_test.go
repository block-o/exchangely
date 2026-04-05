package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	kafkago "github.com/segmentio/kafka-go"
)

func TestTaskConsumerHandleMessageIgnoresInvalidPayload(t *testing.T) {
	handler := &fakeTaskHandler{}
	consumer := &TaskConsumer{handler: handler}

	err := consumer.handleMessage(context.Background(), kafkago.Message{
		Value: []byte("not-json"),
	})
	if err != nil {
		t.Fatalf("expected invalid payload to be ignored, got %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("expected handler to not be called, got %d", handler.calls)
	}
}

func TestTaskConsumerHandleMessageInvokesHandlerAndSwallowsHandlerErrors(t *testing.T) {
	handler := &fakeTaskHandler{err: errors.New("boom")}
	consumer := &TaskConsumer{handler: handler}

	body := []byte(`{"id":"backfill:BTCEUR:1h:1:2","type":"backfill","pair":"BTCEUR","interval":"1h","window_start":"2024-01-01T00:00:00Z","window_end":"2024-01-01T01:00:00Z"}`)
	err := consumer.handleMessage(context.Background(), kafkago.Message{Value: body})
	if err != nil {
		t.Fatalf("expected handler errors to be swallowed, got %v", err)
	}
	if handler.calls != 1 || handler.last.Pair != "BTCEUR" {
		t.Fatalf("unexpected handler state: %+v", handler)
	}
}

func TestMarketEventConsumerHandleMessageDecodesAndForwardsCandle(t *testing.T) {
	sink := &fakeCandleSink{}
	consumer := &MarketEventConsumer{sink: sink}

	body := []byte(`{"pair":"BTCUSD","interval":"1h","timestamp":1704067200,"open":10,"high":11,"low":9,"close":10.5,"volume":12,"source":"coingecko","finalized":false}`)
	if err := consumer.handleMessage(context.Background(), kafkago.Message{Value: body}); err != nil {
		t.Fatalf("handleMessage failed: %v", err)
	}

	if sink.calls != 1 || len(sink.items) != 1 {
		t.Fatalf("expected one forwarded candle, got calls=%d items=%d", sink.calls, len(sink.items))
	}
	if sink.items[0].Pair != "BTCUSD" || sink.items[0].Timestamp != 1704067200 {
		t.Fatalf("unexpected candle: %+v", sink.items[0])
	}
}

func TestMarketEventConsumerHandleMessageReturnsDecodeErrors(t *testing.T) {
	consumer := &MarketEventConsumer{sink: &fakeCandleSink{}}

	err := consumer.handleMessage(context.Background(), kafkago.Message{Value: []byte("{")})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

type fakeTaskHandler struct {
	calls int
	last  task.Task
	err   error
}

func (f *fakeTaskHandler) Process(_ context.Context, item task.Task) error {
	f.calls++
	f.last = item
	return f.err
}

type fakeCandleSink struct {
	calls int
	items []candle.Candle
	err   error
}

func (f *fakeCandleSink) IngestRealtimeCandles(_ context.Context, items []candle.Candle) error {
	f.calls++
	f.items = append([]candle.Candle{}, items...)
	return f.err
}

var _ = time.Time{}
