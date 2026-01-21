package handler

import (
	"context"
	"time"

	"dahlia/commons/error_handler"
	"dahlia/commons/handler"
	"dahlia/internal/domain"
	"dahlia/internal/dto"
	"dahlia/internal/logger"
	"dahlia/internal/service"
)

type SchedulerHandler struct {
	scheduler service.IScheduler
	logger    logger.Logger
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
	ioutil *handler.RequestIo[dto.ScheduleJobRequest],
) (dto.ScheduleJobResponse, *error_handler.ErrorCollection) {
	req := ioutil.Body

	// Use domain constructor to generate UUID and proper timestamps
	executeAt := time.UnixMilli(req.ExecuteAt)
	job := domain.NewScheduledJob(req.JobType, req.QueueName, executeAt, req.Payload)

	err := h.scheduler.ScheduleJob(ctx, job)

	if err != nil {
		h.logger.Error("failed to schedule job",
			logger.String("job_type", req.JobType),
			logger.Error(err))
		return dto.ScheduleJobResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeInternalServerError, "Failed to schedule job", nil)
	}

	return dto.ScheduleJobResponse{
		JobID:     job.JobID,
		JobType:   job.JobName,
		ExecuteAt: job.ExecuteAt,
		Status:    string(job.Status),
	}, nil
}
