package dto

// GetRunRequest represents request to get a run
type GetRunRequest struct {
	// No body fields - run_id comes from path params
}

// GetRunResponse represents response for getting a run
type GetRunResponse struct {
	RunID              string                 `json:"run_id"`
	WorkflowID         string                 `json:"workflow_id"`
	WorkflowVersion    int                    `json:"workflow_version"`
	SignalID           string                 `json:"signal_id"`
	Status             string                 `json:"status"`        // Overall status: TRIGGERED, COMPLETED, FAILED
	CurrentState       string                 `json:"current_state"` // Detailed state: ACTION_0_STARTED, etc.
	CurrentEvalIndex   int                    `json:"current_eval_index"`
	CurrentActionIndex int                    `json:"current_action_index"`
	Context            map[string]interface{} `json:"context"`
	TriggeredAt        int64                  `json:"triggered_at"`
	CompletedAt        int64                  `json:"completed_at,omitempty"`
	CreatedAt          int64                  `json:"created_at"`
	UpdatedAt          int64                  `json:"updated_at"`
}

// ListRunsRequest represents request to list runs
type ListRunsRequest struct {
	// No body fields - all params come from query string
	// workflow_id, status, limit, next_token
}

// ListRunsResponse represents response for listing runs
type ListRunsResponse struct {
	Runs []RunResponse `json:"runs"`
	PaginationResponse
}

// RunResponse represents a single run in list response
type RunResponse struct {
	RunID              string                 `json:"run_id"`
	WorkflowID         string                 `json:"workflow_id"`
	WorkflowVersion    int                    `json:"workflow_version"`
	SignalID           string                 `json:"signal_id"`
	Status             string                 `json:"status"`        // Overall status: TRIGGERED, COMPLETED, FAILED
	CurrentState       string                 `json:"current_state"` // Detailed state: ACTION_0_STARTED, etc.
	CurrentEvalIndex   int                    `json:"current_eval_index"`
	CurrentActionIndex int                    `json:"current_action_index"`
	Context            map[string]interface{} `json:"context"`
	TriggeredAt        int64                  `json:"triggered_at"`
	CompletedAt        int64                  `json:"completed_at,omitempty"`
	CreatedAt          int64                  `json:"created_at"`
	UpdatedAt          int64                  `json:"updated_at"`
}
