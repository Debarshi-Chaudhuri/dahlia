package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"dahlia/internal/domain"
	"dahlia/internal/logger"
	"dahlia/internal/slack"
)

// ErrWorkflowPaused indicates the workflow should be paused (used for delay actions)
var ErrWorkflowPaused = fmt.Errorf("workflow paused for delay")

// ActionExecutor executes workflow actions
type ActionExecutor interface {
	Execute(ctx context.Context, action domain.Action, signal *domain.Signal, runContext map[string]interface{}, actionIndex int) error
}

type actionExecutor struct {
	slackClient slack.Client
	scheduler   IScheduler
	logger      logger.Logger
}

// NewActionExecutor creates a new action executor
func NewActionExecutor(slackClient slack.Client, scheduler IScheduler, log logger.Logger) ActionExecutor {
	return &actionExecutor{
		slackClient: slackClient,
		scheduler:   scheduler,
		logger:      log.With(logger.String("component", "action_executor")),
	}
}

// Execute executes a single action
func (e *actionExecutor) Execute(ctx context.Context, action domain.Action, signal *domain.Signal, runContext map[string]interface{}, actionIndex int) error {
	switch action.Type {
	case domain.ActionTypeSlack:
		return e.executeSlack(ctx, action, signal, runContext)
	case domain.ActionTypeWebhook:
		return e.executeWebhook(ctx, action, signal, runContext)
	case domain.ActionTypeDelay:
		return e.executeDelay(ctx, action, runContext)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// executeSlack sends a Slack message
func (e *actionExecutor) executeSlack(ctx context.Context, action domain.Action, signal *domain.Signal, runContext map[string]interface{}) error {
	startTime := time.Now()

	// Replace template variables in message
	message := e.replaceTemplateVariables(action.Message, signal, runContext)

	err := e.slackClient.SendMessage(ctx, action.Target, message)
	if err != nil {
		e.logger.Error("failed to send slack message",
			logger.String("channel", action.Target),
			logger.Error(err))
		return fmt.Errorf("slack action failed: %w", err)
	}

	duration := time.Since(startTime)
	e.logger.Info("slack message sent",
		logger.String("channel", action.Target),
		logger.Int("duration_ms", int(duration.Milliseconds())))

	return nil
}

// executeWebhook sends a webhook request
func (e *actionExecutor) executeWebhook(ctx context.Context, action domain.Action, signal *domain.Signal, runContext map[string]interface{}) error {

	// Replace template variables in message
	message := e.replaceTemplateVariables(action.Message, signal, runContext)

	// Mock webhook - just log
	e.logger.Info("MOCK: Webhook called",
		logger.String("url", action.Target),
		logger.String("payload", message))

	return nil
}

// executeDelay schedules a delay (handled by scheduler)
func (e *actionExecutor) executeDelay(ctx context.Context, action domain.Action, runContext map[string]interface{}) error {
	e.logger.Info("executing delay action",
		logger.String("duration", action.Duration))

	// Parse duration
	duration, err := time.ParseDuration(action.Duration)
	if err != nil {
		return fmt.Errorf("invalid delay duration: %w", err)
	}

	delaySeconds := int(duration.Seconds())

	// Get run_id and workflow_id from context
	runID, ok := runContext["run_id"].(string)
	if !ok {
		return fmt.Errorf("run_id not found in context")
	}

	workflowID, ok := runContext["workflow_id"].(string)
	if !ok {
		return fmt.Errorf("workflow_id not found in context")
	}

	signalID, ok := runContext["signal_id"].(string)
	if !ok {
		return fmt.Errorf("signal_id not found in context")
	}

	currentActionIndex, ok := runContext["current_action_index"].(int)
	if !ok {
		return fmt.Errorf("current_action_index not found in context")
	}

	// Schedule job
	jobDetails := map[string]interface{}{
		"signal_id":   signalID,
		"workflow_id": workflowID,
		"run_id":      runID,
		"resume_from": fmt.Sprintf("ACTION_%d", currentActionIndex+1),
	}

	if delaySeconds < 60 {
		// Handle short delays inside service

		time.Sleep(time.Duration(delaySeconds) * time.Second)

		return nil
	}

	// Schedule with scheduler service for long delays
	job, err := e.scheduler.ScheduleWithDelay(
		ctx,
		fmt.Sprintf("workflow-resume-%s", runID),
		"executor-queue",
		delaySeconds,
		jobDetails,
	)

	if err != nil {
		e.logger.Error("failed to schedule delay",
			logger.String("run_id", runID),
			logger.Error(err))
		return fmt.Errorf("failed to schedule delay: %w", err)
	}

	e.logger.Info("delay action scheduled",
		logger.String("job_id", job.JobID),
		logger.Int("delay_seconds", delaySeconds))

	return nil
}

// replaceTemplateVariables replaces all {{variable}} placeholders with actual values
func (e *actionExecutor) replaceTemplateVariables(template string, signal *domain.Signal, runContext map[string]interface{}) string {
	// Build variable map from signal and context
	variables := e.buildVariableMap(signal, runContext)

	// Find all {{variable}} patterns
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)

	result := re.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable name (remove {{ and }})
		varName := strings.TrimSpace(match[2 : len(match)-2])

		// Look up value in variables map
		if value, ok := variables[varName]; ok {
			return fmt.Sprintf("%v", value)
		}

		// Try nested path (e.g., "value.status")
		if nestedValue := e.getNestedValue(varName, signal); nestedValue != nil {
			return fmt.Sprintf("%v", nestedValue)
		}

		// Variable not found, return as-is
		e.logger.Warn("template variable not found",
			logger.String("variable", varName))
		return match
	})

	return result
}

// buildVariableMap creates a map of variables from signal and context
func (e *actionExecutor) buildVariableMap(signal *domain.Signal, runContext map[string]interface{}) map[string]interface{} {
	variables := make(map[string]interface{})

	// Add signal properties
	variables["signal_id"] = signal.SignalID
	variables["signal_type"] = signal.SignalType
	variables["org_id"] = signal.OrgID
	variables["timestamp"] = signal.Timestamp

	// Add all metadata fields
	for k, v := range signal.Metadata {
		variables[k] = v
		variables["metadata."+k] = v
	}

	// Add all value fields
	for k, v := range signal.Value {
		variables[k] = v
		variables["value."+k] = v
	}

	// Add run context
	for k, v := range runContext {
		variables[k] = v
	}

	return variables
}

// getNestedValue retrieves nested values like "value.status" or "metadata.count"
func (e *actionExecutor) getNestedValue(path string, signal *domain.Signal) interface{} {
	parts := strings.Split(path, ".")
	if len(parts) != 2 {
		return nil
	}

	switch parts[0] {
	case "value":
		if val, ok := signal.Value[parts[1]]; ok {
			return val
		}
	case "metadata":
		if val, ok := signal.Metadata[parts[1]]; ok {
			return val
		}
	}

	return nil
}
