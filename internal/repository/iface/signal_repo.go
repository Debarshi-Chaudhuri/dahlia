package repository

import (
	"context"
	"dahlia/internal/domain"
	"time"
)

// SignalRepository defines operations for signals
type SignalRepository interface {
	Create(ctx context.Context, signal *domain.Signal) error
	GetByID(ctx context.Context, signalID string) (*domain.Signal, error)
	GetLastSignal(ctx context.Context, signalType, orgID string, before time.Time) (*domain.Signal, error)
	QueryBySignalType(ctx context.Context, signalType string, startTime, endTime time.Time) ([]*domain.Signal, error)
}
