package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"dahlia/internal/domain"
	"dahlia/internal/logger"
	repository "dahlia/internal/repository/iface"

	"github.com/expr-lang/expr"
)

// ConditionEvaluator evaluates workflow conditions
type ConditionEvaluator interface {
	EvaluateAll(ctx context.Context, conditions []domain.Condition, signal *domain.Signal) (bool, error)
	Evaluate(ctx context.Context, condition domain.Condition, signal *domain.Signal) (bool, error)
}

type conditionEvaluator struct {
	signalRepo repository.SignalRepository
	logger     logger.Logger
}

// NewConditionEvaluator creates a new condition evaluator
func NewConditionEvaluator(signalRepo repository.SignalRepository, log logger.Logger) ConditionEvaluator {
	return &conditionEvaluator{
		signalRepo: signalRepo,
		logger:     log.With(logger.String("component", "condition_evaluator")),
	}
}

// EvaluateAll evaluates all conditions (AND logic)
func (e *conditionEvaluator) EvaluateAll(ctx context.Context, conditions []domain.Condition, signal *domain.Signal) (bool, error) {
	for i, condition := range conditions {
		passed, err := e.Evaluate(ctx, condition, signal)
		if err != nil {
			e.logger.Error("condition evaluation failed",
				logger.Int("condition_index", i),
				logger.Error(err))
			return false, err
		}

		if !passed {
			return false, nil
		}

		e.logger.Debug("condition passed",
			logger.Int("condition_index", i),
			logger.String("type", string(condition.Type)))
	}

	return true, nil
}

// Evaluate evaluates a single condition
func (e *conditionEvaluator) Evaluate(ctx context.Context, condition domain.Condition, signal *domain.Signal) (bool, error) {
	switch condition.Type {
	case domain.ConditionTypeAbsence:
		return e.evaluateAbsence(ctx, condition, signal)
	case domain.ConditionTypeNumeric:
		return e.evaluateNumeric(ctx, condition, signal)
	default:
		return false, fmt.Errorf("unknown condition type: %s", condition.Type)
	}
}

// evaluateAbsence checks if no signal arrived within duration
// For absence conditions, this is called AFTER the waiting period has elapsed
func (e *conditionEvaluator) evaluateAbsence(ctx context.Context, condition domain.Condition, signal *domain.Signal) (bool, error) {
	// Parse duration with explicit support for common time units
	duration, err := e.parseDuration(condition.Duration)
	if err != nil {
		return false, fmt.Errorf("invalid duration '%s': %w", condition.Duration, err)
	}

	e.logger.Debug("evaluating absence condition",
		logger.String("duration", condition.Duration),
		logger.String("parsed_duration", duration.String()),
		logger.String("signal_type", signal.SignalType),
		logger.String("org_id", signal.OrgID))

	// Parse the original signal time (when the absence timer started)
	signalTime, err := time.Parse(time.RFC3339, signal.Timestamp)
	if err != nil {
		return false, fmt.Errorf("invalid signal timestamp: %w", err)
	}

	// Calculate the window start time (when we started waiting for absence)
	windowStartTime := signalTime

	// Calculate current time (when this evaluation is happening - should be ~duration later)
	currentTime := time.Now()
	expectedEvalTime := windowStartTime.Add(duration)

	e.logger.Debug("absence condition timing",
		logger.String("window_start", windowStartTime.String()),
		logger.String("expected_eval_time", expectedEvalTime.String()),
		logger.String("current_time", currentTime.String()))

	// Query for any signals that arrived AFTER the original signal within the duration window
	signals, err := e.signalRepo.GetSignalsSince(ctx, signal.SignalType, signal.OrgID, windowStartTime, currentTime)
	if err != nil {
		return false, fmt.Errorf("failed to check for signals since window start: %w", err)
	}

	// Filter out the original signal itself (we only care about NEW signals)
	newSignalCount := 0
	for _, s := range signals {
		if s.SignalID != signal.SignalID {
			newSignalCount++
		}
	}

	e.logger.Debug("absence condition evaluation result",
		logger.Int("new_signals_found", newSignalCount),
		logger.String("condition_passed", strconv.FormatBool(newSignalCount == 0)))

	// Condition passes if NO new signals arrived during the absence window
	return newSignalCount == 0, nil
}

// parseDuration handles common duration formats that time.ParseDuration might not handle
func (e *conditionEvaluator) parseDuration(durationStr string) (time.Duration, error) {
	// First try the standard Go duration parser
	duration, err := time.ParseDuration(durationStr)
	if err == nil {
		return duration, nil
	}

	// Handle common formats that Go doesn't support by default
	switch durationStr {
	case "1m", "1min", "1minute":
		return time.Minute, nil
	case "2m", "2min", "2minutes":
		return 2 * time.Minute, nil
	case "5m", "5min", "5minutes":
		return 5 * time.Minute, nil
	case "10m", "10min", "10minutes":
		return 10 * time.Minute, nil
	case "15m", "15min", "15minutes":
		return 15 * time.Minute, nil
	case "30m", "30min", "30minutes":
		return 30 * time.Minute, nil
	case "40m", "40min", "40minutes":
		return 40 * time.Minute, nil
	case "45m", "45min", "45minutes":
		return 45 * time.Minute, nil
	case "1h", "1hr", "1hour":
		return time.Hour, nil
	case "2h", "2hr", "2hours":
		return 2 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported duration format: %s (try formats like '5m', '10m', '1h')", durationStr)
	}
}

// evaluateNumeric evaluates numeric expressions using expr-lang
func (e *conditionEvaluator) evaluateNumeric(_ context.Context, condition domain.Condition, signal *domain.Signal) (bool, error) {
	// Build expression: field operator value
	// Example: "value.status > 10" or "metadata.count == 5"
	expression := fmt.Sprintf("%s %s %v", condition.Field, condition.Operator, condition.Value)

	// Build environment with signal data
	env := e.buildEnvironment(signal)

	// Compile and evaluate expression
	program, err := expr.Compile(expression, expr.Env(env))
	if err != nil {
		e.logger.Error("failed to compile expression",
			logger.String("expression", expression),
			logger.Error(err))
		return false, fmt.Errorf("invalid expression: %w", err)
	}

	result, err := expr.Run(program, env)
	if err != nil {
		e.logger.Error("failed to evaluate expression",
			logger.String("expression", expression),
			logger.Error(err))
		return false, fmt.Errorf("expression evaluation failed: %w", err)
	}

	// Convert result to bool
	boolResult, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expression did not return boolean: %T", result)
	}

	return boolResult, nil
}

// buildEnvironment creates expression environment from signal
func (e *conditionEvaluator) buildEnvironment(signal *domain.Signal) map[string]interface{} {
	env := map[string]interface{}{
		"signal_type": signal.SignalType,
		"org_id":      signal.OrgID,
		"timestamp":   signal.Timestamp,
		"value":       signal.Value,
		"metadata":    signal.Metadata,
	}

	// Flatten value map for easier access
	for k, v := range signal.Value {
		env[k] = v
	}

	return env
}
