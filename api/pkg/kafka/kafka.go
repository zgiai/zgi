package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

var (
	KafkaWriter *kafka.Writer
	KafkaReader *kafka.Reader
)

type Config struct {
	Brokers  []string
	Topic    string
	GroupID  string
	Producer ProducerConfig
	Consumer ConsumerConfig
}

type ProducerConfig struct {
	BatchSize    int
	BatchTimeout time.Duration
}

type ConsumerConfig struct {
	MinBytes int
	MaxBytes int
}

func Init(cfg Config) error {
	KafkaWriter = &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    cfg.Producer.BatchSize,
		BatchTimeout: cfg.Producer.BatchTimeout,
	}

	KafkaReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:  cfg.Brokers,
		GroupID:  cfg.GroupID,
		Topic:    cfg.Topic,
		MinBytes: cfg.Consumer.MinBytes,
		MaxBytes: cfg.Consumer.MaxBytes,
	})

	return nil
}

func Produce(ctx context.Context, key, value []byte) error {
	return KafkaWriter.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
		Time:  time.Now(),
	})
}

func ProduceJSON(ctx context.Context, key string, value interface{}) error {
	keyBytes := []byte(key)
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	return Produce(ctx, keyBytes, valueBytes)
}

func ProduceWithTopic(ctx context.Context, topic string, key, value []byte) error {
	writer := &kafka.Writer{
		Addr:         KafkaWriter.Addr,
		Topic:        topic,
		Balancer:     KafkaWriter.Balancer,
		BatchSize:    KafkaWriter.BatchSize,
		BatchTimeout: KafkaWriter.BatchTimeout,
	}
	defer writer.Close()

	return writer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
		Time:  time.Now(),
	})
}

func ProduceJSONWithTopic(ctx context.Context, topic string, key string, value interface{}) error {
	keyBytes := []byte(key)
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	return ProduceWithTopic(ctx, topic, keyBytes, valueBytes)
}

func ProduceBatch(ctx context.Context, messages []kafka.Message) error {
	return KafkaWriter.WriteMessages(ctx, messages...)
}

func Consume(ctx context.Context) (kafka.Message, error) {
	return KafkaReader.ReadMessage(ctx)
}

func ConsumeJSON(ctx context.Context, v interface{}) (string, error) {
	msg, err := Consume(ctx)
	if err != nil {
		return "", fmt.Errorf("消费消息失败: %v", err)
	}

	if err := json.Unmarshal(msg.Value, v); err != nil {
		return "", fmt.Errorf("解析消息失败: %v", err)
	}

	return string(msg.Key), nil
}

func ConsumeWithTopic(ctx context.Context, topic string, groupID string) (kafka.Message, error) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  KafkaReader.Config().Brokers,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: KafkaReader.Config().MinBytes,
		MaxBytes: KafkaReader.Config().MaxBytes,
	})
	defer reader.Close()

	return reader.ReadMessage(ctx)
}

func ConsumeJSONWithTopic(ctx context.Context, topic string, groupID string, v interface{}) (string, error) {
	msg, err := ConsumeWithTopic(ctx, topic, groupID)
	if err != nil {
		return "", fmt.Errorf("从主题 %s 消费消息失败: %v", topic, err)
	}

	if err := json.Unmarshal(msg.Value, v); err != nil {
		return "", fmt.Errorf("解析消息失败: %v", err)
	}

	return string(msg.Key), nil
}

func StartConsumer(ctx context.Context, handler func(msg kafka.Message) error) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := Consume(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					logger.WarnContext(ctx, "Kafka message consume failed", zap.Error(err))
					continue
				}

				if err := handler(msg); err != nil {
					logger.WarnContext(ctx, "Kafka message handler failed",
						zap.String("topic", msg.Topic),
						zap.Int("partition", msg.Partition),
						zap.Int64("offset", msg.Offset),
						zap.Error(err),
					)
				}
			}
		}
	}()
}

func StartJSONConsumer(ctx context.Context, handler func(key string, value []byte) error) {
	StartConsumer(ctx, func(msg kafka.Message) error {
		return handler(string(msg.Key), msg.Value)
	})
}

func StartConsumerWithTopic(ctx context.Context, topic string, groupID string, handler func(msg kafka.Message) error) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  KafkaReader.Config().Brokers,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: KafkaReader.Config().MinBytes,
		MaxBytes: KafkaReader.Config().MaxBytes,
	})

	go func() {
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
					logger.WarnContext(ctx, "Kafka topic message consume failed",
						zap.String("topic", topic),
						zap.String("group_id", groupID),
						zap.Error(err),
					)
					continue
				}

				if err := handler(msg); err != nil {
					logger.WarnContext(ctx, "Kafka topic message handler failed",
						zap.String("topic", topic),
						zap.String("group_id", groupID),
						zap.Int("partition", msg.Partition),
						zap.Int64("offset", msg.Offset),
						zap.Error(err),
					)
				}
			}
		}
	}()
}

func KafkaHandler(msg kafka.Message) error {
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Value, &payload); err != nil {
		return err
	}

	logger.Debug("Kafka message received",
		zap.String("topic", msg.Topic),
		zap.Int("partition", msg.Partition),
		zap.Int64("offset", msg.Offset),
		zap.Bool("key_present", len(msg.Key) > 0),
		zap.Int("value_bytes", len(msg.Value)),
		zap.Int("field_count", len(payload)),
	)
	return nil
}

func Close() error {
	if err := KafkaWriter.Close(); err != nil {
		return fmt.Errorf("failed to close kafka KafkaWriter: %v", err)
	}
	if err := KafkaReader.Close(); err != nil {
		return fmt.Errorf("failed to close kafka KafkaReader: %v", err)
	}
	return nil
}
