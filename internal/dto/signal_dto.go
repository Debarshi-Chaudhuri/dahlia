package dto

import "time"

// CreateSignalRequest represents request to create a signal
type CreateSignalRequest struct {
	SignalType string                 `json:"signal_type" binding:"required"`
	OrgID      string                 `json:"org_id" binding:"required"`
	Value      map[string]interface{} `json:"value"`
	Metadata   map[string]interface{} `json:"metadata"`
	Timestamp  time.Time              `json:"timestamp" binding:"required"`
}

// CreateSignalResponse represents response after creating a signal
type CreateSignalResponse struct {
	SignalID        string `json:"signal_id"`
	WorkflowsQueued int    `json:"workflows_queued"`
}
