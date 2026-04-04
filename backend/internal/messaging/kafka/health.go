package kafka

import (
	"context"
	"errors"
	"net"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type HealthChecker struct {
	brokers []string
}

func NewHealthChecker(brokers []string) *HealthChecker {
	return &HealthChecker{brokers: brokers}
}

func (h *HealthChecker) Ping(ctx context.Context) error {
	if len(h.brokers) == 0 {
		return errors.New("no kafka brokers configured")
	}

	conn, err := kafkago.DialContext(ctx, "tcp", h.brokers[0])
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	return conn.SetDeadline(time.Now().Add(2 * time.Second))
}

func ResolveBroker(address string) (string, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}

	return host, nil
}
