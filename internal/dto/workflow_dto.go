package dto

import "dahlia/internal/domain"

// CreateWorkflowRequest represents request to create a workflow
type CreateWorkflowRequest struct {
	WorkflowID string             `json:"workflow_id" binding:"required"`
	Version    int                `json:"version" binding:"required"`
	Name       string             `json:"name" binding:"required"`
	SignalType string             `json:"signal_type" binding:"required"`
	Conditions []domain.Condition `json:"conditions" binding:"required"`
	Actions    []domain.Action    `json:"actions" binding:"required"`
}

// CreateWorkflowResponse represents response after creating a workflow
type CreateWorkflowResponse struct {
	WorkflowID string             `json:"workflow_id"`
	Version    int                `json:"version"`
	Name       string             `json:"name"`
	SignalType string             `json:"signal_type"`
	Conditions []domain.Condition `json:"conditions"`
	Actions    []domain.Action    `json:"actions"`
	CreatedAt  int64              `json:"created_at"`
	UpdatedAt  int64              `json:"updated_at"`
}

// GetWorkflowRequest represents request to get a workflow
type GetWorkflowRequest struct {
	// No body fields - workflow_id comes from path params
}

// GetWorkflowResponse represents response for getting a workflow
type GetWorkflowResponse struct {
	WorkflowID string             `json:"workflow_id"`
	Version    int                `json:"version"`
	Name       string             `json:"name"`
	SignalType string             `json:"signal_type"`
	Conditions []domain.Condition `json:"conditions"`
	Actions    []domain.Action    `json:"actions"`
	CreatedAt  int64              `json:"created_at"`
	UpdatedAt  int64              `json:"updated_at"`
}
