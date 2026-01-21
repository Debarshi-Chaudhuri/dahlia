package handler

import (
	"context"
	"strconv"

	"dahlia/commons/error_handler"
	"dahlia/commons/handler"
	"dahlia/internal/dto"
	"dahlia/internal/logger"
	repositoryIface "dahlia/internal/repository/iface"
)

type RunHandler struct {
	logger  logger.Logger
	runRepo repositoryIface.RunRepository
}

func NewRunHandler(
	log logger.Logger,
	runRepo repositoryIface.RunRepository,
) *RunHandler {
	return &RunHandler{
		logger:  log.With(logger.String("component", "run_handler")),
		runRepo: runRepo,
	}
}

func (h *RunHandler) GetRunService(
	ctx context.Context,
	ioutil *handler.RequestIo[dto.GetRunRequest],
) (dto.GetRunResponse, *error_handler.ErrorCollection) {
	// Get run ID from path params
	runID := ioutil.PathParams["id"]
	if runID == "" {
		return dto.GetRunResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeValidationError, "run_id is required", nil)
	}

	run, err := h.runRepo.GetByID(ctx, runID)
	if err != nil {
		h.logger.Error("failed to get run",
			logger.String("run_id", runID),
			logger.String("error", err.Error()),
		)
		return dto.GetRunResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeNotFound, "run not found", nil)
	}

	return dto.GetRunResponse{
		RunID:              run.RunID,
		WorkflowID:         run.WorkflowID,
		WorkflowVersion:    run.WorkflowVersion,
		SignalID:           run.SignalID,
		Status:             string(run.Status),
		CurrentState:       run.CurrentState,
		CurrentEvalIndex:   run.CurrentEvalIndex,
		CurrentActionIndex: run.CurrentActionIndex,
		Context:            run.Context,
		TriggeredAt:        run.TriggeredAt,
		CompletedAt:        run.CompletedAt,
		CreatedAt:          run.CreatedAt,
		UpdatedAt:          run.UpdatedAt,
	}, nil
}

func (h *RunHandler) ListRunsService(
	ctx context.Context,
	ioutil *handler.RequestIo[dto.ListRunsRequest],
) (dto.ListRunsResponse, *error_handler.ErrorCollection) {
	// Get parameters from query params
	workflowID := ioutil.QueryParams["workflow_id"]
	status := ioutil.QueryParams["status"]
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

	result, err := h.runRepo.GetRunningWorkflows(ctx, limit, nextToken)

	if err != nil {
		h.logger.Error("failed to list runs",
			logger.String("workflow_id", workflowID),
			logger.String("status", status),
			logger.String("error", err.Error()),
		)
		return dto.ListRunsResponse{}, error_handler.NewErrorCollection().
			AddError(error_handler.CodeInternalServerError, "failed to list runs", nil)
	}

	// Convert domain runs to response format
	runResponses := make([]dto.RunResponse, len(result.Runs))
	for i, run := range result.Runs {
		runResponses[i] = dto.RunResponse{
			RunID:              run.RunID,
			WorkflowID:         run.WorkflowID,
			WorkflowVersion:    run.WorkflowVersion,
			SignalID:           run.SignalID,
			Status:             string(run.Status),
			CurrentState:       run.CurrentState,
			CurrentEvalIndex:   run.CurrentEvalIndex,
			CurrentActionIndex: run.CurrentActionIndex,
			Context:            run.Context,
			TriggeredAt:        run.TriggeredAt,
			CompletedAt:        run.CompletedAt,
			CreatedAt:          run.CreatedAt,
			UpdatedAt:          run.UpdatedAt,
		}
	}

	return dto.ListRunsResponse{
		Runs: runResponses,
		PaginationResponse: dto.PaginationResponse{
			Count:     len(runResponses),
			NextToken: result.NextToken,
		},
	}, nil
}
