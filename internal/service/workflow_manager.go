package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	cache "dahlia/internal/cache/iface"
	coordinator "dahlia/internal/coordinator/iface"
	"dahlia/internal/domain"
	"dahlia/internal/logger"
	repository "dahlia/internal/repository/iface"
)

// WorkflowManager manages workflow cache with ZK coordination
type WorkflowManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetWorkflowsBySignalType(ctx context.Context, signalType string) ([]*domain.Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string, version int) (*domain.Workflow, error)
	RefreshCache(ctx context.Context) error
}

type workflowManager struct {
	workflowRepo repository.WorkflowRepository
	cache        cache.Cache
	coordinator  coordinator.Coordinator
	logger       logger.Logger
	mu           sync.RWMutex
	stopCh       chan struct{}
}

// NewWorkflowManager creates a new workflow manager
func NewWorkflowManager(
	workflowRepo repository.WorkflowRepository,
	cache cache.Cache,
	coordinator coordinator.Coordinator,
	log logger.Logger,
) WorkflowManager {
	return &workflowManager{
		workflowRepo: workflowRepo,
		cache:        cache,
		coordinator:  coordinator,
		logger:       log.With(logger.String("component", "workflow_manager")),
		stopCh:       make(chan struct{}),
	}
}

// Start initializes cache and sets up ZK watchers
func (m *workflowManager) Start(ctx context.Context) error {
	// Initial cache load
	if err := m.RefreshCache(ctx); err != nil {
		return fmt.Errorf("failed initial cache load: %w", err)
	}

	// Setup ZK watch for refresh trigger
	err := m.coordinator.WatchNode("/workflows/refresh", m.handleRefreshTrigger)
	if err != nil {
		m.logger.Warn("failed to setup ZK refresh watch",
			logger.Error(err))
		// Non-fatal - continue without watch
	}

	return nil
}

// Stop shuts down workflow manager
func (m *workflowManager) Stop(ctx context.Context) error {
	close(m.stopCh)
	return nil
}

// GetWorkflowsBySignalType returns all workflows for a signal type (cache-first)
func (m *workflowManager) GetWorkflowsBySignalType(ctx context.Context, signalType string) ([]*domain.Workflow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cacheKey := m.getCacheKey(signalType)

	// Try cache first
	cached, err := m.cache.Get(ctx, cacheKey)
	if err == nil {
		var workflows []*domain.Workflow
		if err := json.Unmarshal([]byte(cached), &workflows); err == nil {
			return workflows, nil
		}
	}

	// Cache miss - query repository
	workflows, err := m.workflowRepo.GetBySignalType(ctx, signalType)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflows: %w", err)
	}

	// Update cache
	m.cacheWorkflows(ctx, signalType, workflows)

	return workflows, nil
}

// GetWorkflow returns a specific workflow version
func (m *workflowManager) GetWorkflow(ctx context.Context, workflowID string, version int) (*domain.Workflow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	workflow, err := m.workflowRepo.GetByID(ctx, workflowID, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	return workflow, nil
}

// RefreshCache reloads all workflows from repository
func (m *workflowManager) RefreshCache(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all workflows
	result, err := m.workflowRepo.List(ctx, 50, "")
	if err != nil {
		return fmt.Errorf("failed to list workflows: %w", err)
	}

	// Group by signal type
	grouped := make(map[string][]*domain.Workflow)
	for _, workflow := range result.Workflows {
		grouped[workflow.SignalType] = append(grouped[workflow.SignalType], workflow)
	}

	// Update cache for each signal type
	for signalType, workflows := range grouped {
		m.cacheWorkflows(ctx, signalType, workflows)
	}

	return nil
}

// handleRefreshTrigger handles ZK refresh notifications
func (m *workflowManager) handleRefreshTrigger(data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.RefreshCache(ctx); err != nil {
		m.logger.Error("failed to refresh cache on ZK trigger",
			logger.Error(err))
	}
}

// cacheWorkflows stores workflows in cache
func (m *workflowManager) cacheWorkflows(ctx context.Context, signalType string, workflows []*domain.Workflow) {
	cacheKey := m.getCacheKey(signalType)

	data, err := json.Marshal(workflows)
	if err != nil {
		m.logger.Error("failed to marshal workflows",
			logger.String("signal_type", signalType),
			logger.Error(err))
		return
	}

	// Cache without TTL (invalidated via ZK watch)
	if err := m.cache.Set(ctx, cacheKey, string(data), 0); err != nil {
		m.logger.Error("failed to cache workflows",
			logger.String("signal_type", signalType),
			logger.Error(err))
		return
	}
}

// getCacheKey generates cache key for signal type
func (m *workflowManager) getCacheKey(signalType string) string {
	return fmt.Sprintf("workflow:%s", signalType)
}
