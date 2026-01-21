package repository

import (
	"context"
	"dahlia/internal/domain"
)

// RunRepository defines operations for workflow runs
type RunRepository interface {
	Create(ctx context.Context, run *domain.WorkflowRun) error
	Update(ctx context.Context, run *domain.WorkflowRun) error
	GetByID(ctx context.Context, runID string) (*domain.WorkflowRun, error)
	GetByWorkflowID(ctx context.Context, workflowID string, limit int) ([]*domain.WorkflowRun, error)
	GetByStatus(ctx context.Context, status domain.RunStatus, limit int) ([]*domain.WorkflowRun, error)
}
