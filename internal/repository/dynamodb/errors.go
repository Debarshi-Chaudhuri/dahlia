package dynamodb

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ErrOptimisticLockFailed indicates that an optimistic lock check failed during update
// This happens when the record was modified by another process after it was read
var ErrOptimisticLockFailed = errors.New("optimistic lock failed: record was modified by another process")

// ErrNotFound indicates that the requested resource was not found
var ErrNotFound = errors.New("resource not found")

// ErrDuplicateSignal indicates that a signal with the same signal_type, org_id, and timestamp already exists
var ErrDuplicateSignal = errors.New("duplicate signal: signal with same type, org_id, and timestamp already exists")

// IsOptimisticLockError checks if an error is an optimistic lock failure
func IsOptimisticLockError(err error) bool {
	return errors.Is(err, ErrOptimisticLockFailed)
}

// IsNotFoundError checks if an error indicates a resource was not found
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsDuplicateSignalError checks if an error indicates a duplicate signal
func IsDuplicateSignalError(err error) bool {
	// Check for our custom error
	if errors.Is(err, ErrDuplicateSignal) {
		return true
	}
	// Also check for the underlying DynamoDB conditional check failed error
	var condErr *types.ConditionalCheckFailedException
	return errors.As(err, &condErr)
}
