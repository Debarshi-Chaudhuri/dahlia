package repository

import (
	"context"
	"dahlia/internal/domain"
)

// WorkflowRepository defines operations for workflows
type WorkflowRepository interface {
	Create(ctx context.Context, workflow *domain.Workflow) error
	Update(ctx context.Context, workflow *domain.Workflow) error
	GetByID(ctx context.Context, workflowID string, version int) (*domain.Workflow, error)
	GetBySignalType(ctx context.Context, signalType string) ([]*domain.Workflow, error)
	List(ctx context.Context, limit int) ([]*domain.Workflow, error)
}
