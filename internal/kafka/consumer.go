package kafka

import (
	"context"
	"encoding/json"
	"log"

	"github.com/diploma/analysis-api-service/internal/model"
	kafkago "github.com/segmentio/kafka-go"
)

type CompletedEventHandler interface {
	HandleStaticCompleted(ctx context.Context, event model.AnalysisCompletedEvent) error
	HandleCacheCompleted(ctx context.Context, event model.AnalysisCompletedEvent) error
}

type Consumer struct {
	handler CompletedEventHandler
	brokers string
}

func NewConsumer(brokers string, handler CompletedEventHandler) *Consumer {
	return &Consumer{
		handler: handler,
		brokers: brokers,
	}
}

func (c *Consumer) StartListening(ctx context.Context) {
	go c.listenTopic(ctx, TopicStaticCompleted, c.handleStaticCompleted)
	go c.listenTopic(ctx, TopicCacheCompleted, c.handleCacheCompleted)
	log.Println("[kafka-consumer] started listening for completed events")
}

func (c *Consumer) listenTopic(ctx context.Context, topic string, handler func(context.Context, []byte) error) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  []string{c.brokers},
		Topic:    topic,
		GroupID:  "analysis-api-" + topic,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[kafka-consumer] error reading %s: %v", topic, err)
				continue
			}
			log.Printf("[kafka-consumer] received from %s: %s", topic, string(msg.Value))
			if err := handler(ctx, msg.Value); err != nil {
				log.Printf("[kafka-consumer] error handling %s: %v", topic, err)
			}
		}
	}
}

func (c *Consumer) handleStaticCompleted(ctx context.Context, data []byte) error {
	var event model.AnalysisCompletedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}

	return c.handler.HandleStaticCompleted(ctx, event)
}

func (c *Consumer) handleCacheCompleted(ctx context.Context, data []byte) error {
	var event model.AnalysisCompletedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}

	return c.handler.HandleCacheCompleted(ctx, event)
}
