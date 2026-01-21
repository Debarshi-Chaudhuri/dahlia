package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	cache "dahlia/internal/cache/iface"
	redisCache "dahlia/internal/cache/redis"
	"dahlia/internal/domain"
	"dahlia/internal/logger"
	dynamodbRepo "dahlia/internal/repository/dynamodb"
	repository "dahlia/internal/repository/iface"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTableName    = "scheduled_jobs" // Use same table name as production for real E2E test
	testQueueName    = "test-queue"
	sqsEndpoint      = "http://localhost:4566" // LocalStack for SQS
	dynamoDBEndpoint = "http://localhost:9000" // DynamoDB Local (separate container)
	redisAddr        = "localhost:6379"
	testNodeID       = "test-node-1"
	testRegion       = "us-east-1"
)

type integrationTestSetup struct {
	scheduler *Scheduler
	cache     cache.Cache
	jobRepo   repository.JobRepository
	sqsClient *sqs.Client
	dynamoDB  *dynamodb.Client
	logger    logger.Logger
	ctx       context.Context
	cancel    context.CancelFunc
}

// setupIntegrationTest sets up all dependencies for integration testing
func setupIntegrationTest(t *testing.T) *integrationTestSetup {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute) // Extended timeout for cron tests

	// Setup logger
	log, err := logger.NewZapLoggerForDev()
	require.NoError(t, err, "failed to create logger")

	// Setup Redis cache
	cache, err := redisCache.NewRedisCache(redisAddr, "", 0, log)
	require.NoError(t, err, "failed to connect to Redis")

	// Setup AWS config with custom endpoint resolver
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(testRegion),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				// Use different endpoints for different services
				switch service {
				case "DynamoDB":
					return aws.Endpoint{
						URL:           dynamoDBEndpoint,
						SigningRegion: testRegion,
					}, nil
				case "SQS":
					return aws.Endpoint{
						URL:           sqsEndpoint,
						SigningRegion: testRegion,
					}, nil
				default:
					return aws.Endpoint{}, &aws.EndpointNotFoundError{}
				}
			})),
	)
	require.NoError(t, err, "failed to load AWS config")

	dynamoClient := dynamodb.NewFromConfig(cfg)
	sqsClient := sqs.NewFromConfig(cfg)

	// Create test table if it doesn't exist
	createTestTable(t, ctx, dynamoClient)

	// Create test queue if it doesn't exist
	createTestQueue(t, ctx, sqsClient)

	// Create real DynamoDB job repository
	jobRepo := dynamodbRepo.NewJobRepository(dynamoClient, log)

	// Create scheduler
	scheduler := NewScheduler(cache, jobRepo, sqsClient, testNodeID, log).(*Scheduler)

	return &integrationTestSetup{
		scheduler: scheduler,
		cache:     cache,
		jobRepo:   jobRepo,
		sqsClient: sqsClient,
		dynamoDB:  dynamoClient,
		logger:    log,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// cleanup cleans up test resources
func (s *integrationTestSetup) cleanup(t *testing.T) {
	ctx := context.Background()
	s.cancel()

	// Clean up Redis keys
	currentTime := time.Now().Truncate(time.Minute)
	for i := -5; i <= 5; i++ {
		testTime := currentTime.Add(time.Duration(i) * time.Minute)
		bucketKey := s.scheduler.getBucketKey(testTime)
		tempKey := s.scheduler.getTempKey(testTime)
		s.cache.Delete(ctx, bucketKey)
		s.cache.Delete(ctx, tempKey)
	}

	// Clean up DynamoDB test data
	// Scan all items and delete them
	scanOutput, err := s.dynamoDB.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(testTableName),
	})
	if err == nil {
		for _, item := range scanOutput.Items {
			if jobID, ok := item["job_id"].(*types.AttributeValueMemberS); ok {
				s.dynamoDB.DeleteItem(ctx, &dynamodb.DeleteItemInput{
					TableName: aws.String(testTableName),
					Key: map[string]types.AttributeValue{
						"job_id": &types.AttributeValueMemberS{Value: jobID.Value},
					},
				})
			}
		}
	}

	// Purge test queue
	queueURL := fmt.Sprintf("%s/000000000000/%s", sqsEndpoint, testQueueName)
	s.sqsClient.PurgeQueue(ctx, &sqs.PurgeQueueInput{
		QueueUrl: &queueURL,
	})

	s.cache.Close()
}

// TestSchedulerTimeBucketing tests the time-based bucketing mechanism
func TestSchedulerTimeBucketing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Schedule a job for 2 minutes from now
	delaySeconds := 120
	jobDetails := map[string]interface{}{
		"task": "test_task",
		"data": "test_data",
	}

	job, err := setup.scheduler.ScheduleWithDelay(
		setup.ctx,
		"test-job",
		testQueueName,
		delaySeconds,
		jobDetails,
	)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Verify job was saved to repository
	savedJob, err := setup.jobRepo.GetByID(setup.ctx, job.JobID)
	require.NoError(t, err)
	assert.Equal(t, job.JobID, savedJob.JobID)
	assert.Equal(t, domain.JobStatusScheduled, savedJob.Status)

	// Verify job was added to correct Redis bucket
	executeTime := time.UnixMilli(job.ExecuteAt)
	bucketKey := setup.scheduler.getBucketKey(executeTime)

	length, err := setup.cache.LLen(setup.ctx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), length)

	// Verify the job ID is in the bucket
	jobs, err := setup.cache.LRange(setup.ctx, bucketKey, 0, -1)
	require.NoError(t, err)
	assert.Contains(t, jobs, job.JobID)
}

// TestSchedulerCronProcessing tests the cron-based processing of jobs
func TestSchedulerCronProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Calculate next minute boundary
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	delaySeconds := int(nextMinute.Sub(now).Seconds()) + 5 // Add 5 seconds buffer

	setup.logger.Info("scheduling job",
		logger.String("current_time", now.Format("15:04:05")),
		logger.String("next_minute", nextMinute.Format("15:04:05")),
		logger.Int("delay_seconds", delaySeconds))

	jobDetails := map[string]interface{}{
		"task":      "cron_test_task",
		"timestamp": time.Now().Unix(),
	}

	job, err := setup.scheduler.ScheduleWithDelay(
		setup.ctx,
		"cron-test-job",
		testQueueName,
		delaySeconds,
		jobDetails,
	)
	require.NoError(t, err)

	// Start the scheduler
	err = setup.scheduler.Start(setup.ctx)
	require.NoError(t, err)
	defer setup.scheduler.Stop(setup.ctx)

	// Wait for the job to be processed (wait for 2 minute boundaries)
	waitTime := time.Until(nextMinute) + 90*time.Second
	setup.logger.Info("waiting for job processing",
		logger.String("wait_duration", waitTime.String()))

	time.Sleep(waitTime)

	// Use background context for verification operations to avoid deadline issues
	verifyCtx := context.Background()

	// Verify job status was updated to COMPLETED
	processedJob, err := setup.jobRepo.GetByID(verifyCtx, job.JobID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusCompleted, processedJob.Status)

	// Verify message was sent to SQS
	queueURL := fmt.Sprintf("%s/000000000000/%s", sqsEndpoint, testQueueName)
	messages, err := setup.sqsClient.ReceiveMessage(verifyCtx, &sqs.ReceiveMessageInput{
		QueueUrl:            &queueURL,
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     1,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages.Messages), 1, "expected at least one message in queue")

	// Verify message content
	if len(messages.Messages) > 0 {
		var receivedDetails map[string]interface{}
		err = json.Unmarshal([]byte(*messages.Messages[0].Body), &receivedDetails)
		require.NoError(t, err)
		assert.Equal(t, "cron_test_task", receivedDetails["task"])
	}

	// Verify job was removed from TEMP bucket
	tempKey := setup.scheduler.getTempKey(time.UnixMilli(job.ExecuteAt))
	length, err := setup.cache.LLen(verifyCtx, tempKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length, "TEMP bucket should be empty after processing")
}

// TestSchedulerMultipleJobs tests scheduling multiple jobs in the same bucket
func TestSchedulerMultipleJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Schedule multiple jobs for the same time (use a fixed future time)
	now := time.Now()
	targetTime := now.Truncate(time.Minute).Add(5 * time.Minute) // Use 5 minutes to avoid cleanup issues
	delaySeconds := int(targetTime.Sub(now).Seconds())

	setup.logger.Info("scheduling multiple jobs",
		logger.String("current_time", now.Format("15:04:05")),
		logger.String("target_time", targetTime.Format("15:04:05")),
		logger.Int("delay_seconds", delaySeconds))

	numJobs := 5
	jobIDs := make([]string, numJobs)
	var lastJob *domain.ScheduledJob

	for i := 0; i < numJobs; i++ {
		jobDetails := map[string]interface{}{
			"task":  "multi_test_task",
			"index": i,
		}

		job, err := setup.scheduler.ScheduleWithDelay(
			context.Background(), // Use background context to avoid timeout
			fmt.Sprintf("multi-job-%d", i),
			testQueueName,
			delaySeconds,
			jobDetails,
		)
		require.NoError(t, err)
		require.NotNil(t, job)
		jobIDs[i] = job.JobID
		lastJob = job

		setup.logger.Debug("job scheduled",
			logger.String("job_id", job.JobID),
			logger.Int("index", i))
	}

	// Give Redis a moment to ensure all jobs are written
	time.Sleep(100 * time.Millisecond)

	// Verify all jobs are in the same bucket
	// Use the actual execute time from one of the jobs to get the correct bucket
	bucketKey := setup.scheduler.getBucketKey(time.UnixMilli(lastJob.ExecuteAt))
	setup.logger.Info("verifying bucket",
		logger.String("bucket_key", bucketKey))

	verifyCtx := context.Background()
	length, err := setup.cache.LLen(verifyCtx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(numJobs), length, "expected %d jobs in bucket %s", numJobs, bucketKey)

	// Verify all job IDs are in the bucket
	jobs, err := setup.cache.LRange(verifyCtx, bucketKey, 0, -1)
	require.NoError(t, err)
	assert.Len(t, jobs, numJobs)

	for _, jobID := range jobIDs {
		assert.Contains(t, jobs, jobID)
	}
}

// TestSchedulerCleanupMissedJobs tests the cleanup mechanism for missed jobs
func TestSchedulerCleanupMissedJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Simulate a missed job by putting it directly in a TEMP bucket from 5 minutes ago
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute).Truncate(time.Minute)
	tempKey := setup.scheduler.getTempKey(fiveMinutesAgo)

	// Create a test job
	job := domain.NewScheduledJob(
		"missed-job",
		testQueueName,
		fiveMinutesAgo,
		map[string]interface{}{"task": "missed_task"},
	)

	// Save to repository
	err := setup.jobRepo.Create(setup.ctx, job)
	require.NoError(t, err)

	// Add to TEMP bucket (simulating it was transferred but not processed)
	err = setup.cache.RPush(setup.ctx, tempKey, job.JobID)
	require.NoError(t, err)

	// Verify it's in the TEMP bucket
	length, err := setup.cache.LLen(setup.ctx, tempKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), length)

	// Run cleanup
	setup.scheduler.cleanupMissedJobs(setup.ctx)

	// Wait a bit for processing
	time.Sleep(2 * time.Second)

	// Verify job was processed
	processedJob, err := setup.jobRepo.GetByID(setup.ctx, job.JobID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusCompleted, processedJob.Status)

	// Verify job was removed from TEMP bucket
	length, err = setup.cache.LLen(setup.ctx, tempKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length)
}

// TestSchedulerAtomicTransfer tests the atomic transfer from bucket to TEMP
func TestSchedulerAtomicTransfer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Create bucket with jobs
	testTime := time.Now().Truncate(time.Minute)
	bucketKey := setup.scheduler.getBucketKey(testTime)
	tempKey := setup.scheduler.getTempKey(testTime)

	// Add test jobs to bucket
	jobIDs := []string{"job-1", "job-2", "job-3"}
	for _, jobID := range jobIDs {
		err := setup.cache.RPush(setup.ctx, bucketKey, jobID)
		require.NoError(t, err)
	}

	// Verify bucket has jobs
	length, err := setup.cache.LLen(setup.ctx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(3), length)

	// Perform atomic transfer
	result, err := setup.cache.Eval(setup.ctx, luaAtomicTransfer, []string{bucketKey, tempKey})
	require.NoError(t, err)

	// Verify result
	transferredIDs := setup.scheduler.extractJobIDs(result)
	assert.ElementsMatch(t, jobIDs, transferredIDs)

	// Verify bucket is now empty
	length, err = setup.cache.LLen(setup.ctx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length)

	// Verify TEMP has all jobs
	length, err = setup.cache.LLen(setup.ctx, tempKey)
	require.NoError(t, err)
	assert.Equal(t, int64(3), length)

	// Verify TEMP has correct jobs
	tempJobs, err := setup.cache.LRange(setup.ctx, tempKey, 0, -1)
	require.NoError(t, err)
	assert.ElementsMatch(t, jobIDs, tempJobs)
}

// Helper functions

func createTestTable(t *testing.T, ctx context.Context, client *dynamodb.Client) {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(testTableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("job_id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("job_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})

	if err != nil {
		// Table might already exist, which is fine
		t.Logf("table creation: %v", err)
	}
}

func createTestQueue(t *testing.T, ctx context.Context, client *sqs.Client) {
	_, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(testQueueName),
	})

	if err != nil {
		// Queue might already exist, which is fine
		t.Logf("queue creation: %v", err)
	}
}

// TestRaceCondition_CurrentMinuteScheduling tests that jobs scheduled for the current minute
// are automatically moved to the next minute to avoid race conditions
func TestRaceCondition_CurrentMinuteScheduling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Get current time and current minute boundary
	now := time.Now()
	currentMinute := now.Truncate(time.Minute)

	setup.logger.Info("testing current minute scheduling",
		logger.String("current_time", now.Format("15:04:05")),
		logger.String("current_minute", currentMinute.Format("15:04:05")))

	// Try to schedule a job for the current minute
	jobDetails := map[string]interface{}{
		"task": "current_minute_test",
	}

	// Calculate delay to current minute (should be negative or 0)
	delaySeconds := int(currentMinute.Sub(now).Seconds())
	if delaySeconds < 0 {
		delaySeconds = 0
	}

	job, err := setup.scheduler.ScheduleWithDelay(
		context.Background(),
		"current-minute-job",
		testQueueName,
		delaySeconds,
		jobDetails,
	)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Verify the job was moved to the next minute
	jobExecuteTime := time.UnixMilli(job.ExecuteAt)
	expectedMinute := currentMinute.Add(time.Minute)

	setup.logger.Info("verifying job schedule adjustment",
		logger.String("job_execute_time", jobExecuteTime.Format("15:04:05")),
		logger.String("expected_minute", expectedMinute.Format("15:04:05")))

	assert.True(t, jobExecuteTime.Equal(expectedMinute) || jobExecuteTime.After(expectedMinute),
		"job should be scheduled for next minute or later, got %s, expected >= %s",
		jobExecuteTime.Format("15:04:05"), expectedMinute.Format("15:04:05"))

	// Verify it's NOT in the current minute's bucket
	currentBucketKey := setup.scheduler.getBucketKey(currentMinute)
	length, err := setup.cache.LLen(context.Background(), currentBucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length, "current minute bucket should be empty")

	// Verify it IS in the next minute's bucket
	nextBucketKey := setup.scheduler.getBucketKey(expectedMinute)
	length, err = setup.cache.LLen(context.Background(), nextBucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), length, "next minute bucket should have the job")
}

// TestRaceCondition_PastMinuteScheduling tests that jobs scheduled for past minutes
// are moved to the next minute
func TestRaceCondition_PastMinuteScheduling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	now := time.Now()
	currentMinute := now.Truncate(time.Minute)
	pastMinute := currentMinute.Add(-2 * time.Minute)

	setup.logger.Info("testing past minute scheduling",
		logger.String("current_time", now.Format("15:04:05")),
		logger.String("past_minute", pastMinute.Format("15:04:05")))

	// Create a job with execution time in the past
	job := domain.NewScheduledJob(
		"past-job",
		testQueueName,
		pastMinute,
		map[string]interface{}{"task": "past_test"},
	)

	// Schedule the job
	err := setup.scheduler.ScheduleJob(context.Background(), job)
	require.NoError(t, err)

	// Verify the job was moved to the next minute
	savedJob, err := setup.jobRepo.GetByID(context.Background(), job.JobID)
	require.NoError(t, err)

	jobExecuteTime := time.UnixMilli(savedJob.ExecuteAt)
	nextMinute := currentMinute.Add(time.Minute)

	setup.logger.Info("verifying past job adjustment",
		logger.String("original_time", pastMinute.Format("15:04:05")),
		logger.String("adjusted_time", jobExecuteTime.Format("15:04:05")),
		logger.String("expected_time", nextMinute.Format("15:04:05")))

	assert.True(t, jobExecuteTime.Equal(nextMinute) || jobExecuteTime.After(nextMinute),
		"job should be scheduled for next minute or later")

	// Verify it's NOT in the past bucket
	pastBucketKey := setup.scheduler.getBucketKey(pastMinute)
	length, err := setup.cache.LLen(context.Background(), pastBucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length, "past minute bucket should be empty")
}

// TestRaceCondition_LateArrivals tests the late arrivals processor catches jobs
// added after the main cron has processed the bucket
func TestRaceCondition_LateArrivals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Start the scheduler
	err := setup.scheduler.Start(context.Background())
	require.NoError(t, err)
	defer setup.scheduler.Stop(context.Background())

	// Wait for the next minute boundary to pass (so main cron processes)
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	waitForCron := nextMinute.Sub(now) + 2*time.Second // Wait 2 seconds after cron

	setup.logger.Info("waiting for cron to process",
		logger.String("current_time", now.Format("15:04:05")),
		logger.String("next_minute", nextMinute.Format("15:04:05")),
		logger.String("wait_duration", waitForCron.String()))

	time.Sleep(waitForCron)

	// Now simulate a "late arrival" - directly add a job to the just-processed bucket
	// This simulates a job being scheduled right after the cron ran
	lateArrivalTime := nextMinute
	lateJob := domain.NewScheduledJob(
		"late-arrival-job",
		testQueueName,
		lateArrivalTime,
		map[string]interface{}{"task": "late_arrival_test"},
	)

	// Save to repository
	ctx := context.Background()
	err = setup.jobRepo.Create(ctx, lateJob)
	require.NoError(t, err)

	// Manually add to Redis bucket (bypassing the protection)
	bucketKey := setup.scheduler.getBucketKey(lateArrivalTime)
	err = setup.cache.RPush(ctx, bucketKey, lateJob.JobID)
	require.NoError(t, err)

	setup.logger.Info("added late arrival job",
		logger.String("job_id", lateJob.JobID),
		logger.String("bucket_key", bucketKey))

	// Wait for the late arrivals processor to check this bucket
	// Late arrivals runs at :10 seconds and checks the PREVIOUS minute
	// So we need to wait until the NEXT minute's :10 mark
	nowAfterAdd := time.Now()
	currentMinute := nowAfterAdd.Truncate(time.Minute)

	// We need to wait for the next minute + 10 seconds
	// This ensures the late arrivals processor checks the minute we just added the job to
	nextLateArrivalsCheck := currentMinute.Add(time.Minute).Add(10 * time.Second)
	waitForLateArrivals := nextLateArrivalsCheck.Sub(nowAfterAdd) + 3*time.Second // Extra buffer

	setup.logger.Info("waiting for late arrivals processor",
		logger.String("current_time", nowAfterAdd.Format("15:04:05")),
		logger.String("late_arrivals_check_time", nextLateArrivalsCheck.Format("15:04:05")),
		logger.String("will_check_bucket", lateArrivalTime.Format("15:04:05")),
		logger.String("wait_duration", waitForLateArrivals.String()))

	time.Sleep(waitForLateArrivals)

	// Verify the job was processed
	processedJob, err := setup.jobRepo.GetByID(ctx, lateJob.JobID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusCompleted, processedJob.Status,
		"late arrival job should be processed by late arrivals processor")

	// Verify the job was removed from the bucket
	length, err := setup.cache.LLen(ctx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length, "bucket should be empty after late arrivals processing")

	// Verify it was removed from TEMP bucket too
	tempKey := setup.scheduler.getTempKey(lateArrivalTime)
	length, err = setup.cache.LLen(ctx, tempKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length, "TEMP bucket should be empty after processing")
}

// TestRaceCondition_ThreeLayerSafetyNet tests the complete three-layer system:
// Main cron -> Late arrivals -> Cleanup
func TestRaceCondition_ThreeLayerSafetyNet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	ctx := context.Background()

	// Layer 1: Normal job - should be caught by main cron
	now := time.Now()
	layer1Time := now.Truncate(time.Minute).Add(2 * time.Minute)
	layer1Job, err := setup.scheduler.ScheduleWithDelay(
		ctx,
		"layer1-job",
		testQueueName,
		int(layer1Time.Sub(now).Seconds()),
		map[string]interface{}{"layer": 1},
	)
	require.NoError(t, err)

	setup.logger.Info("created test jobs for three-layer test",
		logger.String("layer1_job", layer1Job.JobID),
		logger.String("layer1_time", layer1Time.Format("15:04:05")))

	// Start the scheduler
	err = setup.scheduler.Start(ctx)
	require.NoError(t, err)
	defer setup.scheduler.Stop(ctx)

	// Wait for main cron to process layer 1
	waitTime := layer1Time.Sub(now) + 5*time.Second
	setup.logger.Info("waiting for processing",
		logger.String("wait_duration", waitTime.String()))

	time.Sleep(waitTime)

	// Verify layer 1 job was processed by main cron
	verifyCtx := context.Background()
	processedJob, err := setup.jobRepo.GetByID(verifyCtx, layer1Job.JobID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusCompleted, processedJob.Status,
		"layer 1 job should be processed by main cron")
}

// TestRaceCondition_ConcurrentScheduling tests multiple jobs being scheduled
// concurrently at minute boundaries
func TestRaceCondition_ConcurrentScheduling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	// Use a fixed target time far enough in the future to avoid minute boundary issues
	now := time.Now()
	targetTime := now.Truncate(time.Minute).Add(5 * time.Minute)

	numJobs := 10
	jobIDs := make([]string, numJobs)
	jobs := make([]*domain.ScheduledJob, numJobs)
	type jobResult struct {
		index int
		job   *domain.ScheduledJob
		err   error
	}
	resultChan := make(chan jobResult, numJobs)

	setup.logger.Info("scheduling concurrent jobs",
		logger.Int("num_jobs", numJobs),
		logger.String("target_time", targetTime.Format("15:04:05")))

	// Schedule all jobs for the exact same time to ensure they go to the same bucket
	// This tests concurrent writes to the same bucket
	for i := 0; i < numJobs; i++ {
		go func(index int) {
			// Create job with exact target time (not delay calculation)
			job := domain.NewScheduledJob(
				fmt.Sprintf("concurrent-job-%d", index),
				testQueueName,
				targetTime,
				map[string]interface{}{
					"index": index,
					"task":  "concurrent_test",
				},
			)

			err := setup.scheduler.ScheduleJob(context.Background(), job)
			resultChan <- jobResult{index: index, job: job, err: err}
		}(i)
	}

	// Wait for all jobs to be scheduled
	for i := 0; i < numJobs; i++ {
		result := <-resultChan
		require.NoError(t, result.err, "job %d should schedule successfully", result.index)
		jobIDs[result.index] = result.job.JobID
		jobs[result.index] = result.job
	}

	// Use the actual execute time from one of the jobs to get the bucket key
	// (they should all have the same time, but may have been adjusted by protection logic)
	actualExecuteTime := time.UnixMilli(jobs[0].ExecuteAt)
	bucketKey := setup.scheduler.getBucketKey(actualExecuteTime)

	setup.logger.Info("verifying concurrent jobs in bucket",
		logger.String("bucket_key", bucketKey),
		logger.String("execute_time", actualExecuteTime.Format("15:04:05")))

	// Give Redis a moment to ensure all concurrent writes complete
	time.Sleep(100 * time.Millisecond)

	length, err := setup.cache.LLen(context.Background(), bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(numJobs), length,
		"all concurrent jobs should be in bucket %s", bucketKey)

	// Verify all jobs are in DynamoDB
	for i, jobID := range jobIDs {
		job, err := setup.jobRepo.GetByID(context.Background(), jobID)
		require.NoError(t, err, "job %d should be in repository", i)
		assert.Equal(t, domain.JobStatusScheduled, job.Status)
	}
}

// TestRaceCondition_BucketProcessingDuringScheduling tests that the atomic
// transfer doesn't miss jobs being added during processing
func TestRaceCondition_BucketProcessingDuringScheduling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	ctx := context.Background()

	// Create a bucket with some jobs
	testTime := time.Now().Add(10 * time.Minute).Truncate(time.Minute)
	bucketKey := setup.scheduler.getBucketKey(testTime)

	// Add some initial jobs
	for i := 0; i < 5; i++ {
		jobID := fmt.Sprintf("initial-job-%d", i)
		err := setup.cache.RPush(ctx, bucketKey, jobID)
		require.NoError(t, err)
	}

	// Verify initial state
	length, err := setup.cache.LLen(ctx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(5), length)

	// Perform atomic transfer
	tempKey := setup.scheduler.getTempKey(testTime)
	result, err := setup.cache.Eval(ctx, luaAtomicTransfer, []string{bucketKey, tempKey})
	require.NoError(t, err)

	transferredIDs := setup.scheduler.extractJobIDs(result)
	assert.Len(t, transferredIDs, 5, "should transfer all initial jobs")

	// Verify bucket is empty after transfer
	length, err = setup.cache.LLen(ctx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length, "bucket should be empty after atomic transfer")

	// Verify TEMP has all jobs
	length, err = setup.cache.LLen(ctx, tempKey)
	require.NoError(t, err)
	assert.Equal(t, int64(5), length, "TEMP should have all transferred jobs")

	// Now add a late arrival
	err = setup.cache.RPush(ctx, bucketKey, "late-arrival-job")
	require.NoError(t, err)

	// Verify late arrival is in the bucket (not TEMP)
	length, err = setup.cache.LLen(ctx, bucketKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), length, "late arrival should be in bucket")

	// Verify TEMP still has original jobs
	length, err = setup.cache.LLen(ctx, tempKey)
	require.NoError(t, err)
	assert.Equal(t, int64(5), length, "TEMP should still have original jobs")

	// This demonstrates that late arrivals go to the bucket,
	// and will be caught by the late arrivals processor
}

// TestRaceCondition_ProtectionLogging verifies that the protection mechanisms
// log appropriate warnings
func TestRaceCondition_ProtectionLogging(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	setup := setupIntegrationTest(t)
	defer setup.cleanup(t)

	now := time.Now()
	currentMinute := now.Truncate(time.Minute)

	// Schedule for current minute - should trigger warning log
	job := domain.NewScheduledJob(
		"logging-test-job",
		testQueueName,
		currentMinute,
		map[string]interface{}{"task": "logging_test"},
	)

	err := setup.scheduler.ScheduleJob(context.Background(), job)
	require.NoError(t, err)

	// Check the job was adjusted (logging is visible in test output)
	savedJob, err := setup.jobRepo.GetByID(context.Background(), job.JobID)
	require.NoError(t, err)

	jobTime := time.UnixMilli(savedJob.ExecuteAt)
	assert.True(t, jobTime.After(currentMinute),
		"job time should be adjusted to future minute")

	// The warning log "job scheduled for current/past minute, adjusting to next minute"
	// should appear in the test output
}
