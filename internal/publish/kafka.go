//go:build kafka

package publish

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/Fhionnghaile/aegis-gateway/internal/event"
)

// KafkaPublisher writes events to Kafka/Redpanda, keyed by source|type.
type KafkaPublisher struct{ w *kafka.Writer }

func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	return &KafkaPublisher{w: &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
		Async:        false,
	}}
}

func (k *KafkaPublisher) Publish(ctx context.Context, e event.Event) error {
	v, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return k.w.WriteMessages(ctx, kafka.Message{Key: e.PartitionKey(), Value: v})
}

func (k *KafkaPublisher) Close() error { return k.w.Close() }
