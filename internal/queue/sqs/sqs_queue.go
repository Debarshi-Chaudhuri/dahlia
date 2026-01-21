// internal/queue/sqs/sqs_queue.go
package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"dahlia/internal/logger"
	queue "dahlia/internal/queue/iface"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type sqsMessage struct {
	rawMessage types.Message
}

func (m *sqsMessage) Body() string {
	if m.rawMessage.Body == nil {
		return ""
	}
	return *m.rawMessage.Body
}

func (m *sqsMessage) ReceiptHandle() string {
	if m.rawMessage.ReceiptHandle == nil {
		return ""
	}
	return *m.rawMessage.ReceiptHandle
}

func (m *sqsMessage) MessageID() string {
	if m.rawMessage.MessageId == nil {
		return ""
	}
	return *m.rawMessage.MessageId
}

// QueueConfig holds configuration for SQS queue
type QueueConfig struct {
	QueueURL        string
	WorkerCount     int
	MaxMessages     int32
	WaitTimeSeconds int32
}

// SQSQueue is a generic SQS queue consumer
type SQSQueue[T any] struct {
	client    *sqs.Client
	config    QueueConfig
	logger    logger.Logger
	processor queue.MessageProcessor[T]
	stopCh    chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewSQSQueue creates a new SQS queue with processor
func NewSQSQueue[T any](
	client *sqs.Client,
	config QueueConfig,
	processor queue.MessageProcessor[T],
	log logger.Logger,
) queue.Queue {
	if config.WorkerCount <= 0 {
		config.WorkerCount = 5
	}
	if config.MaxMessages <= 0 {
		config.MaxMessages = 1
	}
	if config.WaitTimeSeconds <= 0 {
		config.WaitTimeSeconds = 20
	}

	return &SQSQueue[T]{
		client:    client,
		config:    config,
		logger:    log.With(logger.String("component", "sqs_queue")),
		processor: processor,
		stopCh:    make(chan struct{}),
	}
}

func (q *SQSQueue[T]) Send(ctx context.Context, message interface{}) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	bodyStr := string(body)
	_, err = q.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &q.config.QueueURL,
		MessageBody: &bodyStr,
	})

	if err != nil {
		q.logger.Error("failed to send message to SQS",
			logger.String("queue_url", q.config.QueueURL),
			logger.Error(err))
		return fmt.Errorf("failed to send message: %w", err)
	}

	q.logger.Debug("message sent to queue",
		logger.String("queue_url", q.config.QueueURL))

	return nil
}

func (q *SQSQueue[T]) StartConsumer(ctx context.Context) error {
	q.mu.Lock()
	if q.running {
		q.mu.Unlock()
		return fmt.Errorf("consumer already running")
	}
	q.running = true

	// Create a long-lived context for workers, not tied to the startup context
	q.ctx, q.cancel = context.WithCancel(context.Background())
	q.mu.Unlock()

	q.logger.Info("starting SQS consumer",
		logger.String("queue_url", q.config.QueueURL),
		logger.Int("worker_count", q.config.WorkerCount))

	for i := 0; i < q.config.WorkerCount; i++ {
		q.wg.Add(1)
		go q.worker(q.ctx, i+1)
	}

	return nil
}

func (q *SQSQueue[T]) StopConsumer(ctx context.Context) error {
	q.mu.Lock()
	if !q.running {
		q.mu.Unlock()
		return fmt.Errorf("consumer not running")
	}
	q.mu.Unlock()

	q.logger.Info("stopping SQS consumer",
		logger.String("queue_url", q.config.QueueURL))

	// Cancel the worker context
	if q.cancel != nil {
		q.cancel()
	}

	close(q.stopCh)
	q.wg.Wait()

	q.mu.Lock()
	q.running = false
	q.mu.Unlock()

	q.logger.Info("SQS consumer stopped")
	return nil
}

func (q *SQSQueue[T]) worker(ctx context.Context, workerID int) {
	defer q.wg.Done()

	q.logger.Info("worker started",
		logger.Int("worker_id", workerID))

	for {
		select {
		case <-q.stopCh:
			q.logger.Info("worker stopping", logger.Int("worker_id", workerID))
			return
		default:
			q.processMessages(ctx, workerID)
		}
	}
}

func (q *SQSQueue[T]) processMessages(ctx context.Context, workerID int) {
	// Create a context with timeout for the ReceiveMessage call
	// Timeout should be longer than WaitTimeSeconds to allow long polling to complete
	receiveTimeout := time.Duration(q.config.WaitTimeSeconds+5) * time.Second
	receiveCtx, cancel := context.WithTimeout(ctx, receiveTimeout)
	defer cancel()

	result, err := q.client.ReceiveMessage(receiveCtx, &sqs.ReceiveMessageInput{
		QueueUrl:            &q.config.QueueURL,
		MaxNumberOfMessages: q.config.MaxMessages,
		WaitTimeSeconds:     q.config.WaitTimeSeconds,
		VisibilityTimeout:   60,
	})

	if err != nil {
		// Only log error if it's not a context cancellation from stopCh
		select {
		case <-q.stopCh:
			return
		default:
			q.logger.Error("failed to receive messages",
				logger.Int("worker_id", workerID),
				logger.Error(err))
			time.Sleep(1 * time.Second)
		}
		return
	}

	for _, msg := range result.Messages {
		select {
		case <-q.stopCh:
			return
		default:
			newCtx := context.Background()
			q.processMessage(newCtx, msg, workerID)
		}
	}
}

func (q *SQSQueue[T]) processMessage(ctx context.Context, msg types.Message, workerID int) {
	messageID := ""
	if msg.MessageId != nil {
		messageID = *msg.MessageId
	}

	var message T
	if err := json.Unmarshal([]byte(*msg.Body), &message); err != nil {
		q.logger.Error("failed to unmarshal message",
			logger.Int("worker_id", workerID),
			logger.String("message_id", messageID),
			logger.Error(err))
		q.deleteMessage(ctx, msg)
		return
	}

	q.logger.Info("processing message",
		logger.Int("worker_id", workerID),
		logger.String("message_id", messageID))

	success := q.processor.ProcessMessage(ctx, message)

	if success {
		q.deleteMessage(ctx, msg)
		q.logger.Info("message processed successfully",
			logger.Int("worker_id", workerID),
			logger.String("message_id", messageID))
	} else {
		q.logger.Warn("message processing failed, will retry",
			logger.Int("worker_id", workerID),
			logger.String("message_id", messageID))
	}
}

func (q *SQSQueue[T]) deleteMessage(ctx context.Context, msg types.Message) {
	_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &q.config.QueueURL,
		ReceiptHandle: msg.ReceiptHandle,
	})

	if err != nil {
		q.logger.Error("failed to delete message", logger.Error(err))
	}
}
