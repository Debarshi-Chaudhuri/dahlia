package service

import (
	"context"
	"fmt"
	"time"

	"dahlia/internal/domain"
	"dahlia/internal/logger"
	repository "dahlia/internal/repository/iface"
)

// WorkflowExecutor executes workflow runs
type WorkflowExecutor interface {
	Execute(ctx context.Context, signalID, workflowID string, workflowVersion int, runID string, resumeFrom string) error
}

type workflowExecutor struct {
	signalRepo      repository.SignalRepository
	workflowRepo    repository.WorkflowRepository
	runRepo         repository.RunRepository
	actionLogRepo   repository.ActionLogRepository
	workflowManager WorkflowManager
	conditionEval   ConditionEvaluator
	actionExecutor  ActionExecutor
	logger          logger.Logger
}

// NewWorkflowExecutor creates a new workflow executor
func NewWorkflowExecutor(
	signalRepo repository.SignalRepository,
	workflowRepo repository.WorkflowRepository,
	runRepo repository.RunRepository,
	actionLogRepo repository.ActionLogRepository,
	workflowManager WorkflowManager,
	conditionEval ConditionEvaluator,
	actionExecutor ActionExecutor,
	log logger.Logger,
) WorkflowExecutor {
	return &workflowExecutor{
		signalRepo:      signalRepo,
		workflowRepo:    workflowRepo,
		runRepo:         runRepo,
		actionLogRepo:   actionLogRepo,
		workflowManager: workflowManager,
		conditionEval:   conditionEval,
		actionExecutor:  actionExecutor,
		logger:          log.With(logger.String("component", "workflow_executor")),
	}
}

// Execute executes a workflow run (new or resume)
func (e *workflowExecutor) Execute(ctx context.Context, signalID, workflowID string, workflowVersion int, runID string, resumeFrom string) error {

	e.logger.Info("executing workflow",
		logger.String("signal_id", signalID),
		logger.String("workflow_id", workflowID),
		logger.Int("workflow_version", workflowVersion),
		logger.String("resume_from", resumeFrom))

	// Fetch signal
	signal, err := e.signalRepo.GetByID(ctx, signalID)
	if err != nil {
		e.logger.Error("failed to get signal", logger.Error(err))
		return fmt.Errorf("failed to get signal: %w", err)
	}

	// Fetch workflow
	workflow, err := e.workflowManager.GetWorkflow(ctx, workflowID, workflowVersion)
	if err != nil {
		e.logger.Error("failed to get workflow", logger.Error(err))
		return fmt.Errorf("failed to get workflow: %w", err)
	}

	// Check if resuming or new run
	if resumeFrom != "" {
		return e.resumeWorkflow(ctx, signal, workflow, resumeFrom, runID)
	}

	return e.executeNewWorkflow(ctx, signal, workflow)
}

// executeNewWorkflow executes a new workflow run
func (e *workflowExecutor) executeNewWorkflow(ctx context.Context, signal *domain.Signal, workflow *domain.Workflow) error {
	e.logger.Info("executing new workflow",
		logger.String("signal_id", signal.SignalID),
		logger.String("workflow_id", workflow.WorkflowID))

	// Build initial context
	initialContext := map[string]interface{}{
		"signal_type": signal.SignalType,
		"org_id":      signal.OrgID,
	}

	// Create workflow run
	run := domain.NewWorkflowRun(workflow.WorkflowID, workflow.Version, signal.SignalID, initialContext)

	if err := e.runRepo.Create(ctx, run); err != nil {
		e.logger.Error("failed to create workflow run", logger.Error(err))
		return fmt.Errorf("failed to create run: %w", err)
	}

	e.logger.Info("workflow run created",
		logger.String("run_id", run.RunID))

	// Build run context
	runContext := e.buildRunContext(run, signal, workflow)

	// Evaluate conditions
	if !e.evaluateConditions(ctx, run, workflow, signal) {
		return nil // Conditions failed, workflow stops
	}

	// Execute actions
	return e.executeActions(ctx, run, workflow, signal, runContext, 0)
}

// resumeWorkflow resumes a paused workflow
func (e *workflowExecutor) resumeWorkflow(ctx context.Context, signal *domain.Signal, workflow *domain.Workflow, resumeFrom string, runID string) error {
	e.logger.Info("resuming workflow",
		logger.String("signal_id", signal.SignalID),
		logger.String("workflow_id", workflow.WorkflowID),
		logger.String("resume_from", resumeFrom))

	// Parse resume_from (e.g., "ACTION_2")
	var delayActionIndex int
	if _, err := fmt.Sscanf(resumeFrom, "ACTION_%d", &delayActionIndex); err != nil {
		return fmt.Errorf("invalid resume_from format: %s", resumeFrom)
	}

	run, err := e.runRepo.GetByID(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get workflow run: %w", err)
	}

	// Mark the delay action as COMPLETED before continuing
	run.SetActionCompletedStatus(delayActionIndex)
	if err := e.runRepo.Update(ctx, run); err != nil {
		e.logger.Error("failed to mark delay action as completed", logger.Error(err))
		return fmt.Errorf("failed to update run status: %w", err)
	}

	e.logger.Info("delay action completed, continuing workflow",
		logger.String("run_id", run.RunID),
		logger.Int("delay_action_index", delayActionIndex),
		logger.String("status", string(run.Status)))

	run.CurrentActionIndex = delayActionIndex

	runContext := e.buildRunContext(run, signal, workflow)

	// Resume from next action (delay action is now complete)
	return e.executeActions(ctx, run, workflow, signal, runContext, delayActionIndex+1)
}

// evaluateConditions evaluates all workflow conditions
func (e *workflowExecutor) evaluateConditions(ctx context.Context, run *domain.WorkflowRun, workflow *domain.Workflow, signal *domain.Signal) bool {
	e.logger.Info("evaluating conditions",
		logger.String("run_id", run.RunID),
		logger.Int("condition_count", len(workflow.Conditions)))

	for i, condition := range workflow.Conditions {
		// Update status to CONDITION_X
		run.SetConditionStatus(i)
		if err := e.runRepo.Update(ctx, run); err != nil {
			e.logger.Error("failed to update run status", logger.Error(err))
		}

		passed, err := e.conditionEval.Evaluate(ctx, condition, signal)
		if err != nil {
			e.logger.Error("condition evaluation error",
				logger.Int("condition_index", i),
				logger.Error(err))
			run.MarkFailed()
			e.runRepo.Update(ctx, run)
			return false
		}

		if !passed {
			e.logger.Info("condition failed, workflow stops",
				logger.String("run_id", run.RunID),
				logger.Int("condition_index", i))
			run.MarkCompleted()
			e.runRepo.Update(ctx, run)
			return false
		}
	}

	e.logger.Info("all conditions passed",
		logger.String("run_id", run.RunID))

	return true
}

// executeActions executes workflow actions sequentially
func (e *workflowExecutor) executeActions(ctx context.Context, run *domain.WorkflowRun, workflow *domain.Workflow, signal *domain.Signal, runContext map[string]interface{}, startIndex int) error {
	e.logger.Info("executing actions",
		logger.String("run_id", run.RunID),
		logger.Int("start_index", startIndex),
		logger.Int("total_actions", len(workflow.Actions)))

	for i := startIndex; i < len(workflow.Actions); i++ {
		action := workflow.Actions[i]
		runContext["current_action_index"] = i

		// Execute single action and handle result
		shouldPause, err := e.executeSingleAction(ctx, run, action, signal, runContext, i)
		if err != nil {
			return e.handleActionFailure(ctx, run, i, err)
		}

		// If this was a delay action, pause workflow execution
		if shouldPause {
			e.logger.Info("workflow paused, will resume after delay",
				logger.String("run_id", run.RunID))
			return nil
		}
	}

	// All actions completed successfully
	return e.completeWorkflow(ctx, run)
}

// executeSingleAction executes one action and returns (shouldPause, error)
func (e *workflowExecutor) executeSingleAction(ctx context.Context, run *domain.WorkflowRun, action domain.Action, signal *domain.Signal, runContext map[string]interface{}, actionIndex int) (bool, error) {
	// Prepare action for execution
	actionLog, err := e.prepareAction(ctx, run, action, actionIndex)
	if err != nil {
		return false, err
	}

	// Handle delay actions differently (they pause workflow)
	if action.Type == domain.ActionTypeDelay {
		return e.executeDelayAction(ctx, run, action, signal, runContext, actionIndex, actionLog)
	}

	// Execute regular actions
	return false, e.executeRegularAction(ctx, run, action, signal, runContext, actionIndex, actionLog)
}

// prepareAction updates run status to STARTED and creates action log
func (e *workflowExecutor) prepareAction(ctx context.Context, run *domain.WorkflowRun, action domain.Action, actionIndex int) (*domain.ActionLog, error) {
	// Update run status to ACTION_{i}_STARTED
	run.SetActionStartedStatus(actionIndex)
	if err := e.runRepo.Update(ctx, run); err != nil {
		e.logger.Error("failed to update run status to STARTED", logger.Error(err))
		return nil, fmt.Errorf("failed to update run status: %w", err)
	}

	e.logger.Debug("action started",
		logger.Int("action_index", actionIndex),
		logger.String("status", string(run.Status)))

	// Create and save action log
	actionLog := domain.NewActionLog(run.RunID, actionIndex, string(action.Type))
	actionLog.MarkRunning()
	if err := e.actionLogRepo.Create(ctx, actionLog); err != nil {
		e.logger.Error("failed to create action log", logger.Error(err))
		return nil, fmt.Errorf("failed to create action log: %w", err)
	}

	return actionLog, nil
}

// executeDelayAction handles delay actions that pause workflow execution
func (e *workflowExecutor) executeDelayAction(ctx context.Context, run *domain.WorkflowRun, action domain.Action, signal *domain.Signal, runContext map[string]interface{}, actionIndex int, actionLog *domain.ActionLog) (bool, error) {
	e.logger.Info("delay action encountered, scheduling workflow pause",
		logger.String("run_id", run.RunID),
		logger.Int("action_index", actionIndex))

	// Execute delay (schedules resume for later)
	if err := e.actionExecutor.Execute(ctx, action, signal, runContext, actionIndex); err != nil {
		e.logger.Error("delay action scheduling failed",
			logger.Int("action_index", actionIndex),
			logger.Error(err))
		return false, err
	}

	// Mark run as scheduled (workflow will resume later)
	run.SetActionScheduledStatus(actionIndex)
	if err := e.runRepo.Update(ctx, run); err != nil {
		e.logger.Error("failed to update run status to SCHEDULED", logger.Error(err))
		return false, fmt.Errorf("failed to update run status: %w", err)
	}

	// Mark action log as scheduled
	actionLog.MarkScheduled(time.Now().UnixMilli())
	if err := e.actionLogRepo.Update(ctx, actionLog); err != nil {
		e.logger.Error("failed to update action log", logger.Error(err))
		return false, fmt.Errorf("failed to update action log: %w", err)
	}

	e.logger.Info("workflow paused, scheduled to resume after delay",
		logger.String("run_id", run.RunID),
		logger.String("status", string(run.Status)))

	return true, nil // shouldPause = true
}

// executeRegularAction handles non-delay actions
func (e *workflowExecutor) executeRegularAction(ctx context.Context, run *domain.WorkflowRun, action domain.Action, signal *domain.Signal, runContext map[string]interface{}, actionIndex int, actionLog *domain.ActionLog) error {
	startTime := time.Now()
	err := e.actionExecutor.Execute(ctx, action, signal, runContext, actionIndex)
	duration := time.Since(startTime)

	if err != nil {
		// Mark action as failed
		actionLog.MarkFailed(err.Error(), duration.Milliseconds())
		if updateErr := e.actionLogRepo.Update(ctx, actionLog); updateErr != nil {
			e.logger.Error("failed to update action log", logger.Error(updateErr))
		}
		return err
	}

	// Update run status to ACTION_{i}_COMPLETED
	run.SetActionCompletedStatus(actionIndex)
	if err := e.runRepo.Update(ctx, run); err != nil {
		e.logger.Error("failed to update run status to COMPLETED", logger.Error(err))
		// Continue even if status update fails - action execution was successful
	}

	// Mark action log as completed
	actionLog.MarkCompleted(nil, duration.Milliseconds())
	if err := e.actionLogRepo.Update(ctx, actionLog); err != nil {
		e.logger.Error("failed to update action log", logger.Error(err))
	}

	e.logger.Info("action completed",
		logger.Int("action_index", actionIndex),
		logger.String("status", string(run.Status)),
		logger.Int("duration_ms", int(duration.Milliseconds())))

	return nil
}

// handleActionFailure handles action execution failure
func (e *workflowExecutor) handleActionFailure(ctx context.Context, run *domain.WorkflowRun, actionIndex int, err error) error {
	e.logger.Error("action execution failed",
		logger.Int("action_index", actionIndex),
		logger.Error(err))

	// Mark run as failed
	run.MarkFailed()
	if updateErr := e.runRepo.Update(ctx, run); updateErr != nil {
		e.logger.Error("failed to update run status", logger.Error(updateErr))
	}

	return fmt.Errorf("action %d failed: %w", actionIndex, err)
}

// completeWorkflow marks workflow run as completed
func (e *workflowExecutor) completeWorkflow(ctx context.Context, run *domain.WorkflowRun) error {
	run.MarkCompleted()
	if err := e.runRepo.Update(ctx, run); err != nil {
		e.logger.Error("failed to mark run as completed", logger.Error(err))
		return fmt.Errorf("failed to mark run as completed: %w", err)
	}

	e.logger.Info("workflow execution completed",
		logger.String("run_id", run.RunID))

	return nil
}

// buildRunContext creates execution context for run
func (e *workflowExecutor) buildRunContext(run *domain.WorkflowRun, signal *domain.Signal, workflow *domain.Workflow) map[string]interface{} {
	return map[string]interface{}{
		"run_id":      run.RunID,
		"workflow_id": workflow.WorkflowID,
		"signal_id":   signal.SignalID,
		"signal_type": signal.SignalType,
		"org_id":      signal.OrgID,
	}
}
