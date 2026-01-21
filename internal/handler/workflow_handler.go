package handler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"dahlia/commons/error_handler"
	"dahlia/commons/handler"
	coordinator "dahlia/internal/coordinator/iface"
	"dahlia/internal/domain"
	"dahlia/internal/dto"
	"dahlia/internal/logger"
	repositoryIface "dahlia/internal/repository/iface"
)

type WorkflowHandler struct {
	logger       logger.Logger
	workflowRepo repositoryIface.WorkflowRepository
	zkCoord      coordinator.Coordinator
}

func NewWorkflowHandler(
	log logger.Logger,
	workflowRepo repositoryIface.WorkflowRepository,
	zkCoord coordinator.Coordinator,
) *WorkflowHandler {
	return &WorkflowHandler{
		logger:       log.With(logger.String("component", "workflow_handler")),
		workflowRepo: workflowRepo,
		zkCoord:      zkCoord,
	}
}

func (h *WorkflowHandler) CreateWorkflowService(
	ctx context.Context,
	ioutil *handler.RequestIo[dto.CreateWorkflowRequest],
) (dto.CreateWorkflowResponse, *error_handler.ErrorCollection) {
	req := ioutil.Body

	workflow := &domain.Workflow{
		WorkflowID: req.WorkflowID,
		Version:    req.Version,
		Name:       req.Name,
		SignalType: req.SignalType,
		Conditions: req.Conditions,
		Actions:    req.Actions,
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
	}

	// Save to DynamoDB
	if err := h.workflowRepo.Create(ctx, workflow); err != nil {
		h.logger.Error("failed to save workflow",
			logger.String("workflow_id", workflow.WorkflowID),
			logger.String("error", err.Error()),
		)
		return dto.CreateWorkflowResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeInternalServerError, "failed to save workflow", nil)
	}

	// Create ZK node for workflow version tracking
	workflowPath := fmt.Sprintf("/workflows/%s", workflow.WorkflowID)
	if err := h.zkCoord.CreateNode(workflowPath, []byte(fmt.Sprintf("%d", workflow.Version))); err != nil {
		h.logger.Error("failed to create ZK workflow node",
			logger.String("workflow_id", workflow.WorkflowID),
			logger.String("path", workflowPath),
			logger.Int("version", workflow.Version),
			logger.Error(err),
		)
		// Non-fatal - workflow is saved in DB
	}

	// Update ZK refresh trigger to notify all services
	refreshPath := "/workflows/refresh"
	updatedAt := []byte(fmt.Sprintf("%d", time.Now().Unix()))
	if err := h.zkCoord.UpdateNode(refreshPath, updatedAt); err != nil {
		h.logger.Error("failed to update ZK refresh trigger",
			logger.String("path", refreshPath),
			logger.Error(err),
		)
		// Non-fatal - cache will eventually sync
	}

	return dto.CreateWorkflowResponse{
		WorkflowID: workflow.WorkflowID,
		Version:    workflow.Version,
		Name:       workflow.Name,
		SignalType: workflow.SignalType,
		Conditions: req.Conditions, // Return the request conditions as-is
		Actions:    req.Actions,    // Return the request actions as-is
		CreatedAt:  workflow.CreatedAt,
		UpdatedAt:  workflow.UpdatedAt,
	}, nil
}

func (h *WorkflowHandler) GetWorkflowService(
	ctx context.Context,
	ioutil *handler.RequestIo[dto.GetWorkflowRequest],
) (dto.GetWorkflowResponse, *error_handler.ErrorCollection) {
	workflowID := ioutil.PathParams["id"]

	// Get workflows by signal type and find latest
	// For now, get by ID with version 1 - TODO: implement proper latest version logic
	workflow, err := h.workflowRepo.GetByID(ctx, workflowID, 1)
	if err != nil {
		h.logger.Error("failed to get workflow",
			logger.String("workflow_id", workflowID),
			logger.String("error", err.Error()),
		)
		return dto.GetWorkflowResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeNotFound, "workflow not found", nil)

	}

	return dto.GetWorkflowResponse{
		WorkflowID: workflow.WorkflowID,
		Version:    workflow.Version,
		Name:       workflow.Name,
		SignalType: workflow.SignalType,
		Conditions: workflow.Conditions,
		Actions:    workflow.Actions,
		CreatedAt:  workflow.CreatedAt,
		UpdatedAt:  workflow.UpdatedAt,
	}, nil
}

func (h *WorkflowHandler) ListWorkflowsService(
	ctx context.Context,
	ioutil *handler.RequestIo[dto.ListWorkflowsRequest],
) (dto.ListWorkflowsResponse, *error_handler.ErrorCollection) {
	// Get parameters from query params
	signalType := ioutil.QueryParams["signal_type"]
	nextToken := ioutil.QueryParams["next_token"]

	// Parse limit from query params
	limit := 50 // default
	if limitStr := ioutil.QueryParams["limit"]; limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 100 {
				limit = 100 // max limit
			}
		}
	}

	h.logger.Debug("listing workflows",
		logger.String("signal_type", signalType),
		logger.Int("limit", limit))

	var result *repositoryIface.WorkflowPaginationResult
	var err error

	// Query by signal_type if provided, otherwise list all
	if signalType != "" {
		result, err = h.workflowRepo.ListBySignalType(ctx, signalType, limit, nextToken)
	} else {
		result, err = h.workflowRepo.List(ctx, limit, nextToken)
	}

	if err != nil {
		h.logger.Error("failed to list workflows",
			logger.String("signal_type", signalType),
			logger.String("error", err.Error()),
		)
		return dto.ListWorkflowsResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeInternalServerError, "failed to list workflows", nil)
	}

	// Convert domain workflows to response format
	workflowResponses := make([]dto.WorkflowResponse, len(result.Workflows))
	for i, workflow := range result.Workflows {
		workflowResponses[i] = dto.WorkflowResponse{
			WorkflowID: workflow.WorkflowID,
			Version:    workflow.Version,
			Name:       workflow.Name,
			SignalType: workflow.SignalType,
			Conditions: workflow.Conditions,
			Actions:    workflow.Actions,
			CreatedAt:  workflow.CreatedAt,
			UpdatedAt:  workflow.UpdatedAt,
		}
	}

	return dto.ListWorkflowsResponse{
		Workflows: workflowResponses,
		PaginationResponse: dto.PaginationResponse{
			Count:     len(workflowResponses),
			NextToken: result.NextToken,
		},
	}, nil
}
