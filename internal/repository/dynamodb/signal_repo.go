package dynamodb

import (
	"context"
	"fmt"
	"time"

	"dahlia/internal/domain"
	"dahlia/internal/logger"
	repository "dahlia/internal/repository/iface"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type signalRepository struct {
	client    *dynamodb.Client
	tableName string
	logger    logger.Logger
}

// NewSignalRepository creates a new DynamoDB signal repository
func NewSignalRepository(client *dynamodb.Client, log logger.Logger) repository.SignalRepository {
	return &signalRepository{
		client:    client,
		tableName: "signals",
		logger:    log.With(logger.String("component", "signal_repository")),
	}
}

func (r *signalRepository) Create(ctx context.Context, signal *domain.Signal) error {
	r.logger.Debug("creating signal",
		logger.String("signal_id", signal.SignalID),
		logger.String("signal_type", signal.SignalType))

	item, err := attributevalue.MarshalMap(signal)
	if err != nil {
		r.logger.Error("failed to marshal signal", logger.Error(err))
		return fmt.Errorf("failed to marshal signal: %w", err)
	}

	// Conditional write to prevent duplicates
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(signal_id)"),
	})

	if err != nil {
		// Check if it's a duplicate
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			r.logger.Warn("duplicate signal detected",
				logger.String("signal_id", signal.SignalID))
			return fmt.Errorf("duplicate signal: %s", signal.SignalID)
		}
		r.logger.Error("failed to create signal", logger.Error(err))
		return fmt.Errorf("failed to create signal: %w", err)
	}

	r.logger.Info("signal created",
		logger.String("signal_id", signal.SignalID))

	return nil
}

func (r *signalRepository) GetByID(ctx context.Context, signalID string) (*domain.Signal, error) {
	r.logger.Debug("getting signal by ID",
		logger.String("signal_id", signalID))

	result, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"signal_id": &types.AttributeValueMemberS{Value: signalID},
		},
	})

	if err != nil {
		r.logger.Error("failed to get signal", logger.Error(err))
		return nil, fmt.Errorf("failed to get signal: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("signal not found: %s", signalID)
	}

	var signal domain.Signal
	if err := attributevalue.UnmarshalMap(result.Item, &signal); err != nil {
		return nil, fmt.Errorf("failed to unmarshal signal: %w", err)
	}

	return &signal, nil
}

func (r *signalRepository) GetLastSignal(ctx context.Context, signalType, orgID string, before time.Time) (*domain.Signal, error) {
	r.logger.Debug("getting last signal",
		logger.String("signal_type", signalType),
		logger.String("org_id", orgID),
		logger.String("before", before.Format(time.RFC3339)))

	// Query GSI-1: signal_type#org_id
	compositeKey := signalType + "#" + orgID
	beforeTimestamp := before.Format(time.RFC3339)

	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("signal_type_org_id_index"),
		KeyConditionExpression: aws.String("signal_type_org_id = :composite AND #ts < :before"),
		ExpressionAttributeNames: map[string]string{
			"#ts": "timestamp",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":composite": &types.AttributeValueMemberS{Value: compositeKey},
			":before":    &types.AttributeValueMemberS{Value: beforeTimestamp},
		},
		ScanIndexForward: aws.Bool(false), // Descending order
		Limit:            aws.Int32(1),
	})

	if err != nil {
		r.logger.Error("failed to query last signal", logger.Error(err))
		return nil, fmt.Errorf("failed to query last signal: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil // No signal found
	}

	var signal domain.Signal
	if err := attributevalue.UnmarshalMap(result.Items[0], &signal); err != nil {
		return nil, fmt.Errorf("failed to unmarshal signal: %w", err)
	}

	return &signal, nil
}

func (r *signalRepository) QueryBySignalType(ctx context.Context, signalType string, startTime, endTime time.Time) ([]*domain.Signal, error) {
	r.logger.Debug("querying signals by type",
		logger.String("signal_type", signalType),
		logger.String("start_time", startTime.Format(time.RFC3339)),
		logger.String("end_time", endTime.Format(time.RFC3339)))

	startTimestamp := startTime.Format(time.RFC3339)
	endTimestamp := endTime.Format(time.RFC3339)

	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("signal_type_index"),
		KeyConditionExpression: aws.String("signal_type = :type AND #ts BETWEEN :start AND :end"),
		ExpressionAttributeNames: map[string]string{
			"#ts": "timestamp",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type":  &types.AttributeValueMemberS{Value: signalType},
			":start": &types.AttributeValueMemberS{Value: startTimestamp},
			":end":   &types.AttributeValueMemberS{Value: endTimestamp},
		},
	})

	if err != nil {
		r.logger.Error("failed to query signals", logger.Error(err))
		return nil, fmt.Errorf("failed to query signals: %w", err)
	}

	signals := make([]*domain.Signal, 0, len(result.Items))
	for _, item := range result.Items {
		var signal domain.Signal
		if err := attributevalue.UnmarshalMap(item, &signal); err != nil {
			r.logger.Warn("failed to unmarshal signal", logger.Error(err))
			continue
		}
		signals = append(signals, &signal)
	}

	r.logger.Debug("signals queried",
		logger.String("signal_type", signalType),
		logger.Int("count", len(signals)))

	return signals, nil
}
