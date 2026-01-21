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

type workflowRepository struct {
	client    *dynamodb.Client
	tableName string
	logger    logger.Logger
}

// NewWorkflowRepository creates a new DynamoDB workflow repository
func NewWorkflowRepository(client *dynamodb.Client, log logger.Logger) repository.WorkflowRepository {
	return &workflowRepository{
		client:    client,
		tableName: "workflows",
		logger:    log.With(logger.String("component", "workflow_repository")),
	}
}

func (r *workflowRepository) Create(ctx context.Context, workflow *domain.Workflow) error {
	r.logger.Debug("creating workflow",
		logger.String("workflow_id", workflow.WorkflowID),
		logger.String("name", workflow.Name))

	item, err := attributevalue.MarshalMap(workflow)
	if err != nil {
		r.logger.Error("failed to marshal workflow", logger.Error(err))
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(workflow_id) AND attribute_not_exists(version)"),
	})

	if err != nil {
		r.logger.Error("failed to create workflow", logger.Error(err))
		return fmt.Errorf("failed to create workflow: %w", err)
	}

	r.logger.Info("workflow created",
		logger.String("workflow_id", workflow.WorkflowID),
		logger.Int("version", workflow.Version))

	return nil
}

func (r *workflowRepository) Update(ctx context.Context, workflow *domain.Workflow) error {
	r.logger.Debug("updating workflow",
		logger.String("workflow_id", workflow.WorkflowID),
		logger.Int("version", workflow.Version))

	item, err := attributevalue.MarshalMap(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	if err != nil {
		r.logger.Error("failed to update workflow", logger.Error(err))
		return fmt.Errorf("failed to update workflow: %w", err)
	}

	r.logger.Info("workflow updated",
		logger.String("workflow_id", workflow.WorkflowID))

	return nil
}

func (r *workflowRepository) GetByID(ctx context.Context, workflowID string, version int) (*domain.Workflow, error) {
	r.logger.Debug("getting workflow by ID",
		logger.String("workflow_id", workflowID),
		logger.Int("version", version))

	result, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"workflow_id": &types.AttributeValueMemberS{Value: workflowID},
			"version":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", version)},
		},
	})

	if err != nil {
		r.logger.Error("failed to get workflow", logger.Error(err))
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("workflow not found: %s (version %d)", workflowID, version)
	}

	var workflow domain.Workflow
	if err := attributevalue.UnmarshalMap(result.Item, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	return &workflow, nil
}

func (r *workflowRepository) GetBySignalType(ctx context.Context, signalType string) ([]*domain.Workflow, error) {
	r.logger.Debug("getting workflows by signal type",
		logger.String("signal_type", signalType))

	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("signal_type_index"),
		KeyConditionExpression: aws.String("signal_type = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: signalType},
		},
	})

	if err != nil {
		r.logger.Error("failed to query workflows", logger.Error(err))
		return nil, fmt.Errorf("failed to query workflows: %w", err)
	}

	workflows := make([]*domain.Workflow, 0, len(result.Items))
	for _, item := range result.Items {
		var workflow domain.Workflow
		if err := attributevalue.UnmarshalMap(item, &workflow); err != nil {
			r.logger.Warn("failed to unmarshal workflow", logger.Error(err))
			continue
		}
		workflows = append(workflows, &workflow)
	}

	r.logger.Debug("workflows retrieved",
		logger.String("signal_type", signalType),
		logger.Int("count", len(workflows)))

	return workflows, nil
}

func (r *workflowRepository) List(ctx context.Context, limit int) ([]*domain.Workflow, error) {
	r.logger.Debug("listing workflows",
		logger.Int("limit", limit))

	result, err := r.client.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
		Limit:     aws.Int32(int32(limit)),
	})

	if err != nil {
		r.logger.Error("failed to list workflows", logger.Error(err))
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	workflows := make([]*domain.Workflow, 0, len(result.Items))
	for _, item := range result.Items {
		var workflow domain.Workflow
		if err := attributevalue.UnmarshalMap(item, &workflow); err != nil {
			r.logger.Warn("failed to unmarshal workflow", logger.Error(err))
			continue
		}
		workflows = append(workflows, &workflow)
	}

	r.logger.Debug("workflows listed",
		logger.Int("count", len(workflows)))

	return workflows, nil
}
