package repository

import (
	"context"
	"dahlia/internal/domain"
)

// JobRepository defines operations for scheduled jobs
type JobRepository interface {
	Create(ctx context.Context, job *domain.ScheduledJob) error
	Update(ctx context.Context, job *domain.ScheduledJob) error
	GetByID(ctx context.Context, jobID string) (*domain.ScheduledJob, error)
	GetByStatus(ctx context.Context, status domain.JobStatus, limit int) ([]*domain.ScheduledJob, error)
}
