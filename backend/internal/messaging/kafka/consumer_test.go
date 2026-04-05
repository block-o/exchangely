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

	if sink.calls != 1 {
		t.Fatalf("expected one sink call, got %d", sink.calls)
	}
	if len(sink.batches) != 1 {
		t.Fatalf("expected one forwarded batch, got %d", len(sink.batches))
	}
	if len(sink.batches[0]) != 1 {
		t.Fatalf("expected one forwarded candle, got %d", len(sink.batches[0]))
	}
	if sink.batches[0][0].Pair != "BTCUSD" || sink.batches[0][0].Timestamp != 1704067200 {
		t.Fatalf("unexpected candle: %+v", sink.batches[0][0])
	}
}

func TestMarketEventConsumerHandleMessageSkipsInvalidPayloads(t *testing.T) {
	sink := &fakeCandleSink{}
	consumer := &MarketEventConsumer{sink: sink}

	err := consumer.handleMessage(context.Background(), kafkago.Message{Value: []byte("{")})
	if err != nil {
		t.Fatalf("expected invalid payload to be skipped, got %v", err)
	}
	if sink.calls != 0 {
		t.Fatalf("expected sink to not be called, got %d", sink.calls)
	}
}

func TestMarketEventConsumerHandleBatchGroupsByPairAndHour(t *testing.T) {
	sink := &fakeCandleSink{}
	consumer := &MarketEventConsumer{sink: sink}

	encode := func(body string) kafkago.Message {
		return kafkago.Message{Value: []byte(body)}
	}

	err := consumer.handleBatch(context.Background(), []kafkago.Message{
		encode(`{"pair":"BTCUSD","interval":"1h","timestamp":1704067200,"open":10,"high":11,"low":9,"close":10.5,"volume":12,"source":"kraken","finalized":false}`),
		encode(`{"pair":"BTCUSD","interval":"1h","timestamp":1704067260,"open":10.5,"high":11.5,"low":10,"close":11,"volume":9,"source":"kraken","finalized":false}`),
		encode(`{"pair":"ETHUSD","interval":"1h","timestamp":1704067200,"open":20,"high":21,"low":19,"close":20.5,"volume":5,"source":"kraken","finalized":false}`),
		encode(`{"pair":"BTCUSD","interval":"1h","timestamp":1704070800,"open":11,"high":12,"low":10.5,"close":11.2,"volume":4,"source":"kraken","finalized":false}`),
	})
	if err != nil {
		t.Fatalf("handleBatch failed: %v", err)
	}

	if sink.calls != 3 {
		t.Fatalf("expected 3 grouped sink calls, got %d", sink.calls)
	}
	if len(sink.batches) != 3 {
		t.Fatalf("expected 3 grouped batches, got %d", len(sink.batches))
	}
	if len(sink.batches[0]) != 2 || sink.batches[0][0].Pair != "BTCUSD" {
		t.Fatalf("unexpected first batch: %+v", sink.batches[0])
	}
	if len(sink.batches[1]) != 1 || sink.batches[1][0].Pair != "ETHUSD" {
		t.Fatalf("unexpected second batch: %+v", sink.batches[1])
	}
	if len(sink.batches[2]) != 1 || sink.batches[2][0].Timestamp != 1704070800 {
		t.Fatalf("unexpected third batch: %+v", sink.batches[2])
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
	calls   int
	batches [][]candle.Candle
	err     error
}

func (f *fakeCandleSink) IngestRealtimeCandles(_ context.Context, items []candle.Candle) error {
	f.calls++
	f.batches = append(f.batches, append([]candle.Candle{}, items...))
	return f.err
}

var _ = time.Time{}
