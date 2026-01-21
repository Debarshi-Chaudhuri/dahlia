// internal/queue/iface/queue.go
package queue

import (
	"context"
)

// Message represents a queue message
type Message interface {
	Body() string
	ReceiptHandle() string
	MessageID() string
}

// MessageProcessor processes a message and returns true if successful (for deletion)
type MessageProcessor[T any] interface {
	ProcessMessage(ctx context.Context, message T) bool
}

// MessageProcessorFunc allows functions to implement MessageProcessor
type MessageProcessorFunc[T any] func(ctx context.Context, message T) bool

func (f MessageProcessorFunc[T]) ProcessMessage(ctx context.Context, message T) bool {
	return f(ctx, message)
}

// Queue defines queue operations
type Queue interface {
	Send(ctx context.Context, message interface{}) error
	StartConsumer(ctx context.Context) error
	StopConsumer(ctx context.Context) error
}
