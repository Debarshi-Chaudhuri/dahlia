package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ActionStatus represents the status of an action execution
type ActionStatus string

const (
	ActionStatusPending   ActionStatus = "PENDING"
	ActionStatusRunning   ActionStatus = "RUNNING"
	ActionStatusCompleted ActionStatus = "COMPLETED"
	ActionStatusFailed    ActionStatus = "FAILED"
	ActionStatusScheduled ActionStatus = "SCHEDULED"
)

// ActionLog represents a log entry for an executed action
type ActionLog struct {
	RunIDActionIndex string                 `json:"run_id_action_index" dynamodbav:"run_id_action_index"` // PK: run_id#action_index
	ExecutedAt       int64                  `json:"executed_at" dynamodbav:"executed_at"`                 // SK
	LogID            string                 `json:"log_id" dynamodbav:"log_id"`
	RunID            string                 `json:"run_id" dynamodbav:"run_id"`
	ActionIndex      int                    `json:"action_index" dynamodbav:"action_index"`
	ActionType       string                 `json:"action_type" dynamodbav:"action_type"`
	Status           ActionStatus           `json:"status" dynamodbav:"status"`
	Result           map[string]interface{} `json:"result" dynamodbav:"result"`
	ErrorMessage     string                 `json:"error_message,omitempty" dynamodbav:"error_message,omitempty"`
	DurationMs       int64                  `json:"duration_ms,omitempty" dynamodbav:"duration_ms,omitempty"`
}

// NewActionLog creates a new action log
func NewActionLog(runID string, actionIndex int, actionType string) *ActionLog {
	now := time.Now().UnixMilli()

	return &ActionLog{
		LogID:            uuid.New().String(),
		RunIDActionIndex: fmt.Sprintf("%s#%d", runID, actionIndex),
		ExecutedAt:       now,
		RunID:            runID,
		ActionIndex:      actionIndex,
		ActionType:       actionType,
		Status:           ActionStatusPending,
		Result:           make(map[string]interface{}),
	}
}

// MarkRunning marks the action as currently running
func (a *ActionLog) MarkRunning() {
	a.Status = ActionStatusRunning
	a.ExecutedAt = time.Now().UnixMilli()
}

// MarkCompleted marks the action as completed
func (a *ActionLog) MarkCompleted(result map[string]interface{}, durationMs int64) {
	a.Status = ActionStatusCompleted
	a.Result = result
	a.DurationMs = durationMs
	a.ExecutedAt = time.Now().UnixMilli()
}

// MarkFailed marks the action as failed
func (a *ActionLog) MarkFailed(errorMsg string, durationMs int64) {
	a.Status = ActionStatusFailed
	a.ErrorMessage = errorMsg
	a.DurationMs = durationMs
	a.ExecutedAt = time.Now().UnixMilli()
}

// MarkScheduled marks the action as scheduled (for delay actions)
func (a *ActionLog) MarkScheduled(scheduledTime int64) {
	a.Status = ActionStatusScheduled
	a.Result = map[string]interface{}{
		"scheduled_at": scheduledTime,
	}
	a.ExecutedAt = time.Now().UnixMilli()
}
