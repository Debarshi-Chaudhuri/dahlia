package repository

import (
	"context"
	"dahlia/internal/domain"
)

// ActionLogRepository defines operations for action logs
type ActionLogRepository interface {
	Create(ctx context.Context, log *domain.ActionLog) error
	Update(ctx context.Context, log *domain.ActionLog) error
	GetByRunID(ctx context.Context, runID string) ([]*domain.ActionLog, error)
	GetByRunIDAndAction(ctx context.Context, runID string, actionIndex int) (*domain.ActionLog, error)
}
