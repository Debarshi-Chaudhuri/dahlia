// internal/consumer/executor_queue/impl/executor_impl.go
package executor_impl

import (
	"context"
	executor "dahlia/internal/consumer/executor_queue/iface"
	"dahlia/internal/logger"
	queue "dahlia/internal/queue/iface"
	"dahlia/internal/repository/dynamodb"
	repositoryIface "dahlia/internal/repository/iface"
	"dahlia/internal/service"
	"errors"
)

type executorConsumer struct {
	logger       logger.Logger
	queue        queue.Queue
	workflowExec service.WorkflowExecutor
	signalRepo   repositoryIface.SignalRepository
}

// NewExecutorConsumer creates a new executor consumer
func NewExecutorConsumer(
	log logger.Logger,
	q queue.Queue,
	workflowExec service.WorkflowExecutor,
	signalRepo repositoryIface.SignalRepository,
) executor.ExecutorConsumer {
	return &executorConsumer{
		logger:       log.With(logger.String("component", "executor_consumer")),
		queue:        q,
		workflowExec: workflowExec,
		signalRepo:   signalRepo,
	}
}

// ProcessMessage implements ExecutorConsumer interface
func (e *executorConsumer) ProcessMessage(ctx context.Context, message executor.ExecutorMessage) bool {
	// Execute workflow
	err := e.workflowExec.Execute(
		ctx,
		message.SignalID,
		message.WorkflowID,
		message.WorkflowVersion,
		message.RunID,
		message.ResumeFrom,
	)

	if err != nil {
		// Check if it's an optimistic lock failure
		if dynamodb.IsOptimisticLockError(err) {
			e.logger.Warn("optimistic lock failed - workflow run was modified by another process, will retry",
				logger.String("signal_id", message.SignalID),
				logger.String("workflow_id", message.WorkflowID),
				logger.String("run_id", message.RunID),
				logger.Error(err))
			return false // Requeue for retry - another process may have updated the run
		}

		e.logger.Error("workflow execution failed",
			logger.String("signal_id", message.SignalID),
			logger.String("workflow_id", message.WorkflowID),
			logger.Error(err))
		return false // Retry
	}

	e.logger.Info("workflow executed successfully",
		logger.String("signal_id", message.SignalID),
		logger.String("workflow_id", message.WorkflowID))

	return true // Success - delete message
}

// SendMessage sends a message to the executor queue
func (e *executorConsumer) SendMessage(ctx context.Context, message executor.ExecutorMessage) error {
	if err := e.queue.Send(ctx, message); err != nil {
		e.logger.Error("failed to send message to executor queue",
			logger.String("signal_id", message.SignalID),
			logger.Error(err))
		return err
	}

	e.logger.Debug("message sent to executor queue successfully",
		logger.String("signal_id", message.SignalID))

	return nil
}

// isNotFoundError checks if error indicates resource not found
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check common error messages
	errMsg := err.Error()
	return errors.Is(err, dynamodb.ErrNotFound) ||
		errMsg == "signal not found" ||
		errMsg == "not found"
}
