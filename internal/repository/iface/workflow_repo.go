package repository

import (
	"context"
	"dahlia/internal/domain"
)

// WorkflowPaginationResult contains paginated workflow results
type WorkflowPaginationResult struct {
	Workflows []*domain.Workflow
	NextToken string // Base64 encoded LastEvaluatedKey
}

// WorkflowRepository defines operations for workflows
type WorkflowRepository interface {
	Create(ctx context.Context, workflow *domain.Workflow) error
	Update(ctx context.Context, workflow *domain.Workflow) error
	GetByID(ctx context.Context, workflowID string, version int) (*domain.Workflow, error)
	GetBySignalType(ctx context.Context, signalType string) ([]*domain.Workflow, error)
	List(ctx context.Context, limit int, nextToken string) (*WorkflowPaginationResult, error)
	ListBySignalType(ctx context.Context, signalType string, limit int, nextToken string) (*WorkflowPaginationResult, error)
}
