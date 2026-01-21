package service

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	workflowRepo    repository.WorkflowRepository
	coordinator     coordinator.Coordinator
	logger          logger.Logger
	mu              sync.RWMutex
	stopCh          chan struct{}
	workflowCache   map[string]*domain.Workflow // in-memory cache: "workflowID:version" -> workflow
	signalTypeIndex map[string][]string         // index: signalType -> []"workflowID:version"
}

// NewWorkflowManager creates a new workflow manager
func NewWorkflowManager(
	workflowRepo repository.WorkflowRepository,
	coordinator coordinator.Coordinator,
	log logger.Logger,
) WorkflowManager {
	return &workflowManager{
		workflowRepo:    workflowRepo,
		coordinator:     coordinator,
		logger:          log.With(logger.String("component", "workflow_manager")),
		stopCh:          make(chan struct{}),
		workflowCache:   make(map[string]*domain.Workflow),
		signalTypeIndex: make(map[string][]string),
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

	// Try to get workflow keys from signal type index
	if workflowKeys, exists := m.signalTypeIndex[signalType]; exists {
		// Get workflows from cache using workflowID:version keys
		workflows := make([]*domain.Workflow, 0, len(workflowKeys))
		allCached := true

		for _, key := range workflowKeys {
			if workflow, found := m.workflowCache[key]; found {
				workflows = append(workflows, workflow)
			} else {
				allCached = false
				break
			}
		}

		if allCached {
			m.mu.RUnlock()
			return workflows, nil
		}
	}
	m.mu.RUnlock()

	// Cache miss - query repository with write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if workflowKeys, exists := m.signalTypeIndex[signalType]; exists {
		workflows := make([]*domain.Workflow, 0, len(workflowKeys))
		allCached := true

		for _, key := range workflowKeys {
			if workflow, found := m.workflowCache[key]; found {
				workflows = append(workflows, workflow)
			} else {
				allCached = false
				break
			}
		}

		if allCached {
			return workflows, nil
		}
	}

	workflows, err := m.workflowRepo.GetBySignalType(ctx, signalType)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflows: %w", err)
	}

	// Update cache using workflowID:version as keys
	workflowKeys := make([]string, 0, len(workflows))
	for _, workflow := range workflows {
		key := m.getWorkflowKey(workflow.WorkflowID, workflow.Version)
		m.workflowCache[key] = workflow
		workflowKeys = append(workflowKeys, key)
	}

	// Update signal type index
	m.signalTypeIndex[signalType] = workflowKeys

	return workflows, nil
}

// GetWorkflow returns a specific workflow version (cache-first)
func (m *workflowManager) GetWorkflow(ctx context.Context, workflowID string, version int) (*domain.Workflow, error) {
	key := m.getWorkflowKey(workflowID, version)

	m.mu.RLock()
	// Try cache first
	if workflow, exists := m.workflowCache[key]; exists {
		m.mu.RUnlock()
		return workflow, nil
	}
	m.mu.RUnlock()

	// Cache miss - query repository with write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if workflow, exists := m.workflowCache[key]; exists {
		return workflow, nil
	}

	workflow, err := m.workflowRepo.GetByID(ctx, workflowID, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	// Cache the workflow
	m.workflowCache[key] = workflow

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

	// Clear existing caches
	m.workflowCache = make(map[string]*domain.Workflow)
	m.signalTypeIndex = make(map[string][]string)

	// Cache workflows by workflowID:version and build signal type index
	signalTypeMap := make(map[string][]string)
	for _, workflow := range result.Workflows {
		key := m.getWorkflowKey(workflow.WorkflowID, workflow.Version)
		m.workflowCache[key] = workflow
		signalTypeMap[workflow.SignalType] = append(signalTypeMap[workflow.SignalType], key)
	}

	// Update signal type index
	m.signalTypeIndex = signalTypeMap

	m.logger.Info("workflow cache refreshed",
		logger.Int("workflow_count", len(result.Workflows)),
		logger.Int("signal_types", len(m.signalTypeIndex)))

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

// getWorkflowKey generates cache key for workflowID and version
func (m *workflowManager) getWorkflowKey(workflowID string, version int) string {
	return fmt.Sprintf("%s:%d", workflowID, version)
}
