package service

import (
	"context"
	"fmt"
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
func (e *conditionEvaluator) evaluateAbsence(ctx context.Context, condition domain.Condition, signal *domain.Signal) (bool, error) {
	// Parse duration
	duration, err := time.ParseDuration(condition.Duration)
	if err != nil {
		return false, fmt.Errorf("invalid duration: %w", err)
	}

	// Calculate expected time
	signalTime, err := time.Parse(time.RFC3339, signal.Timestamp)
	if err != nil {
		return false, fmt.Errorf("invalid signal timestamp: %w", err)
	}

	expectedTime := signalTime.Add(-duration)

	// Query for last signal before current signal
	lastSignal, err := e.signalRepo.GetLastSignal(ctx, signal.SignalType, signal.OrgID, signalTime)
	if err != nil {
		return false, fmt.Errorf("failed to get last signal: %w", err)
	}

	// If no signal found, absence condition is met
	if lastSignal == nil {
		return true, nil
	}

	// Parse last signal time
	lastSignalTime, err := time.Parse(time.RFC3339, lastSignal.Timestamp)
	if err != nil {
		return false, fmt.Errorf("invalid last signal timestamp: %w", err)
	}

	// Check if last signal is before expected time
	if lastSignalTime.Before(expectedTime) {
		return true, nil
	}

	return false, nil
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
