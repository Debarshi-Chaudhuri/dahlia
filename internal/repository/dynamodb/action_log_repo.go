package dynamodb

import (
	"context"
	"fmt"

	"dahlia/internal/domain"
	"dahlia/internal/logger"
	repository "dahlia/internal/repository/iface"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type actionLogRepository struct {
	client    *dynamodb.Client
	tableName string
	logger    logger.Logger
}

// NewActionLogRepository creates a new DynamoDB action log repository
func NewActionLogRepository(client *dynamodb.Client, log logger.Logger) repository.ActionLogRepository {
	return &actionLogRepository{
		client:    client,
		tableName: "action_logs",
		logger:    log.With(logger.String("component", "action_log_repository")),
	}
}

func (r *actionLogRepository) Create(ctx context.Context, log *domain.ActionLog) error {
	r.logger.Debug("creating action log",
		logger.String("run_id", log.RunID),
		logger.Int("action_index", log.ActionIndex))

	item, err := attributevalue.MarshalMap(log)
	if err != nil {
		r.logger.Error("failed to marshal action log", logger.Error(err))
		return fmt.Errorf("failed to marshal action log: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	if err != nil {
		r.logger.Error("failed to create action log", logger.Error(err))
		return fmt.Errorf("failed to create action log: %w", err)
	}

	r.logger.Debug("action log created",
		logger.String("log_id", log.LogID))

	return nil
}

func (r *actionLogRepository) Update(ctx context.Context, log *domain.ActionLog) error {
	r.logger.Debug("updating action log",
		logger.String("log_id", log.LogID),
		logger.String("status", string(log.Status)))

	item, err := attributevalue.MarshalMap(log)
	if err != nil {
		return fmt.Errorf("failed to marshal action log: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	if err != nil {
		r.logger.Error("failed to update action log", logger.Error(err))
		return fmt.Errorf("failed to update action log: %w", err)
	}

	r.logger.Debug("action log updated",
		logger.String("log_id", log.LogID))

	return nil
}

func (r *actionLogRepository) GetByRunID(ctx context.Context, runID string) ([]*domain.ActionLog, error) {
	r.logger.Debug("getting action logs by run ID",
		logger.String("run_id", runID))

	// Query by run_id prefix since PK is run_id#action_index
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("begins_with(run_id_action_index, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: runID + "#"},
		},
	})

	if err != nil {
		r.logger.Error("failed to query action logs", logger.Error(err))
		return nil, fmt.Errorf("failed to query action logs: %w", err)
	}

	logs := make([]*domain.ActionLog, 0, len(result.Items))
	for _, item := range result.Items {
		var log domain.ActionLog
		if err := attributevalue.UnmarshalMap(item, &log); err != nil {
			r.logger.Warn("failed to unmarshal action log", logger.Error(err))
			continue
		}
		logs = append(logs, &log)
	}

	r.logger.Debug("action logs retrieved",
		logger.String("run_id", runID),
		logger.Int("count", len(logs)))

	return logs, nil
}

func (r *actionLogRepository) GetByRunIDAndAction(ctx context.Context, runID string, actionIndex int) (*domain.ActionLog, error) {
	r.logger.Debug("getting action log by run ID and action",
		logger.String("run_id", runID),
		logger.Int("action_index", actionIndex))

	compositeKey := fmt.Sprintf("%s#%d", runID, actionIndex)

	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("run_id_action_index = :key"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":key": &types.AttributeValueMemberS{Value: compositeKey},
		},
		Limit: aws.Int32(1),
	})

	if err != nil {
		r.logger.Error("failed to get action log", logger.Error(err))
		return nil, fmt.Errorf("failed to get action log: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("action log not found: %s#%d", runID, actionIndex)
	}

	var log domain.ActionLog
	if err := attributevalue.UnmarshalMap(result.Items[0], &log); err != nil {
		return nil, fmt.Errorf("failed to unmarshal action log: %w", err)
	}

	return &log, nil
}
