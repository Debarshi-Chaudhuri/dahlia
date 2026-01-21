package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	cache "dahlia/internal/cache/iface"
	"dahlia/internal/domain"
	"dahlia/internal/logger"
	repository "dahlia/internal/repository/iface"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/robfig/cron/v3"
)

const (
	luaAtomicTransfer = `
		local jobs = redis.call('LRANGE', KEYS[1], 0, -1)
		redis.call('DEL', KEYS[1])
		if #jobs > 0 then
			redis.call('RPUSH', KEYS[2], unpack(jobs))
		end
		return jobs
	`
)

type IScheduler interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	ScheduleJob(ctx context.Context, job *domain.ScheduledJob) error
	ScheduleWithDelay(ctx context.Context, jobName, queueName string, delaySeconds int, jobDetails map[string]interface{}) (*domain.ScheduledJob, error)
}

type Scheduler struct {
	cache     cache.Cache
	jobRepo   repository.JobRepository
	sqsClient *sqs.Client
	logger    logger.Logger
	cron      *cron.Cron
	nodeID    string
	stopCh    chan struct{}
}

// NewScheduler creates a new scheduler service
func NewScheduler(
	cache cache.Cache,
	jobRepo repository.JobRepository,
	sqsClient *sqs.Client,
	nodeID string,
	log logger.Logger,
) IScheduler {
	return &Scheduler{
		cache:     cache,
		jobRepo:   jobRepo,
		sqsClient: sqsClient,
		logger:    log.With(logger.String("component", "scheduler")),
		cron:      cron.New(cron.WithSeconds()),
		nodeID:    nodeID,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the scheduler cron jobs
func (s *Scheduler) Start(ctx context.Context) error {
	// Main cron: Every minute at :00 seconds
	_, err := s.cron.AddFunc("0 * * * * *", func() {
		s.processCurrentMinuteBucket(context.Background())
	})
	if err != nil {
		return fmt.Errorf("failed to add main cron: %w", err)
	}

	// Late arrivals check: 10 seconds after main cron to catch jobs added after initial processing
	_, err = s.cron.AddFunc("10 * * * * *", func() {
		s.processLateArrivals(context.Background())
	})
	if err != nil {
		return fmt.Errorf("failed to add late arrivals cron: %w", err)
	}

	// Cleanup cron: Every minute at :30 seconds for jobs missed 5 minutes ago
	_, err = s.cron.AddFunc("30 * * * * *", func() {
		s.cleanupMissedJobs(context.Background())
	})
	if err != nil {
		return fmt.Errorf("failed to add cleanup cron: %w", err)
	}

	s.cron.Start()

	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop(ctx context.Context) error {
	close(s.stopCh)
	cronCtx := s.cron.Stop()
	<-cronCtx.Done()
	return nil
}

// ScheduleJob schedules a job for delayed execution
func (s *Scheduler) ScheduleJob(ctx context.Context, job *domain.ScheduledJob) error {
	executeTime := time.UnixMilli(job.ExecuteAt)
	currentTime := time.Now()

	s.logger.Debug("scheduling job",
		logger.String("job_id", job.JobID),
		logger.String("original_execute_time", executeTime.Format("2006-01-02 15:04:05")),
		logger.String("current_time", currentTime.Format("2006-01-02 15:04:05")))

	// Check if the job's execution time is in the past or current minute that might have been processed
	currentMinute := currentTime.Truncate(time.Minute)
	executeMinute := executeTime.Truncate(time.Minute)

	s.logger.Debug("minute comparison",
		logger.String("job_id", job.JobID),
		logger.String("current_minute", currentMinute.Format("2006-01-02 15	:04:05")),
		logger.String("execute_minute", executeMinute.Format("2006-01-02 15:04:05")),
		logger.String("execute_before_current", strconv.FormatBool(executeMinute.Before(currentMinute))),
		logger.String("execute_equals_current", strconv.FormatBool(executeMinute.Equal(currentMinute))))

	// If scheduling for current or past minute, add safety buffer to next processing cycle
	if executeMinute.Before(currentMinute) || executeMinute.Equal(currentMinute) {
		s.logger.Warn("job scheduled for current/past minute, adjusting to next minute",
			logger.String("job_id", job.JobID),
			logger.String("original_time", executeTime.Format("15:04:05")),
			logger.String("current_time", currentTime.Format("15:04:05")))

		// Move to next minute to avoid race condition
		executeTime = currentMinute.Add(time.Minute)
		job.ExecuteAt = executeTime.UnixMilli()

		s.logger.Debug("adjusted execution time",
			logger.String("job_id", job.JobID),
			logger.String("new_execute_time", executeTime.Format("2006-01-02 15:04:05")))
	}

	// Save to DynamoDB
	if err := s.jobRepo.Create(ctx, job); err != nil {
		s.logger.Error("failed to create job in dynamodb", logger.Error(err))
		return fmt.Errorf("failed to create job: %w", err)
	}

	// Add to Redis bucket
	bucketKey := s.getBucketKey(executeTime)

	s.logger.Debug("adding job to redis bucket",
		logger.String("job_id", job.JobID),
		logger.String("bucket_key", bucketKey),
		logger.String("final_execute_time", executeTime.Format("2006-01-02 15:04:05")))

	if err := s.cache.RPush(ctx, bucketKey, job.JobID); err != nil {
		s.logger.Error("failed to add job to redis bucket",
			logger.String("bucket_key", bucketKey),
			logger.Error(err))
		return fmt.Errorf("failed to add job to bucket: %w", err)
	}

	return nil
}

// ScheduleWithDelay schedules a job with delay in seconds
func (s *Scheduler) ScheduleWithDelay(ctx context.Context, jobName, queueName string, delaySeconds int, jobDetails map[string]interface{}) (*domain.ScheduledJob, error) {
	executeAt := time.Now().Add(time.Duration(delaySeconds) * time.Second)

	// For longer delays, use Redis buckets
	job := domain.NewScheduledJob(jobName, queueName, executeAt, jobDetails)
	if err := s.ScheduleJob(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

// processCurrentMinuteBucket processes jobs for the current minute
func (s *Scheduler) processCurrentMinuteBucket(ctx context.Context) {
	currentTime := time.Now().Truncate(time.Minute)
	bucketKey := s.getBucketKey(currentTime)
	tempKey := s.getTempKey(currentTime)

	// Check if bucket has jobs
	length, err := s.cache.LLen(ctx, bucketKey)
	if err != nil {
		s.logger.Error("failed to get bucket length", logger.Error(err))
		return
	}

	if length == 0 {
		return
	}

	// Atomic transfer from bucket to TEMP
	result, err := s.cache.Eval(ctx, luaAtomicTransfer, []string{bucketKey, tempKey})
	if err != nil {
		s.logger.Error("failed to transfer jobs to TEMP bucket", logger.Error(err))
		return
	}

	jobIDs := s.extractJobIDs(result)
	// Process each job
	for _, jobID := range jobIDs {
		s.processJob(ctx, jobID, tempKey)
	}
}

// processJob processes a single job
func (s *Scheduler) processJob(ctx context.Context, jobID, tempKey string) {
	// Fetch job from DynamoDB
	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		s.logger.Error("failed to get job from dynamodb",
			logger.String("job_id", jobID),
			logger.Error(err))
		s.cache.LRem(ctx, tempKey, 1, jobID)
		return
	}

	// Update status to RUNNING
	job.Status = domain.JobStatusRunning
	job.UpdatedAt = time.Now().UnixMilli()
	if err := s.jobRepo.Update(ctx, job); err != nil {
		s.logger.Error("failed to update job status to RUNNING",
			logger.String("job_id", jobID),
			logger.Error(err))
	}

	// Send to destination queue
	if err := s.sendToQueue(ctx, job); err != nil {
		s.logger.Error("failed to send job to queue",
			logger.String("job_id", jobID),
			logger.Error(err))
		return
	}

	// Mark as COMPLETED
	job.Status = domain.JobStatusCompleted
	job.UpdatedAt = time.Now().UnixMilli()
	if err := s.jobRepo.Update(ctx, job); err != nil {
		s.logger.Error("failed to update job status to COMPLETED",
			logger.String("job_id", jobID),
			logger.Error(err))
	}

	// Remove from TEMP bucket
	if err := s.cache.LRem(ctx, tempKey, 1, jobID); err != nil {
		s.logger.Error("failed to remove job from TEMP bucket",
			logger.String("job_id", jobID),
			logger.Error(err))
	}

}

// processLateArrivals checks for jobs added to the previous minute's bucket after initial processing
func (s *Scheduler) processLateArrivals(ctx context.Context) {
	// Check the previous minute's bucket for any late arrivals
	previousMinute := time.Now().Add(-1 * time.Minute).Truncate(time.Minute)
	bucketKey := s.getBucketKey(previousMinute)
	tempKey := s.getTempKey(previousMinute)

	// Check if there are any jobs in the bucket (late arrivals)
	length, err := s.cache.LLen(ctx, bucketKey)
	if err != nil {
		s.logger.Error("failed to check late arrivals bucket", logger.Error(err))
		return
	}

	if length == 0 {
		return // No late arrivals
	}

	// Atomic transfer late arrivals to TEMP bucket
	result, err := s.cache.Eval(ctx, luaAtomicTransfer, []string{bucketKey, tempKey})
	if err != nil {
		s.logger.Error("failed to transfer late arrivals", logger.Error(err))
		return
	}

	jobIDs := s.extractJobIDs(result)
	// Process each late arrival job
	for _, jobID := range jobIDs {
		s.processJob(ctx, jobID, tempKey)
	}
}

// cleanupMissedJobs checks TEMP buckets from 5 minutes ago
func (s *Scheduler) cleanupMissedJobs(ctx context.Context) {
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute).Truncate(time.Minute)
	tempKey := s.getTempKey(fiveMinutesAgo)

	length, err := s.cache.LLen(ctx, tempKey)
	if err != nil {
		s.logger.Error("failed to get TEMP bucket length", logger.Error(err))
		return
	}

	if length == 0 {
		return
	}

	jobIDs, err := s.cache.LRange(ctx, tempKey, 0, -1)
	if err != nil {
		s.logger.Error("failed to get jobs from TEMP bucket", logger.Error(err))
		return
	}

	for _, jobID := range jobIDs {
		s.processJob(ctx, jobID, tempKey)
	}
}

// sendToQueue sends job details to the destination queue
func (s *Scheduler) sendToQueue(ctx context.Context, job *domain.ScheduledJob) error {
	queueURL := fmt.Sprintf("http://localhost:4566/000000000000/%s", job.QueueName)

	messageBody, err := json.Marshal(job.JobDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal job details: %w", err)
	}

	bodyStr := string(messageBody)
	_, err = s.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: &bodyStr,
	})

	if err != nil {
		return fmt.Errorf("failed to send message to SQS: %w", err)
	}

	return nil
}

// executeJobDirectly executes a job without going through scheduler (for short delays)
func (s *Scheduler) executeJobDirectly(ctx context.Context, jobName, queueName string, jobDetails map[string]interface{}) {
	queueURL := fmt.Sprintf("http://localhost:4566/000000000000/%s", queueName)

	messageBody, err := json.Marshal(jobDetails)
	if err != nil {
		s.logger.Error("failed to marshal job details", logger.Error(err))
		return
	}

	bodyStr := string(messageBody)
	_, err = s.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: &bodyStr,
	})

	if err != nil {
		s.logger.Error("failed to send message to SQS", logger.Error(err))
		return
	}
}

// getBucketKey generates Redis key for a given time
func (s *Scheduler) getBucketKey(t time.Time) string {
	return fmt.Sprintf("schedule:%04d:%02d:%02d:%02d:%02d:%s",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), s.nodeID)
}

// getTempKey generates TEMP Redis key for a given time
func (s *Scheduler) getTempKey(t time.Time) string {
	return fmt.Sprintf("schedule:%04d:%02d:%02d:%02d:%02d:%s_TEMP",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), s.nodeID)
}

// extractJobIDs extracts job IDs from Lua script result
func (s *Scheduler) extractJobIDs(result interface{}) []string {
	jobIDs := []string{}

	if result == nil {
		return jobIDs
	}

	if jobArray, ok := result.([]interface{}); ok {
		for _, job := range jobArray {
			if jobStr, ok := job.(string); ok {
				jobIDs = append(jobIDs, jobStr)
			}
		}
	}

	return jobIDs
}
