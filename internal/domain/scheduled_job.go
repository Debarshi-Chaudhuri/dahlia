package domain

import (
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the status of a scheduled job
type JobStatus string

const (
	JobStatusScheduled JobStatus = "SCHEDULED"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
)

// ScheduledJob represents a job scheduled for delayed execution
type ScheduledJob struct {
	JobID      string                 `json:"job_id" dynamodbav:"job_id"`
	ExecuteAt  int64                  `json:"execute_at" dynamodbav:"execute_at"`
	CreatedAt  int64                  `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt  int64                  `json:"updated_at" dynamodbav:"updated_at"`
	JobName    string                 `json:"job_name" dynamodbav:"job_name"`
	JobDetails map[string]interface{} `json:"job_details" dynamodbav:"job_details"`
	QueueName  string                 `json:"queue_name" dynamodbav:"queue_name"`
	Status     JobStatus              `json:"status" dynamodbav:"status"`
}

// NewScheduledJob creates a new scheduled job
func NewScheduledJob(jobName, queueName string, executeAt time.Time, jobDetails map[string]interface{}) *ScheduledJob {
	now := time.Now().UnixMilli()
	return &ScheduledJob{
		JobID:      generateJobID(),
		ExecuteAt:  executeAt.UnixMilli(),
		CreatedAt:  now,
		UpdatedAt:  now,
		JobName:    jobName,
		JobDetails: jobDetails,
		QueueName:  queueName,
		Status:     JobStatusScheduled,
	}
}

func generateJobID() string {
	return uuid.New().String()
}
