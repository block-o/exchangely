package kafka

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type TopicConfig struct {
	Name              string
	NumPartitions     int
	ReplicationFactor int
}

func EnsureTopics(ctx context.Context, brokers []string, topics []TopicConfig) error {
	return ensureTopicsWithRetry(ctx, brokers, topics, 10, time.Second, ensureTopicsOnce)
}

func ensureTopicsWithRetry(
	ctx context.Context,
	brokers []string,
	topics []TopicConfig,
	attempts int,
	retryDelay time.Duration,
	ensureOnce func(context.Context, []string, []TopicConfig) error,
) error {
	if len(brokers) == 0 || len(topics) == 0 {
		return nil
	}
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ensureOnce(ctx, brokers, topics); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == attempts-1 {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	return fmt.Errorf("ensure kafka topics: %w", lastErr)
}

type adminConn interface {
	Controller() (kafkago.Broker, error)
	CreateTopics(...kafkago.TopicConfig) error
	Close() error
}

type dialAdminConn func(context.Context, string, string) (adminConn, error)

type kafkaAdminConn struct {
	*kafkago.Conn
}

func defaultDialAdminConn(ctx context.Context, network, address string) (adminConn, error) {
	conn, err := kafkago.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return kafkaAdminConn{Conn: conn}, nil
}

func ensureTopicsOnce(ctx context.Context, brokers []string, topics []TopicConfig) error {
	return ensureTopicsOnceWithDialer(ctx, brokers, topics, defaultDialAdminConn)
}

func ensureTopicsOnceWithDialer(ctx context.Context, brokers []string, topics []TopicConfig, dial dialAdminConn) error {
	var conn adminConn
	var err error
	for _, broker := range brokers {
		conn, err = dial(ctx, "tcp", broker)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}

	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))
	controllerConn, err := dial(ctx, "tcp", controllerAddr)
	if err != nil {
		return err
	}
	defer func() {
		_ = controllerConn.Close()
	}()

	configs := make([]kafkago.TopicConfig, 0, len(topics))
	for _, topic := range topics {
		if strings.TrimSpace(topic.Name) == "" {
			continue
		}
		partitions := topic.NumPartitions
		if partitions <= 0 {
			partitions = 1
		}
		replicas := topic.ReplicationFactor
		if replicas <= 0 {
			replicas = 1
		}
		configs = append(configs, kafkago.TopicConfig{
			Topic:             topic.Name,
			NumPartitions:     partitions,
			ReplicationFactor: replicas,
		})
	}

	if len(configs) == 0 {
		return nil
	}

	if err := controllerConn.CreateTopics(configs...); err != nil && !strings.Contains(err.Error(), "Topic with this name already exists") {
		return err
	}

	return nil
}
