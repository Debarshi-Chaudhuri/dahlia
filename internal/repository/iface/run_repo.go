package repository

import (
	"context"
	"dahlia/internal/domain"
)

// PaginationResult contains paginated results
type PaginationResult struct {
	Runs      []*domain.WorkflowRun
	NextToken string // Base64 encoded LastEvaluatedKey
}

// RunRepository defines operations for workflow runs
type RunRepository interface {
	Create(ctx context.Context, run *domain.WorkflowRun) error
	Update(ctx context.Context, run *domain.WorkflowRun) error
	GetByID(ctx context.Context, runID string) (*domain.WorkflowRun, error)
	GetByWorkflowID(ctx context.Context, workflowID string, limit int, nextToken string) (*PaginationResult, error)
	GetByStatus(ctx context.Context, status domain.RunStatus, limit int, nextToken string) (*PaginationResult, error)
	GetRunningWorkflows(ctx context.Context, limit int, nextToken string) (*PaginationResult, error)
}
