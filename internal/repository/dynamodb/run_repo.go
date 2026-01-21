package dynamodb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"dahlia/internal/domain"
	"dahlia/internal/logger"

	repositoryIface "dahlia/internal/repository/iface"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type runRepository struct {
	client    *dynamodb.Client
	tableName string
	logger    logger.Logger
}

// NewRunRepository creates a new DynamoDB run repository
func NewRunRepository(client *dynamodb.Client, log logger.Logger) repositoryIface.RunRepository {
	return &runRepository{
		client:    client,
		tableName: "workflow_runs",
		logger:    log.With(logger.String("component", "run_repository")),
	}
}

func (r *runRepository) Create(ctx context.Context, run *domain.WorkflowRun) error {
	r.logger.Debug("creating workflow run",
		logger.String("run_id", run.RunID),
		logger.String("workflow_id", run.WorkflowID))

	item, err := attributevalue.MarshalMap(run)
	if err != nil {
		r.logger.Error("failed to marshal run", logger.Error(err))
		return fmt.Errorf("failed to marshal run: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	if err != nil {
		r.logger.Error("failed to create run", logger.Error(err))
		return fmt.Errorf("failed to create run: %w", err)
	}

	r.logger.Info("workflow run created",
		logger.String("run_id", run.RunID),
		logger.String("status", string(run.Status)))

	return nil
}

func (r *runRepository) Update(ctx context.Context, run *domain.WorkflowRun) error {
	r.logger.Debug("updating workflow run",
		logger.String("run_id", run.RunID),
		logger.String("status", string(run.Status)),
		logger.Int("updated_at", int(run.UpdatedAt)))

	// Store the old updated_at for conditional check
	oldUpdatedAt := run.UpdatedAt

	// Update the updated_at to current time for the new version
	// Note: The caller should have already updated this, but we ensure it here
	if run.UpdatedAt == oldUpdatedAt {
		r.logger.Warn("updated_at was not changed before update call, this might indicate a bug",
			logger.String("run_id", run.RunID))
	}

	item, err := attributevalue.MarshalMap(run)
	if err != nil {
		return fmt.Errorf("failed to marshal run: %w", err)
	}

	// Use conditional write to ensure the record hasn't been modified
	// This implements optimistic locking
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
		// Condition: updated_at must equal the old value (or not exist for new records)
		ConditionExpression: aws.String("updated_at = :old_updated_at OR attribute_not_exists(updated_at)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":old_updated_at": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", oldUpdatedAt)},
		},
	})

	if err != nil {
		// Check if it's a conditional check failure
		var conditionalCheckErr *types.ConditionalCheckFailedException
		if ok := errors.As(err, &conditionalCheckErr); ok {
			r.logger.Warn("optimistic lock failed - run was modified by another process",
				logger.String("run_id", run.RunID),
				logger.Int("expected_updated_at", int(oldUpdatedAt)))
			return fmt.Errorf("%w: run_id=%s", ErrOptimisticLockFailed, run.RunID)
		}

		r.logger.Error("failed to update run", logger.Error(err))
		return fmt.Errorf("failed to update run: %w", err)
	}

	r.logger.Debug("workflow run updated successfully",
		logger.String("run_id", run.RunID),
		logger.Int("new_updated_at", int(run.UpdatedAt)))

	return nil
}

func (r *runRepository) GetByID(ctx context.Context, runID string) (*domain.WorkflowRun, error) {
	r.logger.Debug("getting run by ID",
		logger.String("run_id", runID))

	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("run_id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: runID},
		},
		Limit: aws.Int32(1),
	})

	if err != nil {
		r.logger.Error("failed to get run", logger.Error(err))
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	var run domain.WorkflowRun
	if err := attributevalue.UnmarshalMap(result.Items[0], &run); err != nil {
		return nil, fmt.Errorf("failed to unmarshal run: %w", err)
	}

	return &run, nil
}

func (r *runRepository) GetByWorkflowID(ctx context.Context, workflowID string, limit int, nextToken string) (*repositoryIface.PaginationResult, error) {
	r.logger.Debug("getting runs by workflow ID",
		logger.String("workflow_id", workflowID),
		logger.Int("limit", limit))

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("workflow_id_index"),
		KeyConditionExpression: aws.String("workflow_id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: workflowID},
		},
		ScanIndexForward: aws.Bool(false), // Descending order
		Limit:            aws.Int32(int32(limit)),
	}

	// Add pagination token if provided
	if nextToken != "" {
		exclusiveStartKey, err := decodeNextToken(nextToken)
		if err != nil {
			r.logger.Warn("failed to decode next token", logger.Error(err))
			return nil, fmt.Errorf("invalid next token: %w", err)
		}
		queryInput.ExclusiveStartKey = exclusiveStartKey
	}

	result, err := r.client.Query(ctx, queryInput)
	if err != nil {
		r.logger.Error("failed to query runs", logger.Error(err))
		return nil, fmt.Errorf("failed to query runs: %w", err)
	}

	runs := make([]*domain.WorkflowRun, 0, len(result.Items))
	for _, item := range result.Items {
		var run domain.WorkflowRun
		if err := attributevalue.UnmarshalMap(item, &run); err != nil {
			r.logger.Warn("failed to unmarshal run", logger.Error(err))
			continue
		}
		runs = append(runs, &run)
	}

	// Encode next token if there are more results
	var encodedNextToken string
	if result.LastEvaluatedKey != nil {
		encodedNextToken, err = encodeNextToken(result.LastEvaluatedKey)
		if err != nil {
			r.logger.Warn("failed to encode next token", logger.Error(err))
		}
	}

	r.logger.Debug("runs retrieved",
		logger.String("workflow_id", workflowID),
		logger.Int("count", len(runs)),
		logger.String("has_more", fmt.Sprintf("%t", encodedNextToken != "")))

	return &repositoryIface.PaginationResult{
		Runs:      runs,
		NextToken: encodedNextToken,
	}, nil
}

func (r *runRepository) GetByStatus(ctx context.Context, status domain.RunStatus, limit int, nextToken string) (*repositoryIface.PaginationResult, error) {
	r.logger.Debug("getting runs by status",
		logger.String("status", string(status)),
		logger.Int("limit", limit))

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("status_index"),
		KeyConditionExpression: aws.String("#status = :status"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: string(status)},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(int32(limit)),
	}

	// Add pagination token if provided
	if nextToken != "" {
		exclusiveStartKey, err := decodeNextToken(nextToken)
		if err != nil {
			r.logger.Warn("failed to decode next token", logger.Error(err))
			return nil, fmt.Errorf("invalid next token: %w", err)
		}
		queryInput.ExclusiveStartKey = exclusiveStartKey
	}

	result, err := r.client.Query(ctx, queryInput)
	if err != nil {
		r.logger.Error("failed to query runs", logger.Error(err))
		return nil, fmt.Errorf("failed to query runs: %w", err)
	}

	runs := make([]*domain.WorkflowRun, 0, len(result.Items))
	for _, item := range result.Items {
		var run domain.WorkflowRun
		if err := attributevalue.UnmarshalMap(item, &run); err != nil {
			r.logger.Warn("failed to unmarshal run", logger.Error(err))
			continue
		}
		runs = append(runs, &run)
	}

	// Encode next token if there are more results
	var encodedNextToken string
	if result.LastEvaluatedKey != nil {
		encodedNextToken, err = encodeNextToken(result.LastEvaluatedKey)
		if err != nil {
			r.logger.Warn("failed to encode next token", logger.Error(err))
		}
	}

	r.logger.Debug("runs retrieved",
		logger.String("status", string(status)),
		logger.Int("count", len(runs)),
		logger.String("has_more", fmt.Sprintf("%t", encodedNextToken != "")))

	return &repositoryIface.PaginationResult{
		Runs:      runs,
		NextToken: encodedNextToken,
	}, nil
}

// GetRunningWorkflows returns all runs that are not completed or failed (currently running)
func (r *runRepository) GetRunningWorkflows(ctx context.Context, limit int, nextToken string) (*repositoryIface.PaginationResult, error) {
	r.logger.Debug("getting running workflows",
		logger.Int("limit", limit))

	// Scan with filter expression to find runs that are not COMPLETED or FAILED
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(r.tableName),
		FilterExpression: aws.String("#status <> :completed AND #status <> :failed"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":completed": &types.AttributeValueMemberS{Value: string(domain.RunStatusCompleted)},
			":failed":    &types.AttributeValueMemberS{Value: string(domain.RunStatusFailed)},
		},
		Limit: aws.Int32(int32(limit)),
	}

	// Add pagination token if provided
	if nextToken != "" {
		exclusiveStartKey, err := decodeNextToken(nextToken)
		if err != nil {
			r.logger.Warn("failed to decode next token", logger.Error(err))
			return nil, fmt.Errorf("invalid next token: %w", err)
		}
		scanInput.ExclusiveStartKey = exclusiveStartKey
	}

	result, err := r.client.Scan(ctx, scanInput)
	if err != nil {
		r.logger.Error("failed to scan running workflows", logger.Error(err))
		return nil, fmt.Errorf("failed to scan running workflows: %w", err)
	}

	runs := make([]*domain.WorkflowRun, 0, len(result.Items))
	for _, item := range result.Items {
		var run domain.WorkflowRun
		if err := attributevalue.UnmarshalMap(item, &run); err != nil {
			r.logger.Warn("failed to unmarshal run", logger.Error(err))
			continue
		}
		runs = append(runs, &run)
	}

	// Encode next token if there are more results
	var encodedNextToken string
	if result.LastEvaluatedKey != nil {
		encodedNextToken, err = encodeNextToken(result.LastEvaluatedKey)
		if err != nil {
			r.logger.Warn("failed to encode next token", logger.Error(err))
		}
	}

	r.logger.Debug("running workflows retrieved",
		logger.Int("count", len(runs)),
		logger.String("has_more", fmt.Sprintf("%t", encodedNextToken != "")))

	return &repositoryIface.PaginationResult{
		Runs:      runs,
		NextToken: encodedNextToken,
	}, nil
}

// Helper functions for pagination token encoding/decoding
func encodeNextToken(lastEvaluatedKey map[string]types.AttributeValue) (string, error) {
	if lastEvaluatedKey == nil {
		return "", nil
	}

	jsonData, err := json.Marshal(lastEvaluatedKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal last evaluated key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(jsonData), nil
}

func decodeNextToken(nextToken string) (map[string]types.AttributeValue, error) {
	if nextToken == "" {
		return nil, nil
	}

	jsonData, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode next token: %w", err)
	}

	var lastEvaluatedKey map[string]types.AttributeValue
	if err := json.Unmarshal(jsonData, &lastEvaluatedKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal next token: %w", err)
	}

	return lastEvaluatedKey, nil
}
