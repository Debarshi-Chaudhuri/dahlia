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

	return nil
}

func (r *runRepository) Update(ctx context.Context, run *domain.WorkflowRun, expectedUpdatedAt int64) error {
	item, err := attributevalue.MarshalMap(run)
	if err != nil {
		return fmt.Errorf("failed to marshal run: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("updated_at = :expected_updated_at OR attribute_not_exists(updated_at)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":expected_updated_at": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expectedUpdatedAt)},
		},
	})

	if err != nil {
		// Check if it's a conditional check failure
		var conditionalCheckErr *types.ConditionalCheckFailedException
		if ok := errors.As(err, &conditionalCheckErr); ok {
			r.logger.Warn("optimistic lock failed - run was modified by another process",
				logger.String("run_id", run.RunID),
				logger.Int("expected_updated_at", int(expectedUpdatedAt)),
				logger.Int("new_updated_at", int(run.UpdatedAt)))
			return fmt.Errorf("%w: run_id=%s", ErrOptimisticLockFailed, run.RunID)
		}

		r.logger.Error("failed to update run", logger.Error(err))
		return fmt.Errorf("failed to update run: %w", err)
	}

	return nil
}

func (r *runRepository) GetByID(ctx context.Context, runID string) (*domain.WorkflowRun, error) {
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

	return &repositoryIface.PaginationResult{
		Runs:      runs,
		NextToken: encodedNextToken,
	}, nil
}

func (r *runRepository) GetByStatus(ctx context.Context, status domain.RunStatus, limit int, nextToken string) (*repositoryIface.PaginationResult, error) {
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

	return &repositoryIface.PaginationResult{
		Runs:      runs,
		NextToken: encodedNextToken,
	}, nil
}

// GetRunningWorkflows returns all runs that are not completed or failed (currently running)
// Uses GSI on status + created_at to query and order by created_at DESC
func (r *runRepository) GetRunningWorkflows(ctx context.Context, limit int, nextToken string) (*repositoryIface.PaginationResult, error) {
	r.logger.Debug("getting running workflows",
		logger.Int("limit", limit))

	// Query GSI for TRIGGERED status
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("status_index"),
		KeyConditionExpression: aws.String("#status = :triggered"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":triggered": &types.AttributeValueMemberS{Value: string(domain.RunStatusTriggered)},
		},
		ScanIndexForward: aws.Bool(false), // Descending order by created_at (sort key)
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
		r.logger.Error("failed to query running workflows", logger.Error(err))
		return nil, fmt.Errorf("failed to query running workflows: %w", err)
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

	// Convert to simple map with just the raw values
	simpleMap := make(map[string]string)
	for key, value := range lastEvaluatedKey {
		switch v := value.(type) {
		case *types.AttributeValueMemberS:
			simpleMap[key] = "S:" + v.Value
		case *types.AttributeValueMemberN:
			simpleMap[key] = "N:" + v.Value
		case *types.AttributeValueMemberB:
			simpleMap[key] = "B:" + base64.StdEncoding.EncodeToString(v.Value)
		default:
			return "", fmt.Errorf("unsupported attribute type: %T", value)
		}
	}

	jsonData, err := json.Marshal(simpleMap)
	if err != nil {
		return "", fmt.Errorf("failed to json marshal: %w", err)
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

	var simpleMap map[string]string
	if err := json.Unmarshal(jsonData, &simpleMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal next token: %w", err)
	}

	// Convert back to AttributeValue map
	result := make(map[string]types.AttributeValue)
	for key, value := range simpleMap {
		if len(value) < 2 || value[1] != ':' {
			return nil, fmt.Errorf("invalid token format for key %s", key)
		}

		prefix := value[:1]
		data := value[2:]

		switch prefix {
		case "S":
			result[key] = &types.AttributeValueMemberS{Value: data}
		case "N":
			result[key] = &types.AttributeValueMemberN{Value: data}
		case "B":
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode binary data for key %s: %w", key, err)
			}
			result[key] = &types.AttributeValueMemberB{Value: decoded}
		default:
			return nil, fmt.Errorf("unsupported attribute type prefix: %s", prefix)
		}
	}

	return result, nil
}
