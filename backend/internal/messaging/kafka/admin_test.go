package kafka

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestEnsureTopicsWithRetryRetriesUntilSuccess(t *testing.T) {
	attempts := 0
	err := ensureTopicsWithRetry(
		context.Background(),
		[]string{"broker:9092"},
		[]TopicConfig{{Name: "exchangely.tasks"}},
		3,
		0,
		func(context.Context, []string, []TopicConfig) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestEnsureTopicsWithRetryWrapsFinalError(t *testing.T) {
	err := ensureTopicsWithRetry(
		context.Background(),
		[]string{"broker:9092"},
		[]TopicConfig{{Name: "exchangely.tasks"}},
		2,
		0,
		func(context.Context, []string, []TopicConfig) error {
			return errors.New("still broken")
		},
	)
	if err == nil || err.Error() != "ensure kafka topics: still broken" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureTopicsOnceWithDialerNormalizesTopicConfig(t *testing.T) {
	root := &fakeAdminConn{
		controller: kafkago.Broker{Host: "controller", Port: 9093},
	}
	controller := &fakeAdminConn{}

	dialCalls := []string{}
	dial := func(_ context.Context, _ string, address string) (adminConn, error) {
		dialCalls = append(dialCalls, address)
		switch address {
		case "broker-a:9092":
			return root, nil
		case "controller:9093":
			return controller, nil
		default:
			return nil, fmt.Errorf("unexpected dial %s", address)
		}
	}

	err := ensureTopicsOnceWithDialer(context.Background(), []string{"broker-a:9092"}, []TopicConfig{
		{Name: "exchangely.tasks"},
		{Name: " ", NumPartitions: 99, ReplicationFactor: 99},
		{Name: "exchangely.market.ticks", NumPartitions: 0, ReplicationFactor: 0},
	}, dial)
	if err != nil {
		t.Fatalf("ensureTopicsOnceWithDialer failed: %v", err)
	}

	if len(controller.created) != 2 {
		t.Fatalf("expected 2 created topics, got %d", len(controller.created))
	}
	if controller.created[0].Topic != "exchangely.tasks" || controller.created[0].NumPartitions != 1 || controller.created[0].ReplicationFactor != 1 {
		t.Fatalf("unexpected first topic config: %+v", controller.created[0])
	}
	if controller.created[1].Topic != "exchangely.market.ticks" || controller.created[1].NumPartitions != 1 || controller.created[1].ReplicationFactor != 1 {
		t.Fatalf("unexpected second topic config: %+v", controller.created[1])
	}
	if len(dialCalls) != 2 {
		t.Fatalf("expected broker and controller dials, got %+v", dialCalls)
	}
}

func TestEnsureTopicsOnceWithDialerTriesNextBroker(t *testing.T) {
	root := &fakeAdminConn{
		controller: kafkago.Broker{Host: "controller", Port: 9093},
	}
	controller := &fakeAdminConn{}

	dial := func(_ context.Context, _ string, address string) (adminConn, error) {
		switch address {
		case "broker-a:9092":
			return nil, errors.New("dial failed")
		case "broker-b:9092":
			return root, nil
		case "controller:9093":
			return controller, nil
		default:
			return nil, fmt.Errorf("unexpected dial %s", address)
		}
	}

	err := ensureTopicsOnceWithDialer(context.Background(), []string{"broker-a:9092", "broker-b:9092"}, []TopicConfig{
		{Name: "exchangely.tasks"},
	}, dial)
	if err != nil {
		t.Fatalf("expected fallback broker success, got %v", err)
	}
}

func TestEnsureTopicsOnceWithDialerIgnoresAlreadyExists(t *testing.T) {
	root := &fakeAdminConn{
		controller: kafkago.Broker{Host: "controller", Port: 9093},
	}
	controller := &fakeAdminConn{
		createErr: errors.New("Topic with this name already exists"),
	}

	dial := func(_ context.Context, _ string, address string) (adminConn, error) {
		if address == "controller:9093" {
			return controller, nil
		}
		return root, nil
	}

	err := ensureTopicsOnceWithDialer(context.Background(), []string{"broker-a:9092"}, []TopicConfig{
		{Name: "exchangely.tasks"},
	}, dial)
	if err != nil {
		t.Fatalf("expected already-exists error to be ignored, got %v", err)
	}
}

type fakeAdminConn struct {
	controller kafkago.Broker
	created    []kafkago.TopicConfig
	createErr  error
}

func (f *fakeAdminConn) Controller() (kafkago.Broker, error) {
	if f.controller.Host == "" {
		return kafkago.Broker{}, errors.New("no controller")
	}
	return f.controller, nil
}

func (f *fakeAdminConn) CreateTopics(configs ...kafkago.TopicConfig) error {
	f.created = append(f.created, configs...)
	return f.createErr
}

func (f *fakeAdminConn) Close() error {
	return nil
}

var _ = time.Second
