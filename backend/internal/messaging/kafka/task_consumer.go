package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
	kafkago "github.com/segmentio/kafka-go"
)

type TaskHandler interface {
	Process(ctx context.Context, item task.Task) error
}

type TaskConsumer struct {
	reader  *kafkago.Reader
	handler TaskHandler
}

func NewTaskConsumer(brokers []string, topic, groupID string, handler TaskHandler) *TaskConsumer {
	if len(brokers) == 0 || topic == "" || groupID == "" || handler == nil {
		return nil
	}

	return &TaskConsumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,
			MaxWait: 1 * time.Second,
		}),
		handler: handler,
	}
}

func (c *TaskConsumer) Run(ctx context.Context) error {
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

		var item task.Task
		if err := json.Unmarshal(message.Value, &item); err != nil {
			log.Printf("task consumer invalid payload: %v", err)
			if commitErr := c.reader.CommitMessages(ctx, message); commitErr != nil {
				return commitErr
			}
			continue
		}

		if err := c.handler.Process(ctx, item); err != nil {
			log.Printf("task consumer processing failed for %s: %v", item.ID, err)
		}

		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return err
		}
	}
}

func (c *TaskConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
