package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

func ExampleUsage() {
	ctx := context.Background()

	err := Produce(ctx, []byte("user_id_123"), []byte("Hello Kafka"))
	if err != nil {
		fmt.Printf("failed to produce message: %v\n", err)
	}

	type UserEvent struct {
		UserID    string    `json:"user_id"`
		Action    string    `json:"action"`
		Timestamp time.Time `json:"timestamp"`
	}

	event := UserEvent{
		UserID:    "user_123",
		Action:    "login",
		Timestamp: time.Now(),
	}

	err = ProduceJSON(ctx, "user_login", event)
	if err != nil {
		fmt.Printf("failed to produce JSON message: %v\n", err)
	}

	err = ProduceJSONWithTopic(ctx, "user_events", "user_login", event)
	if err != nil {
		fmt.Printf("failed to produce message to topic: %v\n", err)
	}

	msg, err := Consume(ctx)
	if err == nil {
		fmt.Printf("received message: Key=%s, Value=%s\n", string(msg.Key), string(msg.Value))
	} else {
		fmt.Printf("failed to consume message: %v\n", err)
	}

	var userEvent UserEvent
	key, err := ConsumeJSON(ctx, &userEvent)
	if err == nil {
		fmt.Printf("received event: Key=%s, UserID=%s, Action=%s\n",
			key, userEvent.UserID, userEvent.Action)
	} else {
		fmt.Printf("failed to consume JSON message: %v\n", err)
	}

	StartConsumer(ctx, func(msg kafka.Message) error {
		fmt.Printf("processing message: Key=%s, Value=%s\n", string(msg.Key), string(msg.Value))
		return nil
	})

	StartJSONConsumer(ctx, func(key string, value []byte) error {
		var event UserEvent
		if err := json.Unmarshal(value, &event); err != nil {
			return err
		}
		fmt.Printf("processing event: Key=%s, UserID=%s, Action=%s\n",
			key, event.UserID, event.Action)
		return nil
	})

	StartConsumerWithTopic(ctx, "user_events", "user_service", func(msg kafka.Message) error {
		fmt.Printf("processing user event: Key=%s, Value=%s\n", string(msg.Key), string(msg.Value))
		return nil
	})

	fmt.Println("example completed")
}
