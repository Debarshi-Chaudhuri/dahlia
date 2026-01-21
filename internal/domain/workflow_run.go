package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RunStatus represents the overall status of a workflow run (for GSI queries)
type RunStatus string

const (
	RunStatusTriggered RunStatus = "TRIGGERED"
	RunStatusCompleted RunStatus = "COMPLETED"
	RunStatusFailed    RunStatus = "FAILED"
)

// WorkflowRun represents an execution of a workflow
type WorkflowRun struct {
	RunID              string                 `json:"run_id" dynamodbav:"run_id"`
	CreatedAt          int64                  `json:"created_at" dynamodbav:"created_at"`
	WorkflowID         string                 `json:"workflow_id" dynamodbav:"workflow_id"`
	WorkflowVersion    int                    `json:"workflow_version" dynamodbav:"workflow_version"`
	SignalID           string                 `json:"signal_id" dynamodbav:"signal_id"`
	Status             RunStatus              `json:"status" dynamodbav:"status"`               // Overall status: TRIGGERED, COMPLETED, FAILED (for GSI)
	CurrentState       string                 `json:"current_state" dynamodbav:"current_state"` // Detailed state: TRIGGERED, ACTION_0_STARTED, ACTION_1_COMPLETED, etc.
	CurrentEvalIndex   int                    `json:"current_eval_index" dynamodbav:"current_eval_index"`
	CurrentActionIndex int                    `json:"current_action_index" dynamodbav:"current_action_index"`
	Context            map[string]interface{} `json:"context" dynamodbav:"context"`
	TriggeredAt        int64                  `json:"triggered_at" dynamodbav:"triggered_at"`
	CompletedAt        int64                  `json:"completed_at,omitempty" dynamodbav:"completed_at,omitempty"`
	UpdatedAt          int64                  `json:"updated_at" dynamodbav:"updated_at"`
}

// NewWorkflowRun creates a new workflow run
func NewWorkflowRun(workflowID string, workflowVersion int, signalID string, context map[string]interface{}) *WorkflowRun {
	now := time.Now().UnixMilli()

	if context == nil {
		context = make(map[string]interface{})
	}

	return &WorkflowRun{
		RunID:              uuid.New().String(),
		CreatedAt:          now,
		WorkflowID:         workflowID,
		WorkflowVersion:    workflowVersion,
		SignalID:           signalID,
		Status:             RunStatusTriggered,         // Overall status for GSI
		CurrentState:       string(RunStatusTriggered), // Initial detailed state
		CurrentEvalIndex:   0,
		CurrentActionIndex: 0,
		Context:            context,
		TriggeredAt:        now,
		UpdatedAt:          now,
	}
}

// GetConditionStatus returns status for a specific condition index
// e.g., "CONDITION_0", "CONDITION_1"
func (r *WorkflowRun) GetConditionStatus(index int) string {
	return fmt.Sprintf("CONDITION_%d", index)
}

// GetActionStatus returns status for a specific action index
// e.g., "ACTION_0", "ACTION_1"
func (r *WorkflowRun) GetActionStatus(index int) string {
	return fmt.Sprintf("ACTION_%d", index)
}

// GetActionStartedStatus returns started status for a specific action index
// e.g., "ACTION_0_STARTED", "ACTION_1_STARTED"
func (r *WorkflowRun) GetActionStartedStatus(index int) string {
	return fmt.Sprintf("ACTION_%d_STARTED", index)
}

// GetActionCompletedStatus returns completed status for a specific action index
// e.g., "ACTION_0_COMPLETED", "ACTION_1_COMPLETED"
func (r *WorkflowRun) GetActionCompletedStatus(index int) string {
	return fmt.Sprintf("ACTION_%d_COMPLETED", index)
}

// GetActionScheduledStatus returns scheduled status for a specific action index
// e.g., "ACTION_0_SCHEDULED", "ACTION_1_SCHEDULED"
func (r *WorkflowRun) GetActionScheduledStatus(index int) string {
	return fmt.Sprintf("ACTION_%d_SCHEDULED", index)
}

// SetConditionStatus updates current_state to specific condition index
func (r *WorkflowRun) SetConditionStatus(index int) {
	r.CurrentState = r.GetConditionStatus(index)
	r.CurrentEvalIndex = index
	r.UpdatedAt = time.Now().UnixMilli()
	// Status remains TRIGGERED during execution
}

// SetActionStatus updates current_state to specific action index
func (r *WorkflowRun) SetActionStatus(index int) {
	r.CurrentState = r.GetActionStatus(index)
	r.CurrentActionIndex = index
	r.UpdatedAt = time.Now().UnixMilli()
	// Status remains TRIGGERED during execution
}

// SetActionStartedStatus updates current_state to action started
func (r *WorkflowRun) SetActionStartedStatus(index int) {
	r.CurrentState = r.GetActionStartedStatus(index)
	r.CurrentActionIndex = index
	r.UpdatedAt = time.Now().UnixMilli()
	// Status remains TRIGGERED during execution
}

// SetActionCompletedStatus updates current_state to action completed
func (r *WorkflowRun) SetActionCompletedStatus(index int) {
	r.CurrentState = r.GetActionCompletedStatus(index)
	r.CurrentActionIndex = index
	r.UpdatedAt = time.Now().UnixMilli()
	// Status remains TRIGGERED during execution
}

// SetActionScheduledStatus updates current_state to specific action scheduled
func (r *WorkflowRun) SetActionScheduledStatus(index int) {
	r.CurrentState = r.GetActionScheduledStatus(index)
	r.CurrentActionIndex = index
	r.UpdatedAt = time.Now().UnixMilli()
	// Status remains TRIGGERED during execution
}

// MarkCompleted marks the run as completed
func (r *WorkflowRun) MarkCompleted() {
	r.Status = RunStatusCompleted               // Update overall status for GSI
	r.CurrentState = string(RunStatusCompleted) // Update detailed state
	r.CompletedAt = time.Now().UnixMilli()
	r.UpdatedAt = r.CompletedAt
}

// MarkFailed marks the run as failed
func (r *WorkflowRun) MarkFailed() {
	r.Status = RunStatusFailed               // Update overall status for GSI
	r.CurrentState = string(RunStatusFailed) // Update detailed state
	r.UpdatedAt = time.Now().UnixMilli()
}
