// internal/consumer/executor_queue/provider.go
package executor_queue

import (
	"context"

	executor "dahlia/internal/consumer/executor_queue/iface"
	executorImpl "dahlia/internal/consumer/executor_queue/impl"
	"dahlia/internal/logger"
	queue "dahlia/internal/queue/iface"
	"dahlia/internal/queue/sqs"

	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"go.uber.org/fx"
)

// ExecutorQueueParams holds dependencies for executor queue
type ExecutorQueueParams struct {
	fx.In

	Logger    logger.Logger
	SQSClient *awssqs.Client
	// Add other dependencies as they're implemented:
	// WorkflowManager WorkflowManager
	// SignalRepo      SignalRepository
	// etc.
}

// ExecutorQueueResult holds what this module provides
type ExecutorQueueResult struct {
	fx.Out

	Consumer executor.ExecutorConsumer
	Queue    queue.Queue `name:"executor_queue"`
}

// ProvideExecutorQueueAndConsumer provides both queue and consumer with proper dependency injection
func ProvideExecutorQueueAndConsumer(params ExecutorQueueParams) ExecutorQueueResult {
	// Create a placeholder consumer first
	var consumer executor.ExecutorConsumer

	// Create the queue
	q := sqs.NewSQSQueue(
		params.SQSClient,
		sqs.QueueConfig{
			QueueURL:        "http://localhost:4566/000000000000/executor-queue",
			WorkerCount:     1,
			MaxMessages:     1,
			WaitTimeSeconds: 20,
		},
		// Use a wrapper that will call the actual consumer
		queue.MessageProcessorFunc[executor.ExecutorMessage](func(ctx context.Context, msg executor.ExecutorMessage) bool {
			return consumer.ProcessMessage(ctx, msg)
		}),
		params.Logger,
	)

	// Now create the consumer with the queue
	consumer = executorImpl.NewExecutorConsumer(params.Logger, q)

	return ExecutorQueueResult{
		Consumer: consumer,
		Queue:    q,
	}
}

// ExecutorQueueModule provides the FX module for executor queue
func ExecutorQueueModule() fx.Option {
	return fx.Options(
		fx.Provide(
			ProvideExecutorQueueAndConsumer,
		),
		fx.Invoke(func(params struct {
			fx.In
			Lifecycle fx.Lifecycle
			Queue     queue.Queue `name:"executor_queue"`
			Logger    logger.Logger
		}) {
			params.Lifecycle.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					params.Logger.Info("starting executor queue consumer")
					return params.Queue.StartConsumer(ctx)
				},
				OnStop: func(ctx context.Context) error {
					params.Logger.Info("stopping executor queue consumer")
					return params.Queue.StopConsumer(ctx)
				},
			})
		}),
	)
}
