package repository

import "errors"

// ErrOptimisticLockFailed indicates that an optimistic lock check failed during update
// This happens when the record was modified by another process after it was read
var ErrOptimisticLockFailed = errors.New("optimistic lock failed: record was modified by another process")

// IsOptimisticLockError checks if an error is an optimistic lock failure
func IsOptimisticLockError(err error) bool {
	return errors.Is(err, ErrOptimisticLockFailed)
}
