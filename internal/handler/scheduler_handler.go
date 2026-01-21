package handler

import (
	"context"

	"dahlia/commons/error_handler"
	"dahlia/commons/handler"
	"dahlia/internal/domain"
	"dahlia/internal/logger"
	"dahlia/internal/service"
)

type SchedulerHandler struct {
	scheduler service.IScheduler
	logger    logger.Logger
}

// ScheduleRequest represents the request to schedule a job
type ScheduleRequest struct {
	JobName      string                 `json:"job_name" binding:"required"`
	QueueName    string                 `json:"queue_name" binding:"required"`
	DelaySeconds int                    `json:"delay_seconds" binding:"required,min=60"`
	JobDetails   map[string]interface{} `json:"job_details"`
}

// ScheduleResponse represents the response after scheduling
type ScheduleResponse struct {
	JobID       string `json:"job_id,omitempty"`
	ExecuteAt   int64  `json:"execute_at,omitempty"`
	Message     string `json:"message"`
	IsImmediate bool   `json:"is_immediate"` // true if handled by goroutine
}

// NewSchedulerHandler creates a new scheduler handler
func NewSchedulerHandler(scheduler service.IScheduler, log logger.Logger) *SchedulerHandler {
	return &SchedulerHandler{
		scheduler: scheduler,
		logger:    log.With(logger.String("component", "scheduler_handler")),
	}
}

// ScheduleService handles job scheduling
func (h *SchedulerHandler) ScheduleService(
	ctx context.Context,
	ioutil *handler.RequestIo[ScheduleRequest],
) (ScheduleResponse, *error_handler.ErrorCollection) {
	req := ioutil.Body

	h.logger.Info("schedule job request received",
		logger.String("job_name", req.JobName),
		logger.String("queue_name", req.QueueName),
		logger.Int("delay_seconds", req.DelaySeconds))

	// Validate delay
	if req.DelaySeconds < 60 {
		h.logger.Warn("invalid delay seconds",
			logger.Int("delay_seconds", req.DelaySeconds))
		return ScheduleResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeValidationError, "delay_seconds must be at least 1", nil)
	}

	// Schedule job with delay
	job, err := h.scheduler.ScheduleWithDelay(
		ctx,
		req.JobName,
		req.QueueName,
		req.DelaySeconds,
		req.JobDetails,
	)

	if err != nil {
		h.logger.Error("failed to schedule job",
			logger.String("job_name", req.JobName),
			logger.Error(err))
		return ScheduleResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeInternalServerError, "Failed to schedule job", nil)
	}

	// If job is nil, it means it was handled immediately
	if job == nil {
		h.logger.Info("job scheduled for immediate execution (< 60s delay)")
		return ScheduleResponse{
			Message:     "Job scheduled for immediate execution",
			IsImmediate: true,
		}, nil
	}

	h.logger.Info("job scheduled successfully",
		logger.String("job_id", job.JobID),
		logger.Int("execute_at", int(job.ExecuteAt)))

	return ScheduleResponse{
		JobID:       job.JobID,
		ExecuteAt:   job.ExecuteAt,
		Message:     "Job scheduled successfully",
		IsImmediate: false,
	}, nil
}

// GetJobStatusService retrieves the status of a scheduled job
func (h *SchedulerHandler) GetJobStatusService(
	ctx context.Context,
	ioutil *handler.RequestIo[struct{}],
) (JobStatusResponse, *error_handler.ErrorCollection) {
	jobID := ioutil.PathParams["job_id"]

	h.logger.Info("get job status request",
		logger.String("job_id", jobID))

	// TODO: Implement when we have GetByID in service layer

	return JobStatusResponse{
		JobID:  jobID,
		Status: string(domain.JobStatusScheduled),
	}, nil
}

// JobStatusResponse represents job status
type JobStatusResponse struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	ExecuteAt int64  `json:"execute_at,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}
