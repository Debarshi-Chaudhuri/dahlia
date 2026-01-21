package dynamodb

import "errors"

// ErrOptimisticLockFailed indicates that an optimistic lock check failed during update
// This happens when the record was modified by another process after it was read
var ErrOptimisticLockFailed = errors.New("optimistic lock failed: record was modified by another process")

// ErrNotFound indicates that the requested resource was not found
var ErrNotFound = errors.New("resource not found")

// IsOptimisticLockError checks if an error is an optimistic lock failure
func IsOptimisticLockError(err error) bool {
	return errors.Is(err, ErrOptimisticLockFailed)
}

// IsNotFoundError checks if an error indicates a resource was not found
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}
