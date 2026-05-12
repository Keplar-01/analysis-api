package kafka

import (
	"context"
	"encoding/json"
	"log"

	kafkago "github.com/segmentio/kafka-go"
)

const (
	TopicStartStatic     = "events.analysis.start_static"
	TopicStaticCompleted = "events.analysis.static_completed"
	TopicStartCache      = "events.analysis.start_cache"
	TopicCacheCompleted  = "events.analysis.cache_completed"
)

type Producer struct {
	writer *kafkago.Writer
}

func NewProducer(brokers string) *Producer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers),
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: kafkago.RequireAll,
	}
	return &Producer{writer: w}
}

func (p *Producer) Publish(ctx context.Context, topic string, key string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := kafkago.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
	}

	log.Printf("[kafka-producer] publishing to %s: %s", topic, string(data))
	return p.writer.WriteMessages(ctx, msg)
}

func (p *Producer) Close() error {
	return p.writer.Close()
}

func (p *Producer) Brokers() string {
	return p.writer.Addr.String()
}
