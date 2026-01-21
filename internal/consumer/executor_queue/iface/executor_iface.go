package executor_queue

import (
	"context"
)

// ExecutorMessage represents a message in the executor queue
type ExecutorMessage struct {
	SignalID        string `json:"signal_id"`
	WorkflowID      string `json:"workflow_id"`
	WorkflowVersion int    `json:"workflow_version"`
	ResumeFrom      string `json:"resume_from,omitempty"` // e.g., "ACTION_2"
}

// ExecutorConsumer defines the interface for processing executor queue messages
type ExecutorConsumer interface {
	// ProcessMessage processes a single executor message
	// Returns true if processing succeeded (message should be deleted)
	ProcessMessage(ctx context.Context, message ExecutorMessage) bool

	// SendMessage sends a message to the executor queue
	SendMessage(ctx context.Context, message ExecutorMessage) error
}
