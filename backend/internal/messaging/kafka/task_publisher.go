package kafka

import (
	"context"
	"encoding/json"

	"github.com/block-o/exchangely/backend/internal/domain/task"
	kafkago "github.com/segmentio/kafka-go"
)

type TaskPublisher struct {
	writer *kafkago.Writer
}

func NewTaskPublisher(brokers []string, topic string) *TaskPublisher {
	if len(brokers) == 0 || topic == "" {
		return nil
	}

	return &TaskPublisher{
		writer: &kafkago.Writer{
			Addr:         kafkago.TCP(brokers...),
			Topic:        topic,
			RequiredAcks: kafkago.RequireOne,
		},
	}
}

func (p *TaskPublisher) Publish(ctx context.Context, tasks []task.Task) error {
	if p == nil || p.writer == nil || len(tasks) == 0 {
		return nil
	}

	messages := make([]kafkago.Message, 0, len(tasks))
	for _, item := range tasks {
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		messages = append(messages, kafkago.Message{
			Key:   []byte(item.Pair),
			Value: body,
		})
	}

	return p.writer.WriteMessages(ctx, messages...)
}

func (p *TaskPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
