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
	if len(brokers) == 0 || len(topics) == 0 {
		return nil
	}

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if err := ensureTopicsOnce(ctx, brokers, topics); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return fmt.Errorf("ensure kafka topics: %w", lastErr)
}

func ensureTopicsOnce(ctx context.Context, brokers []string, topics []TopicConfig) error {
	var conn *kafkago.Conn
	var err error
	for _, broker := range brokers {
		conn, err = kafkago.DialContext(ctx, "tcp", broker)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}

	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))
	controllerConn, err := kafkago.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return err
	}
	defer controllerConn.Close()

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
