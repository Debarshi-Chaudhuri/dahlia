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

type jobRepository struct {
	client    *dynamodb.Client
	tableName string
	logger    logger.Logger
}

// NewJobRepository creates a new DynamoDB job repository
func NewJobRepository(client *dynamodb.Client, log logger.Logger) repository.JobRepository {
	return &jobRepository{
		client:    client,
		tableName: "scheduled_jobs",
		logger:    log.With(logger.String("component", "job_repository")),
	}
}

func (r *jobRepository) Create(ctx context.Context, job *domain.ScheduledJob) error {
	r.logger.Debug("creating scheduled job",
		logger.String("job_id", job.JobID))

	item, err := attributevalue.MarshalMap(job)
	if err != nil {
		r.logger.Error("failed to marshal job", logger.Error(err))
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	if err != nil {
		r.logger.Error("failed to create job", logger.Error(err))
		return fmt.Errorf("failed to create job: %w", err)
	}

	r.logger.Info("scheduled job created",
		logger.String("job_id", job.JobID))

	return nil
}

func (r *jobRepository) Update(ctx context.Context, job *domain.ScheduledJob) error {
	r.logger.Debug("updating scheduled job",
		logger.String("job_id", job.JobID),
		logger.String("status", string(job.Status)))

	item, err := attributevalue.MarshalMap(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	if err != nil {
		r.logger.Error("failed to update job", logger.Error(err))
		return fmt.Errorf("failed to update job: %w", err)
	}

	r.logger.Debug("scheduled job updated",
		logger.String("job_id", job.JobID))

	return nil
}

func (r *jobRepository) GetByID(ctx context.Context, jobID string) (*domain.ScheduledJob, error) {
	r.logger.Debug("getting job by ID",
		logger.String("job_id", jobID))

	result, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"job_id": &types.AttributeValueMemberS{Value: jobID},
		},
	})

	if err != nil {
		r.logger.Error("failed to get job", logger.Error(err))
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	var job domain.ScheduledJob
	err = attributevalue.UnmarshalMap(result.Item, &job)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

func (r *jobRepository) GetByStatus(ctx context.Context, status domain.JobStatus, limit int) ([]*domain.ScheduledJob, error) {
	r.logger.Debug("querying jobs by status",
		logger.String("status", string(status)),
		logger.Int("limit", limit))

	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("status_index"),
		KeyConditionExpression: aws.String("#status = :status"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: string(status)},
		},
		Limit: aws.Int32(int32(limit)),
	})

	if err != nil {
		r.logger.Error("failed to query jobs", logger.Error(err))
		return nil, fmt.Errorf("failed to query jobs: %w", err)
	}

	jobs := make([]*domain.ScheduledJob, 0, len(result.Items))
	for _, item := range result.Items {
		var job domain.ScheduledJob
		if err := attributevalue.UnmarshalMap(item, &job); err != nil {
			r.logger.Warn("failed to unmarshal job", logger.Error(err))
			continue
		}
		jobs = append(jobs, &job)
	}

	r.logger.Debug("jobs retrieved",
		logger.String("status", string(status)),
		logger.Int("count", len(jobs)))

	return jobs, nil
}
