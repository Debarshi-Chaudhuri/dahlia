package dto

// ScheduleJobRequest represents request to schedule a job
type ScheduleJobRequest struct {
	JobType   string                 `json:"job_type" binding:"required"`
	Payload   map[string]interface{} `json:"payload" binding:"required"`
	ExecuteAt int64                  `json:"execute_at" binding:"required"`
}

// ScheduleJobResponse represents response after scheduling a job
type ScheduleJobResponse struct {
	JobID     string `json:"job_id"`
	JobType   string `json:"job_type"`
	ExecuteAt int64  `json:"execute_at"`
	Status    string `json:"status"`
}
