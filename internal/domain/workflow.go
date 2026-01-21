package domain

import (
	"time"

	"github.com/google/uuid"
)

// ConditionType represents the type of condition
type ConditionType string

const (
	ConditionTypeAbsence ConditionType = "absence"
	ConditionTypeNumeric ConditionType = "numeric"
)

// Condition represents a workflow condition
type Condition struct {
	Type     ConditionType `json:"type" dynamodbav:"type"`
	Field    string        `json:"field,omitempty" dynamodbav:"field,omitempty"`
	Operator string        `json:"operator,omitempty" dynamodbav:"operator,omitempty"` // ">", "<", "=", "!=", ">=", "<="
	Value    interface{}   `json:"value,omitempty" dynamodbav:"value,omitempty"`
	Duration string        `json:"duration,omitempty" dynamodbav:"duration,omitempty"` // For absence: "5m"
}

// ActionType represents the type of action
type ActionType string

const (
	ActionTypeSlack   ActionType = "slack"
	ActionTypeWebhook ActionType = "webhook"
	ActionTypeDelay   ActionType = "delay"
)

// Action represents a workflow action
type Action struct {
	Type     ActionType `json:"type" dynamodbav:"type"`
	Target   string     `json:"target,omitempty" dynamodbav:"target,omitempty"`     // Slack channel or webhook URL
	Message  string     `json:"message,omitempty" dynamodbav:"message,omitempty"`   // Template with {{var}}
	Duration string     `json:"duration,omitempty" dynamodbav:"duration,omitempty"` // For delay: "15m"
}

// Workflow represents a workflow definition
type Workflow struct {
	WorkflowID string      `json:"workflow_id" dynamodbav:"workflow_id"`
	Version    int         `json:"version" dynamodbav:"version"`
	Name       string      `json:"name" dynamodbav:"name"`
	SignalType string      `json:"signal_type" dynamodbav:"signal_type"`
	Conditions []Condition `json:"conditions" dynamodbav:"conditions"`
	Actions    []Action    `json:"actions" dynamodbav:"actions"`
	CreatedAt  int64       `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt  int64       `json:"updated_at" dynamodbav:"updated_at"`
}

// NewWorkflow creates a new workflow
func NewWorkflow(name, signalType string, conditions []Condition, actions []Action) *Workflow {
	now := time.Now().UnixMilli()

	if conditions == nil {
		conditions = []Condition{}
	}
	if actions == nil {
		actions = []Action{}
	}

	return &Workflow{
		WorkflowID: uuid.New().String(),
		Version:    1,
		Name:       name,
		SignalType: signalType,
		Conditions: conditions,
		Actions:    actions,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
