package interfaces

import (
	"context"
)

type EventBus interface {
	Publish(ctx context.Context, topic string, payload interface{}) error
}
